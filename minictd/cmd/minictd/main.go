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
		listen     = flag.String("listen", "127.0.0.1:7601", "listen address (host:port)")
		token      = flag.String("token", "", "bearer token (DISCOURAGED: visible in /proc/<pid>/cmdline; prefer $MINICTD_TOKEN or --token-file)")
		tokenFile  = flag.String("token-file", "", "file holding the bearer token (first line)")
		noAuth     = flag.Bool("insecure-no-auth", false, "allow running without a token (dev only: any local user could drive sandboxes as this uid)")
		stateDir   = flag.String("state-dir", "", "instances root (default ~/.local/state/minictd)")
		minictBin  = flag.String("minict-bin", "minict", "path to the patched minict runtime binary")
		execBin    = flag.String("minict-exec-bin", "", "path to minict-exec (default: ./minict-exec beside this binary, then PATH)")
		webproxy   = flag.String("webproxy", "", "path to tools/webproxy.py (host side of the unix-socket bridge)")
		python     = flag.String("python", "python3", "interpreter for webproxy.py")
		baseRootfs = flag.String("base-rootfs", "", "base rootfs dir backing Image \"base\" (sandbox/rootfs from mkrootfs.sh)")
		subnet     = flag.String("sandbox-subnet", "10.200.0.0/24", "defensive stub subnet reported by NetworkSubnets")
		gateway    = flag.String("sandbox-gateway", "10.200.0.1", "defensive stub gateway reported by NetworkSubnets")
		autostart  = flag.Bool("autostart", false, "on boot, Start() every instance that was running when the daemon went away (wake-recreate from spec.json; issue #91, D47)")
		reconcile  = flag.Int("reconcile-interval", 0, "proactive A3 reconcile loop interval in seconds (0 = off): heal state.json for instances whose payload died outside a query (issue #91, D47)")
		cgRoot     = flag.String("cgroup-root", os.Getenv("MINICTD_CGROUP_ROOT"), "cgroups v2 enforcement base (CG1/S2, issue #4; default $MINICTD_CGROUP_ROOT then the daemon's own delegated cgroup; enforcement degrades to notes + rlimit when no writable v2 subtree exists)")
		showVer    = flag.Bool("version", false, "print version and exit")
	)
	flag.Parse()

	if *showVer {
		fmt.Println("minictd", contract.Version)
		return
	}

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

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

	store := &minict.Store{
		Root:           *stateDir,
		MinictBin:      *minictBin,
		MinictExecBin:  *execBin,
		WebProxyPy:     *webproxy,
		Python:         *python,
		BaseRootfs:     *baseRootfs,
		SandboxSubnet:  *subnet,
		SandboxGateway: *gateway,
		CgroupRoot:     *cgRoot,
		Log:            log,
	}

	srv := server.New(store, *token, log)
	httpServer := &http.Server{
		Addr:              *listen,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
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
		_ = httpServer.Shutdown(ctx)
	}()

	log.Info("minictd: listening", "addr", *listen,
		"state_dir", *stateDir,
		"minict", *minictBin,
		"exec_mode", map[bool]string{true: "setns", false: "newns"}[*execBin != ""],
		"version", contract.Version)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fatal(log, "listen: %v", err)
	}
}

func fatal(log *slog.Logger, f string, a ...any) {
	log.Error(fmt.Sprintf(f, a...))
	os.Exit(1)
}
