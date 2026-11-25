package minict

// pasta.go — GĐ2 mục 4 (issue #165, D67): per-instance host→namespace
// port forwards served by pasta (passt project), declared in
// RunSpec.PastaPorts ("khai báo trong config instance", docs/roadmap.md).
//
// The daemon attaches pasta to the instance's EXISTING userns+netns after
// the payload is alive — the same out-of-band mechanics minict.sh has used
// since D37 (--userns/--netns resolved via /proc/<pid>/ns/*; pasta's euid
// equals the userns owner, so tap + route setup is rootless). The C shim
// never learns about pasta: pasta runs host-side as a daemon child, the
// same posture as webproxy.py — NOT subject to the shim seccomp filter,
// D11 blocklist and MD0 parity untouched.
//
// Forward semantics (passt.1): "-t first[-last][:toaddr]" — one spec arg
// per protocol (comma-joined entries). pasta binds forwarded host ports
// on all interfaces by default; the D42 default "auto" forwarding (ports
// bound in the namespace forwarded back on the same host port) stays ON —
// explicit PastaPorts only ADD deterministic forwards.
//
// Fail-closed posture (D34/D37): pasta_ports declared but the binary,
// /dev/net/tun, or the netns are missing => Run/Start returns an error
// and Run's R4 rollback wipes the half-built instance. No silent skip.
//
// DNS (D42 parity, improved): --dns-forward <host default gw> maps
// namespace DNS flows to the host resolver. The resolver file is written
// BEFORE the payload spawns (legacy: rootfs/etc, overlay: upper/etc —
// never lower: zero-copy lower may be a SHARED source tree), removing
// minict.sh's mount-order race. Any pasta failure fails the whole
// Run/Start, so no dead resolver can survive (fail-closed replaces
// minict.sh's write-after-attach ordering).

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	// pastaPortHostMin mirrors webportsOf's 1024 floor: pasta runs
	// rootless (daemon uid), host ports < 1024 would fail at bind time.
	pastaPortHostMin = 1024
	pastaPortHostMax = 65535
	pastaPortNsMin   = 1
	pastaPortNsMax   = 65535
	// pastaPortsCap bounds the argv and config size; docker -p has no
	// cap but our instances are tiny — 16 is generous.
	pastaPortsCap = 16

	pastaReadyTimeout = 5 * time.Second // D37 readiness window
	pastaAliveDelay   = 300 * time.Millisecond
	pastaNsWait       = 2 * time.Second // shim unshare window (D67 fix)
)

// pastaProto is the transport protocol of one forward.
type pastaProto string

const (
	pastaTCP pastaProto = "tcp"
	pastaUDP pastaProto = "udp"
)

// pastaFwd is one validated host→namespace forward.
type pastaFwd struct {
	Proto pastaProto
	Host  int
	NS    int
}

func (f pastaFwd) String() string { return fmt.Sprintf("%s/%d->%d", f.Proto, f.Host, f.NS) }

// key identifies a forward's host-side bind (S6 collision map key).
func (f pastaFwd) key() string { return fmt.Sprintf("%s/%d", f.Proto, f.Host) }

// parsePastaPorts validates RunSpec.PastaPorts fail-closed host-side so a
// bad entry fails Run with a clear 4xx instead of dying inside pasta's
// argv parsing. Grammar per entry (trim first):
//
//	[tcp|udp/]<host>:<ns>     — tcp default; numbers only (no ranges in v1)
func parsePastaPorts(entries []string) ([]pastaFwd, error) {
	if len(entries) == 0 {
		return nil, nil
	}
	if len(entries) > pastaPortsCap {
		return nil, fmt.Errorf("spec.PastaPorts: %d entries exceeds the cap of %d", len(entries), pastaPortsCap)
	}
	seen := map[string]bool{}
	fwds := make([]pastaFwd, 0, len(entries))
	for _, e := range entries {
		raw := strings.TrimSpace(e)
		proto := pastaTCP
		rest := raw
		if before, after, ok := strings.Cut(raw, "/"); ok {
			switch strings.ToLower(before) {
			case string(pastaTCP):
				proto = pastaTCP
			case string(pastaUDP):
				proto = pastaUDP
			default:
				return nil, fmt.Errorf("spec.PastaPorts entry %q: protocol must be tcp or udp", raw)
			}
			rest = after
		}
		if strings.ContainsAny(rest, " \t-") {
			return nil, fmt.Errorf("spec.PastaPorts entry %q: ranges/spaces are not supported (v1 grammar: [tcp|udp/]<host>:<ns>)", raw)
		}
		hp, np, ok := strings.Cut(rest, ":")
		if !ok || hp == "" || np == "" {
			return nil, fmt.Errorf("spec.PastaPorts entry %q: need [tcp|udp/]<host>:<ns>", raw)
		}
		host, err := strconv.Atoi(hp)
		if err != nil {
			return nil, fmt.Errorf("spec.PastaPorts entry %q: host port %q is not a number", raw, hp)
		}
		ns, err := strconv.Atoi(np)
		if err != nil {
			return nil, fmt.Errorf("spec.PastaPorts entry %q: namespace port %q is not a number", raw, np)
		}
		if host < pastaPortHostMin || host > pastaPortHostMax {
			return nil, fmt.Errorf("spec.PastaPorts entry %q: host port %d outside %d..%d (rootless bind floor)", raw, host, pastaPortHostMin, pastaPortHostMax)
		}
		if ns < pastaPortNsMin || ns > pastaPortNsMax {
			return nil, fmt.Errorf("spec.PastaPorts entry %q: namespace port %d outside %d..%d", raw, ns, pastaPortNsMin, pastaPortNsMax)
		}
		key := string(proto) + "/" + strconv.Itoa(host)
		if seen[key] {
			return nil, fmt.Errorf("spec.PastaPorts entry %q: duplicate host bind %s", raw, key)
		}
		seen[key] = true
		fwds = append(fwds, pastaFwd{Proto: proto, Host: host, NS: ns})
	}
	return fwds, nil
}

// pastaArgs builds the pasta argv for one attach. No shell — exec arglist
// only (nothing from the spec reaches a shell). D37/D42 parity with
// minict.sh spawn_pasta, plus explicit forwards from the config.
func pastaArgs(shimPID int, fwds []pastaFwd, gw string) []string {
	args := []string{
		"--foreground",
		"--userns", fmt.Sprintf("/proc/%d/ns/user", shimPID),
		"--netns", fmt.Sprintf("/proc/%d/ns/net", shimPID),
		"--config-net",
	}
	if gw != "" {
		args = append(args, "--dns-forward", gw)
	}
	var tcp, udp []string
	for _, f := range fwds {
		spec := fmt.Sprintf("%d:%d", f.Host, f.NS)
		if f.Proto == pastaUDP {
			udp = append(udp, spec)
		} else {
			tcp = append(tcp, spec)
		}
	}
	if len(tcp) > 0 {
		args = append(args, "-t", strings.Join(tcp, ","))
	}
	if len(udp) > 0 {
		args = append(args, "-u", strings.Join(udp, ","))
	}
	return args
}

// hostDefaultGW parses the host's default-route gateway out of
// /proc/net/route (stdlib-only — no `ip` dependency). Rows are hex
// little-endian; Destination 00000000 = default route. Empty result = no
// default route or unreadable file (caller fail-soft, D42).
func hostDefaultGW(routeFile string) string {
	b, err := os.ReadFile(routeFile)
	if err != nil {
		return ""
	}
	for i, line := range strings.Split(string(b), "\n") {
		if i == 0 || strings.TrimSpace(line) == "" {
			continue // header row / trailing newline
		}
		f := strings.Fields(line)
		if len(f) < 3 || f[1] != "00000000" {
			continue
		}
		gw := f[2]
		if len(gw) != 8 {
			continue
		}
		var parts [4]byte
		for j := 0; j < 4; j++ {
			v, err := strconv.ParseUint(gw[2*j:2*j+2], 16, 8)
			if err != nil {
				return ""
			}
			// little-endian: memory byte j is octet 3-j of the dotted quad
			parts[3-j] = byte(v)
		}
		return fmt.Sprintf("%d.%d.%d.%d", parts[0], parts[1], parts[2], parts[3])
	}
	return ""
}

// pastaResolverDir returns the directory the D42 resolver file must be
// written to. Overlay instances (D66): UPPER/etc — lower may be a SHARED
// zero-copy source (base/dir/template); writing there would corrupt the
// base image for every future instance (MD44 guards base immutability).
func pastaResolverDir(dir string, overlay bool) string {
	if overlay {
		return filepath.Join(dir, "upper", "etc")
	}
	return filepath.Join(dir, "rootfs", "etc")
}

// bakeResolver writes `nameserver <gw>` pre-spawn so the resolver is in
// the tree BEFORE the shim mounts/pivots (no mount race). Failure is a
// note, not an error (fail-soft D42: egress still works, only by-name
// resolution doesn't — minict.sh parity). A pasta attach failure later
// fails the whole Run/Start, so no dead resolver survives.
func (s *Store) bakeResolver(dir string, overlay bool, gw string) []string {
	notes := []string{}
	if gw == "" {
		return append(notes, "pasta DNS: no host default route — resolver not configured (D42 fail-soft)")
	}
	d := pastaResolverDir(dir, overlay)
	if err := os.MkdirAll(d, 0755); err != nil {
		return append(notes, fmt.Sprintf("pasta DNS: mkdir %s: %v — resolve-by-name will not work", d, err))
	}
	if err := os.WriteFile(filepath.Join(d, "resolv.conf"), []byte("nameserver "+gw+"\n"), 0644); err != nil {
		return append(notes, fmt.Sprintf("pasta DNS: resolver write: %v — resolve-by-name will not work", err))
	}
	return append(notes, "pasta DNS: nameserver "+gw+" baked into instance resolver (D42)")
}

// pastaBin resolves the pasta binary: explicit --pasta-bin first, then
// ./pasta beside the minictd binary, then PATH (MinictExecBin discovery
// parity). Empty resolution is an ERROR whenever pasta_ports is declared
// (fail-closed; run tools/fetch-pasta.sh for the pinned binary).
func (s *Store) pastaBin() (string, error) {
	if s.PastaBin != "" {
		if fi, err := os.Stat(s.PastaBin); err == nil && !fi.IsDir() {
			return s.PastaBin, nil
		}
		return "", fmt.Errorf("pasta binary %q not found (run tools/fetch-pasta.sh or fix --pasta-bin)", s.PastaBin)
	}
	if exe, err := os.Executable(); err == nil {
		cand := filepath.Join(filepath.Dir(exe), "pasta")
		if fi, err := os.Stat(cand); err == nil && !fi.IsDir() {
			return cand, nil
		}
	}
	if p, err := exec.LookPath("pasta"); err == nil {
		return p, nil
	}
	return "", fmt.Errorf("pasta binary not found (--pasta-bin, beside minictd, or PATH; run tools/fetch-pasta.sh for the pinned build)")
}

// spawnPasta attaches pasta to a live instance's userns+netns and waits
// until the tap carries a default route (D37 readiness: ns-scoped
// /proc/<shim>/net/route), then re-checks aliveness (a pasta that dies
// right after configuring the tap must fail the attach — minict.sh
// parity). Returns the pasta pid, attach latency (host-side, reported
// SEPARATELY from minict cold_start — AB1 precedent) and an error.
// Errors here are fatal for the operation: Run's R4 rollback wipes the
// instance; Start kills the payload (fail-closed, D34).
func (s *Store) spawnPasta(name, dir string, shimPID int, fwds []pastaFwd, gw string) (pid int, attachMs int64, err error) {
	bin, err := s.pastaBin()
	if err != nil {
		return 0, 0, err
	}
	// The shim must have COMPLETED its unshare() before pasta spawns:
	// /proc/<pid>/ns/* exists from fork time but still points at the INIT
	// namespaces until unshare runs. Attaching then makes pasta setns into
	// the INIT userns — EINVAL ("reentering own/ancestor namespace",
	// user_namespaces(7)) — and the readiness loop would false-positive on
	// the init route table (observed on a loaded runner, CI run
	// 45587261541: "Couldn't enter user namespace … Invalid argument" after
	// an instant routeOK; MD47's "attach 0ms" note was the same artifact).
	// Readlink (not stat): a dead payload gives ENOENT here.
	selfU, _ := os.Readlink("/proc/self/ns/user")
	selfN, _ := os.Readlink("/proc/self/ns/net")
	if err := waitNsDistinct(selfU, selfN, shimPID, pastaNsWait); err != nil {
		return 0, 0, fmt.Errorf("pasta attach: %w", err)
	}
	tun, terr := os.Stat("/dev/net/tun")
	if terr != nil || tun.Mode()&(os.ModeDevice|os.ModeCharDevice) != (os.ModeDevice|os.ModeCharDevice) {
		return 0, 0, fmt.Errorf("pasta attach: /dev/net/tun missing on this host (D34: dev container env-blocked; GitHub runners have it)")
	}

	logf, err := os.OpenFile(filepath.Join(dir, "pasta.log"), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return 0, 0, fmt.Errorf("pasta log: %w", err)
	}
	defer logf.Close()
	cmd := exec.Command(bin, pastaArgs(shimPID, fwds, gw)...)
	cmd.Stdin = nil // /dev/null semantics
	cmd.Stdout, cmd.Stderr = logf, logf
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return 0, 0, fmt.Errorf("start pasta: %w", err)
	}
	pid = cmd.Process.Pid
	go func() { // reaper: log only — state heal owns the port recovery
		if werr := cmd.Wait(); werr != nil {
			s.Log.Warn("pasta exited", "name", name, "pid", pid, "err", fmt.Sprint(werr))
		} else {
			s.Log.Info("pasta exited", "name", name, "pid", pid)
		}
	}()

	t0 := time.Now()
	deadline := time.Now().Add(pastaReadyTimeout)
	routeOK := false
	for time.Now().Before(deadline) {
		if b, rerr := os.ReadFile(fmt.Sprintf("/proc/%d/net/route", shimPID)); rerr == nil {
			for _, ln := range strings.Split(string(b), "\n")[1:] {
				f := strings.Fields(ln)
				if len(f) > 1 && f[1] == "00000000" {
					routeOK = true
					break
				}
			}
		}
		if routeOK {
			break
		}
		if syscall.Kill(pid, 0) != nil {
			break // pasta died early — log tail says why
		}
		time.Sleep(50 * time.Millisecond)
	}
	attachMs = time.Since(t0).Milliseconds()
	if !routeOK {
		s.killBestEffort(pid, 200*time.Millisecond)
		return 0, attachMs, fmt.Errorf("pasta attach: no default route within %s (log %s: %s)",
			pastaReadyTimeout, filepath.Join(dir, "pasta.log"), pastaLogTail(dir))
	}
	time.Sleep(pastaAliveDelay)
	if syscall.Kill(pid, 0) != nil {
		return 0, attachMs, fmt.Errorf("pasta attached %s then died (log %s: %s)",
			name, filepath.Join(dir, "pasta.log"), pastaLogTail(dir))
	}
	// pasta-pid file (minict.sh parity — operational debug aid)
	_ = os.WriteFile(filepath.Join(dir, "pasta-pid"), []byte(strconv.Itoa(pid)+"\n"), 0644)
	return pid, attachMs, nil
}

// pastaLogTail returns the last 1KB of pasta.log for error messages.
func pastaLogTail(dir string) string {
	b, err := os.ReadFile(filepath.Join(dir, "pasta.log"))
	if err != nil {
		return "unreadable"
	}
	t := string(b)
	if len(t) > 1024 {
		t = t[len(t)-1024:]
	}
	return strings.TrimSpace(t)
}

// waitNsDistinct waits until /proc/<shimPID>'s user AND net namespaces
// differ from the daemon's own (i.e. the shim has executed unshare). Pure
// link-target comparison — no /proc/net scraping, no uid assumptions.
func waitNsDistinct(selfUser, selfNet string, shimPID int, d time.Duration) error {
	deadline := time.Now().Add(d)
	for {
		u, err1 := os.Readlink(fmt.Sprintf("/proc/%d/ns/user", shimPID))
		n, err2 := os.Readlink(fmt.Sprintf("/proc/%d/ns/net", shimPID))
		if err1 == nil && err2 == nil && u != selfUser && n != selfNet {
			return nil
		}
		if err1 != nil || err2 != nil {
			return fmt.Errorf("/proc/%d/ns gone (payload died before unshare?)", shimPID)
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("shim namespaces still == daemon's after %s (unshare not started?)", d)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// stopPasta kills a recorded pasta attach (pid reuse guarded by the
// recorded start ticks) and clears the pasta-pid file + state fields.
// Best-effort: pasta may have died already (payload crash, daemon
// restart). Called from Stop, Remove and the alive() heal path — pasta
// holds HOST-side listening sockets for its forwards, so every path that
// ends a payload must end pasta too or the next Start would bind-fail.
func (s *Store) stopPasta(st *InstanceState) {
	if st.PastaPID > 0 && s.pidAlive(st.PastaPID, st.PastaTicks) {
		s.killBestEffort(st.PastaPID, 200*time.Millisecond)
	}
	st.PastaPID = 0
	st.PastaTicks = 0
	_ = os.Remove(filepath.Join(s.dir(st.Name), "pasta-pid"))
}

// pastaPortsHeldBy maps declared pasta host binds ("tcp/18080") to the
// RUNNING instance that holds them — S6 fail-closed parity (issue #124)
// for pasta forwards: two instances declaring the same host port would
// otherwise have the second pasta die at bind time with only a log line.
// Third-party bind collisions stay pasta's own failure (spawn is
// fail-closed + wipe); a pre-probe cannot predict pasta's bind semantics
// (all-interface default) without pretending to be pasta.
func (s *Store) pastaPortsHeldBy(except string) (map[string]string, error) {
	held := map[string]string{}
	names, err := s.ListByNamePrefix(context.Background(), "")
	if err != nil {
		return nil, err
	}
	for _, n := range names {
		if n == except {
			continue
		}
		st, err := s.loadState(n)
		if err != nil {
			continue // ghost dir / missing state — skip, not a port holder
		}
		st = s.alive(st)
		if st.State != StateRunning {
			continue
		}
		spec, err := s.loadSpec(n)
		if err != nil {
			continue
		}
		fwds, err := parsePastaPorts(spec.PastaPorts)
		if err != nil {
			continue
		}
		for _, f := range fwds {
			held[f.key()] = n
		}
	}
	return held, nil
}

// pastaFwdList renders the forwards for notes ("tcp/18080->8080, ...").
func pastaFwdList(fwds []pastaFwd) string {
	parts := make([]string, len(fwds))
	for i, f := range fwds {
		parts[i] = f.String()
	}
	return strings.Join(parts, ", ")
}
