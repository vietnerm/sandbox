// minictd — rootless sandbox daemon implementing the sandboxd runtime
// contract (14 ops + RunSpec) on top of the minict runtime.
//
// It owns a state directory, drives the patched minict binary (MINICT_SPEC,
// D23/D24), serves the HTTP API mirror of contract.Runtime, and delegates
// exec to minict-exec (setns) with the new-ns PoC mode as fallback (D5).
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/vietnerm/sandbox/minictd/contract"
	"github.com/vietnerm/sandbox/minictd/internal/minict"
	"github.com/vietnerm/sandbox/minictd/internal/server"
)

func main() {
	var (
		listen     = flag.String("listen", "127.0.0.1:7601", "listen address (host:port); empty disables TCP (unix-socket-only posture, S4)")
		unixSocket = flag.String("unix-socket", "", "unix socket path (S4, issue #116; socket 0600 + dir 0700 fail-closed, symlink/non-socket refused; empty = off; token auth stays REQUIRED on every transport)")
		token      = flag.String("token", "", "bearer token (DISCOURAGED: visible in /proc/<pid>/cmdline; prefer $MINICTD_TOKEN or --token-file)")
		tokenFile  = flag.String("token-file", "", "file holding the bearer token (first line)")
		noAuth     = flag.Bool("insecure-no-auth", false, "allow running without a token (dev only: any local user could drive sandboxes as this uid)")
		stateDir   = flag.String("state-dir", "", "instances root (default ~/.local/state/minictd)")
		minictBin  = flag.String("minict-bin", "minict", "path to the patched minict runtime binary")
		execBin    = flag.String("minict-exec-bin", "", "path to minict-exec (default: ./minict-exec beside this binary, then PATH)")
		pastaBin   = flag.String("pasta-bin", "", "path to the pasta binary (passt project; pinned build via tools/fetch-pasta.sh; default: ./pasta beside this binary, then PATH). Only needed when an instance declares pasta_ports — then missing binary = fail-closed (D34)")
		webproxy   = flag.String("webproxy", "", "path to tools/webproxy.py (host side of the unix-socket bridge)")
		python     = flag.String("python", "python3", "interpreter for webproxy.py")
		baseRootfs = flag.String("base-rootfs", "", "base rootfs dir backing Image \"base\" (sandbox/rootfs from mkrootfs.sh)")
		subnet     = flag.String("sandbox-subnet", "10.200.0.0/24", "defensive stub subnet reported by NetworkSubnets")
		gateway    = flag.String("sandbox-gateway", "10.200.0.1", "defensive stub gateway reported by NetworkSubnets")
		autostart  = flag.Bool("autostart", false, "on boot, Start() every instance that was running when the daemon went away (wake-recreate from spec.json; issue #91, D47)")
		reconcile  = flag.Int("reconcile-interval", 0, "proactive A3 reconcile loop interval in seconds (0 = off): heal state.json for instances whose payload died outside a query (issue #91, D47)")
		cgRoot     = flag.String("cgroup-root", os.Getenv("MINICTD_CGROUP_ROOT"), "cgroups v2 enforcement base (CG1/S2, issue #4; default $MINICTD_CGROUP_ROOT then the daemon's own delegated cgroup; enforcement degrades to notes + rlimit when no writable v2 subtree exists)")
		traefikDir = flag.String("traefik-dir", "", "directory for generated Traefik file-provider routes (G3, issue #97; one sandbox-<name>.yml per running instance; empty = feature off)")
		tfDomain   = flag.String("traefik-domain", "localhost", "domain suffix for generated Traefik Host() rules (s-<name>.<domain>, G3)")
		showVer      = flag.Bool("version", false, "print version and exit")
		logFormat    = flag.String("log-format", "json", "log format: text or json (default json, GĐ2 mục 7, issue #168). JSON emits one structured line per lifecycle event with time/level/msg/instance/op/result/duration_ms/request_id")
		metricsLn    = flag.String("metrics-listen", "", "NOT IMPLEMENTED: /metrics is served on the main --listen/--unix-socket with the same token. Any non-empty value is rejected fail-closed (exit 1) so a future loopback-only listener can be added without widening the attack surface today (GĐ2 mục 7c, issue #168)")
	)
	flag.Parse()

	if *showVer {
		fmt.Println("minictd", contract.Version)
		return
	}

	// GĐ2 mục 7a (issue #168, D68): structured JSON logs by default;
	// --log-format text keeps the legacy TextHandler for operator debugging.
	// ReplaceAttr normalizes level to lowercase (info/warn/error) for stable
	// log parsing; time is RFC3339Nano (slog default). The handler is shared
	// by the daemon and the HTTP request logger (request_id injected via
	// slog.With in server.go middleware).
	handlerOpts := &slog.HandlerOptions{Level: slog.LevelInfo,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.LevelKey && len(groups) == 0 {
				lvl, ok := a.Value.Any().(slog.Level)
				if ok {
					a.Value = slog.StringValue(strings.ToLower(lvl.String()))
				}
			}
			return a
		},
	}
	var log *slog.Logger
	switch strings.ToLower(*logFormat) {
	case "text":
		log = slog.New(slog.NewTextHandler(os.Stderr, handlerOpts))
	case "json", "":
		log = slog.New(slog.NewJSONHandler(os.Stderr, handlerOpts))
	default:
		fatal(nil, "invalid --log-format %q (want text|json)", *logFormat)
	}

	// GĐ2 mục 7c (issue #168, D68): --metrics-listen is a placeholder.
	// /metrics is served on the main listener with the same token. Any
	// non-empty value exits fail-closed so a future loopback-only listener
	// can be added without widening the attack surface today. Non-loopback
	// addresses are ALWAYS rejected (roadmap mục 7c).
	if *metricsLn != "" {
		fatal(log, "--metrics-listen is not implemented: /metrics is served on --listen/--unix-socket with the same token (got %q; a future loopback-only listener may relax this)", *metricsLn)
	}

	// R3 (2026-11-18): a token-less daemon on 127.0.0.1 lets ANY local
	// user create sandboxes with arbitrary bind mounts as this uid, so
	// auth is mandatory unless explicitly waived.
	if *token == "" {
		*token = os.Getenv("MINICTD_TOKEN")
	}
	if *token == "" && *tokenFile != "" {
		b, err := os.ReadFile(*tokenFile)
		if err != nil {
			fatal(log, "read --token-file: %v", err)
		}
		*token = strings.TrimSpace(strings.SplitN(string(b), "\n", 2)[0])
	}
	if *token == "" && !*noAuth {
		fatal(log, "no token: set $MINICTD_TOKEN or --token-file (or --insecure-no-auth for local dev)")
	}
	if *token == "" {
		log.Warn("minictd: running WITHOUT auth (--insecure-no-auth)")
	}
	_ = os.Unsetenv("MINICTD_TOKEN")

	if *stateDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			fatal(log, "resolve home: %v", err)
		}
		*stateDir = filepath.Join(home, ".local", "state", "minictd")
	}

	// Default minict-exec discovery: beside this binary, then PATH.
	if *execBin == "" {
		if exe, err := os.Executable(); err == nil {
			cand := filepath.Join(filepath.Dir(exe), "minict-exec")
			if _, err := os.Stat(cand); err == nil {
				*execBin = cand
			}
		}
	}
	// D67 (issue #165): pasta binary discovery mirrors MinictExecBin —
	// beside this binary, then PATH (resolution happens at spawn time and
	// is fail-closed when pasta_ports is declared).
	if *pastaBin == "" {
		if exe, err := os.Executable(); err == nil {
			cand := filepath.Join(filepath.Dir(exe), "pasta")
			if _, err := os.Stat(cand); err == nil {
				*pastaBin = cand
			}
		}
	}

	store := &minict.Store{
		Root:           *stateDir,
		MinictBin:      *minictBin,
		MinictExecBin:  *execBin,
		PastaBin:       *pastaBin,
		WebProxyPy:     *webproxy,
		Python:         *python,
		BaseRootfs:     *baseRootfs,
		SandboxSubnet:  *subnet,
		SandboxGateway: *gateway,
		CgroupRoot:     *cgRoot,
		TraefikDir:     *traefikDir,
		TraefikDomain:  *tfDomain,
		Log:            log,
	}

	// G3 (issue #97): reality first — rebuild the route dir from the
	// state dir at boot (covers daemon restarts with Traefik watching
	// the directory while minictd was away).
	if *traefikDir != "" {
		if err := store.SyncAllRoutes(); err != nil {
			log.Warn("minictd: route sync failed", "err", err.Error())
		}
	}

	srv := server.New(store, *token, log)
	httpServer := &http.Server{
		Addr:              *listen,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	// S4 (issue #116): open listeners BEFORE any background work so a
	// bad transport config dies fast and nothing half-boots. Both
	// transports serve the SAME handler (auth included — R3: the token
	// stays required on the unix socket too).
	var listeners []net.Listener
	if *listen != "" {
		ln, err := net.Listen("tcp", *listen)
		if err != nil {
			fatal(log, "listen tcp %s: %v", *listen, err)
		}
		listeners = append(listeners, ln)
	}
	if *unixSocket != "" {
		ln, err := openUnixSocket(*unixSocket)
		if err != nil {
			fatal(log, "%v", err)
		}
		listeners = append(listeners, ln)
	}
	if len(listeners) == 0 {
		fatal(log, "no listener: set --listen host:port and/or --unix-socket PATH")
	}

	ctx, cancelCtx := context.WithCancel(context.Background())
	defer cancelCtx()

	// A3 (issue #91, D47): proactive reconcile — compare state dir vs
	// /proc every N seconds and heal instances whose payload died
	// outside a query. Off by default; the lazy path (alive on query)
	// stays as the fallback when this flag is 0.
	if *reconcile > 0 {
		go func() {
			t := time.NewTicker(time.Duration(*reconcile) * time.Second)
			defer t.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-t.C:
					healed, err := store.ReconcileAll()
					if err != nil {
						log.Warn("minictd reconcile", "err", err.Error())
						continue
					}
					for _, n := range healed {
						log.Info("minictd reconcile: healed (payload died)", "name", n)
					}
				}
			}
		}()
		log.Info("minictd: reconcile loop on", "interval_s", *reconcile)
	}

	// Autostart (issue #91, D47): recreate instances that were running
	// when the daemon went away (reboot, daemon crash, unit restart).
	// Start() is idempotent for live instances and heals then spawns
	// from spec.json for dead ones. Operator-stopped instances stay
	// stopped (state=stopped -> not in the recovery set).
	if *autostart {
		go func() {
			want, err := store.AutostartNames()
			if err != nil {
				log.Warn("minictd autostart: scan failed", "err", err.Error())
				return
			}
			for _, n := range want {
				sctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
				if err := store.Start(sctx, n); err != nil {
					log.Warn("minictd autostart: start failed", "name", n, "err", err.Error())
				} else {
					log.Info("minictd autostart: started", "name", n)
				}
				cancel()
			}
		}()
	}

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-shutdown
		log.Info("minictd: shutting down (instances keep running; re-attach with Start)")
		cancelCtx()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(ctx) // also closes every listener (Shutdown closes ln)
		if *unixSocket != "" {
			// Unlink the socket on clean shutdown — stale sockets are
			// handled by openUnixSocket, but leaving a socket file that
			// nothing serves behind is confusing for operators and keeps
			// the 0700 dir looking "live".
			if err := os.Remove(*unixSocket); err != nil && !os.IsNotExist(err) {
				log.Warn("minictd: unlink unix socket", "err", err.Error())
			}
		}
	}()

	log.Info("minictd: listening",
		"tcp", map[bool]string{true: *listen, false: "off"}[*listen != ""],
		"unix", map[bool]string{true: *unixSocket, false: "off"}[*unixSocket != ""],
		"state_dir", *stateDir,
		"minict", *minictBin,
		"exec_mode", map[bool]string{true: "setns", false: "newns"}[*execBin != ""],
		"version", contract.Version)
	// S4 (issue #116): serve the same handler on every listener. First
	// serve error wins; http.ErrServerClosed (graceful shutdown path) is
	// not an error.
	servErr := make(chan error, len(listeners))
	for _, ln := range listeners {
		l := ln
		go func() {
			err := httpServer.Serve(l)
			if err != nil && err != http.ErrServerClosed {
				err = fmt.Errorf("serve %s: %w", l.Addr(), err)
			} else {
				err = nil
			}
			servErr <- err
		}()
	}
	if err := <-servErr; err != nil {
		fatal(log, "%v", err)
	}
}

func fatal(log *slog.Logger, f string, a ...any) {
	if log != nil {
		log.Error(fmt.Sprintf(f, a...))
	} else {
		fmt.Fprintf(os.Stderr, "minictd: "+f+"\n", a...)
	}
	os.Exit(1)
}
