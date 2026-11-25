package minict

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/vietnerm/sandbox/minictd/contract"
)

// Store owns the minictd instance directory and knows how to drive the
// minict binary. It is the single implementation of contract.Runtime on the
// minict side (see runtime.go for the 14 ops).
type Store struct {
	// Root is the instances root directory, one subdir per sandbox:
	//   <root>/<name>/{spec.json,state.json,spec.minict,log,rootfs/,lock}
	Root string
	// MinictBin is the compiled minict runtime binary (sandbox/minict).
	MinictBin string
	// MinictExecBin is the setns-based exec helper (cmd/minict-exec).
	// Empty or missing => exec falls back to the new-ns mode (D5).
	MinictExecBin string
	// WebProxyPy is tools/webproxy.py (host side of the unix-socket
	// bridge). Needed only when the spec asks for webports.
	WebProxyPy string
	// Python is the interpreter used to run WebProxyPy (default python3).
	Python string
	// BaseRootfs is the prebuilt base rootfs (sandbox/rootfs from
	// mkrootfs.sh) backing Image == "base".
	BaseRootfs string
	// SandboxSubnet/Gateway are returned by NetworkSubnets as a defensive
	// stub for the control plane's netguard (minict netns is loopback-only,
	// so this is belt-and-braces, not routing truth).
	SandboxSubnet  string
	SandboxGateway string
	// CgroupRoot overrides the cgroups v2 enforcement base (CG1/S2,
	// issue #4). Empty = auto-probe from the daemon's own cgroup
	// (/proc/self/cgroup); on hosts without a delegated subtree the
	// probe fails soft and enforcement degrades to notes + rlimit.
	CgroupRoot string
	// TraefikDir is the directory receiving generated Traefik
	// file-provider routes (G3, issue #97). Empty = feature off
	// (fail-soft, CG1 pattern). One sandbox-<name>.yml per running
	// instance; managed files all carry the "sandbox-" prefix.
	TraefikDir string
	// TraefikDomain is the Host() rule suffix for generated routes
	// (s-<name>.<domain>). Empty falls back to "localhost".
	TraefikDomain string
	// cg is the lazily-resolved cgroup manager (CG1). Guarded by cgMu
	// because Run/Start can run concurrently for different instances.
	cg   *cgroupManager
	cgMu sync.Mutex
	// Log receives structured daemon logging.
	Log *slog.Logger
}

// Instance states (minictd state machine — A3 in the integration plan).
//
//	creating -> running  (Run/Start finished spawning the payload)
//	running  -> stopped  (Stop/Remove, or the payload died on its own)
//	stopped  -> running  (Start: same rootfs, same spec, new payload)
//	stopped  -> removed  (Remove: directory is gone)
const (
	StateCreating = "creating"
	StateRunning  = "running"
	StateStopped  = "stopped"
)

// ExecMode records how exec is served for an instance:
//
//	setns — minict-exec joins the running instance's namespaces
//	        (RootlessKit/podman model; requires MinictExecBin)
//	newns — legacy PoC mode: a fresh namespace set on the same rootfs
//	        (isolated from the running payload; D5 fallback)
const (
	ExecSetns = "setns"
	ExecNewns = "newns"
)

// InstanceState is minictd's own persisted state (state.json). It is the
// operational truth for liveness; spec.json is the byte-exact RunSpec used
// for re-Start and for Inspect fidelity.
type InstanceState struct {
	Name       string      `json:"name"`
	State      string      `json:"state"`
	PID        int         `json:"pid,omitempty"`
	StartTicks uint64      `json:"start_ticks,omitempty"` // /proc/<pid>/stat field 22 — guards pid reuse
	ProxyPIDs  map[int]int `json:"proxy_pids,omitempty"`  // hostPort -> webproxy.py pid
	Ports      []int       `json:"ports,omitempty"`       // preview ports (host == container)
	CreatedAt  time.Time   `json:"created_at"`
	StartedAt  time.Time   `json:"started_at,omitempty"`
	ExecMode   string      `json:"exec_mode"`
	RootfsSrc  string      `json:"rootfs_src"`
	Notes      []string    `json:"notes,omitempty"`
	Exited     bool        `json:"exited,omitempty"`
	ExitCode   int         `json:"exit_code,omitempty"` // recorded by the reaper goroutine, when known
	KilledBy   string      `json:"killed_by,omitempty"` // "stop" | "remove" — distinguishes an operator stop
	// CgroupPath is the cgroups v2 dir enforcing this instance's
	// resources (CG1/S2, issue #4). Empty = not enforced (notes-only
	// fallback). Persisted so Stop/Remove/heal can clean it up even
	// across a daemon restart.
	CgroupPath string `json:"cgroup_path,omitempty"`
	// Overlay records that this instance runs on the overlayfs rootfs
	// (GĐ2 mục 3 residual, issue #162, D66): rootfs is an EMPTY
	// MOUNTPOINT host-side, the real tree is upper over lower. Snapshot
	// refuses (empty archive — the mount lives only in the shim's mount
	// ns) and spawnProxies resolves relay sockets in upper, not rootfs.
	Overlay bool `json:"overlay,omitempty"`
}

// nameRe mirrors docker's name charset: alphanumeric first char, then
// alphanumerics, underscore, dot, dash. Length 1..64.
var nameRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,63}$`)

// envKeyRe is what MINICT_SPEC accepts for env keys (checked host-side so a
// bad key fails Run with a clear error instead of a C-side exit 125).
var envKeyRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// ErrInvalidName is returned (HTTP 400) for a name outside nameRe. Every
// op that turns a name into a path goes through validName first: the HTTP
// layer hands PathValue() through unescaped, so "..%2F..%2Fx" would
// otherwise reach filepath.Join(Root, name) and escape the state dir
// (review finding R1 at integration, 2026-11-18).
var ErrInvalidName = errors.New("invalid instance name")

func validName(name string) error {
	if name == "" {
		return fmt.Errorf("%w: spec.Name is required", ErrInvalidName)
	}
	if !nameRe.MatchString(name) {
		return fmt.Errorf("%w: %q (want [a-zA-Z0-9][a-zA-Z0-9_.-]{0,63})", ErrInvalidName, name)
	}
	return nil
}

func (s *Store) dir(name string) string { return filepath.Join(s.Root, name) }

const (
	fSpec  = "spec.json"
	fState = "state.json"
	fMspec = "spec.minict"
	fLog   = "log"
	fLock  = "lock"
)

func (s *Store) writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0644)
}

func (s *Store) readJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

func (s *Store) saveState(name string, st *InstanceState) error {
	return s.writeJSON(filepath.Join(s.dir(name), fState), st)
}

func (s *Store) loadState(name string) (*InstanceState, error) {
	if err := validName(name); err != nil {
		return nil, err
	}
	var st InstanceState
	if err := s.readJSON(filepath.Join(s.dir(name), fState), &st); err != nil {
		return nil, err
	}
	return &st, nil
}

func (s *Store) loadSpec(name string) (*contract.RunSpec, error) {
	var spec contract.RunSpec
	if err := s.readJSON(filepath.Join(s.dir(name), fSpec), &spec); err != nil {
		return nil, err
	}
	return &spec, nil
}

// lock takes an exclusive flock on <dir>/lock. This is the minictd-level
// fix for the flat-file TOCTOU window in the PoC state dir: every mutating
// op holds it for the whole critical section. Best-effort 10s deadline.
// The instance dir must already exist (Stop/Start/Remove treat a missing
// dir as not-found); Run uses lockCreate.
func (s *Store) lock(ctx context.Context, name string) (func(), error) {
	if err := validName(name); err != nil {
		return nil, err
	}
	if _, err := os.Stat(s.dir(name)); err != nil {
		return nil, err
	}
	return s.lockCreate(ctx, name)
}

// lockCreate creates the instance dir if needed, then locks.
func (s *Store) lockCreate(ctx context.Context, name string) (func(), error) {
	if err := validName(name); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(s.dir(name), 0755); err != nil {
		return nil, err
	}
	return s.lockCreateLocked(ctx, name)
}

func (s *Store) lockCreateLocked(ctx context.Context, name string) (func(), error) {
	path := filepath.Join(s.dir(name), fLock)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("open lock: %w", err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return func() {
				_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
				_ = f.Close()
			}, nil
		}
		if ctx.Err() != nil {
			_ = f.Close()
			return nil, ctx.Err()
		}
		if time.Now().After(deadline) {
			_ = f.Close()
			return nil, fmt.Errorf("timed out waiting for instance lock %q", name)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// pidAlive reports whether the given host pid still belongs to the payload
// we spawned: it must exist, not be a zombie, and its /proc/<pid>/stat
// start time (field 22, ticks since boot) must match what we recorded —
// which defeats pid reuse after a crash.
func (s *Store) pidAlive(pid int, startTicks uint64) bool {
	if pid <= 0 {
		return false
	}
	if err := syscall.Kill(pid, 0); err != nil {
		return false
	}
	state, ticks, ok := procStat(pid)
	if !ok {
		return false
	}
	if state == 'Z' {
		return false
	}
	if startTicks != 0 && ticks != 0 && ticks != startTicks {
		return false
	}
	return true
}

// alive resolves liveness from the persisted state and self-heals
// state.json when the payload died on its own (kernel OOM-kill, payload
// exit, host reboot). Returns the (possibly healed) state.
func (s *Store) alive(st *InstanceState) *InstanceState {
	if st.State == StateRunning && !s.pidAlive(st.PID, st.StartTicks) {
		st.State = StateStopped
		st.PID = 0
		st.StartTicks = 0
		if !st.Exited {
			st.Exited = true
			st.ExitCode = 0 // cause unknown from outside; reaper refines when it saw the wait
		}
		// CG1/S2 deliberate NON-cleanup (SIA medium finding REASSESSED):
		// the heal path must NOT rmdir the instance cgroup — an exited
		// instance keeps its cgroup for post-mortem counter reads
		// (memory.peak/memory.events, docker keeps the exited container's
		// cgroup until rm), and the CI suite reads those counters AFTER the
		// exit (measured: a heal-time rmdir erased the evidence between the
		// exit and the read — run 44711850479). Cleanup stays on the two
		// paths that release the instance identity: Stop (CgroupPath=“”) and
		// Remove (dir unlink); an empty cgroup dir holds no processes and
		// costs nothing until then.
		_ = s.saveState(st.Name, st)
	}
	return st
}

// webportsOf extracts the preview ports from the reserved label, falling
// back to none. Label value: "3000" or "3000,5173".
func webportsOf(spec *contract.RunSpec) ([]int, error) {
	labels := parseLabels(spec.Labels)
	raw, ok := labels[contract.LabelMinictWebports]
	if !ok || strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var ports []int
	for _, f := range strings.Split(raw, ",") {
		f = strings.TrimSpace(f)
		var p int
		if _, err := fmt.Sscanf(f, "%d", &p); err != nil || p < 1024 || p > 65535 {
			return nil, fmt.Errorf("label %s: bad port %q (need 1024..65535)",
				contract.LabelMinictWebports, f)
		}
		ports = append(ports, p)
	}
	return ports, nil
}

// ErrPortInUse is returned when a Run/Start declares a webport already held
// by another running instance (S6, issue #124, D55). The API maps it to 409
// Conflict (parity ErrNotRunning) so the operator gets a clear error instead
// of a silently broken preview (the second webproxy would EADDRINUSE and only
// a Note would record it).
var ErrPortInUse = errors.New("webport already in use by another running instance")

// portsHeldBy returns the set of preview ports currently held by OTHER
// running instances (port -> instance name). `except` is the instance being
// Run/Start'd (so its own stale state doesn't false-positive on Start).
// Stopped/dead instances don't hold ports (Stop/Remove kill the webproxies;
// alive() heals a dead-pid running state to stopped).
func (s *Store) portsHeldBy(except string) (map[int]string, error) {
	held := map[int]string{}
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
		for _, p := range st.Ports {
			held[p] = n
		}
	}
	return held, nil
}

// ErrInstanceRunning is returned when an op requires a STOPPED instance but
// the target is running (e.g. save-template, GĐ2 mục 2 issue #127, D56). Mapped
// to 409 Conflict — the operator must Stop first (fail-closed: a live rootfs
// copy races payload writes).
var ErrInstanceRunning = errors.New("instance is running, stop it first")

// templatesDir returns the template store directory (<Root>/_templates/).
// Template dirs are skipped by ListByNamePrefix (nameRe rejects the "_"
// prefix), so they don't pollute the instance list.
func (s *Store) templatesDir() string {
	return filepath.Join(s.Root, "_templates")
}

// backupsDir returns the snapshot/backup store directory (<Root>/_backups/).
// Skipped by ListByNamePrefix (nameRe rejects the "_" prefix, Phiên 21).
func (s *Store) backupsDir() string {
	return filepath.Join(s.Root, "_backups")
}

func parseLabels(pairs []string) map[string]string {
	m := make(map[string]string)
	for _, kv := range pairs {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		m[k] = v
	}
	return m
}

// parseDockerSize converts a docker-style size string ("10240m", "10g",
// "65536", "512mb") to bytes. Docker uses powers of 1024 for k/m/g.
func parseDockerSize(s string) (int64, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return 0, fmt.Errorf("empty size")
	}
	mult := int64(1)
	num := s
	switch {
	case strings.HasSuffix(s, "kb"), strings.HasSuffix(s, "k"):
		mult, num = 1024, strings.TrimSuffix(strings.TrimSuffix(s, "kb"), "k")
	case strings.HasSuffix(s, "mb"), strings.HasSuffix(s, "m"):
		mult, num = 1024*1024, strings.TrimSuffix(strings.TrimSuffix(s, "mb"), "m")
	case strings.HasSuffix(s, "gb"), strings.HasSuffix(s, "g"):
		mult, num = 1024*1024*1024, strings.TrimSuffix(strings.TrimSuffix(s, "gb"), "g")
	case strings.HasSuffix(s, "b"):
		num = strings.TrimSuffix(s, "b")
	}
	var v int64
	if _, err := fmt.Sscanf(strings.TrimSpace(num), "%d", &v); err != nil {
		return 0, fmt.Errorf("bad size %q", s)
	}
	return v * mult, nil
}

// renderSpec turns the RunSpec into MINICT_SPEC lines (the D23 file the
// patched minict.c parses host-side before unshare). Every directive it
// emits is validated here too — the C parser is the second, fail-closed
// gate. Anything minict cannot honour becomes a Note, never a silent lie.
//
// cgAvail tells whether cgroups v2 enforcement is possible (CG1/S2,
// issue #4): when true the resource-limit notes record the cgroup mapping
// applied after spawn; when false they keep the pre-CG1 fallback wording.
//
// ovl != nil emits the overlayfs directive (D66, issue #162) and re-validates
// the paths (the C parser re-checks too — twin gates, never trust one side).
func renderSpec(spec *contract.RunSpec, ports []int, cgAvail bool, ovl *overlayConfig) (lines, notes []string, err error) {
	addNote := func(f string, a ...any) { notes = append(notes, fmt.Sprintf(f, a...)) }

	// --- hostname ---
	if spec.Hostname != "" {
		if len(spec.Hostname) > 64 || strings.ContainsAny(spec.Hostname, " \t\n\r") {
			return nil, nil, fmt.Errorf("invalid hostname %q", spec.Hostname)
		}
		lines = append(lines, "hostname "+spec.Hostname)
	}

	// --- env: KEY=VALUE per line, cap 64 (patch SPEC_MAX_ENTRIES) ---
	if len(spec.Env) > 64 {
		return nil, nil, fmt.Errorf("spec.Env: %d entries exceeds the MINICT_SPEC cap of 64", len(spec.Env))
	}
	for _, e := range spec.Env {
		k, v, ok := strings.Cut(e, "=")
		if !ok || k == "" {
			return nil, nil, fmt.Errorf("spec.Env entry %q is not KEY=VALUE", e)
		}
		if !envKeyRe.MatchString(k) {
			return nil, nil, fmt.Errorf("spec.Env key %q must match [A-Za-z_][A-Za-z0-9_]*", k)
		}
		if len(e) > 1024 {
			return nil, nil, fmt.Errorf("spec.Env entry %q exceeds the 1024B line cap", k)
		}
		lines = append(lines, "env "+k+"="+v)
	}

	// --- volumes -> bind (docker -v src:dst[:ro]; named volumes rejected) ---
	for _, v := range spec.Volumes {
		parts := strings.SplitN(v, ":", 3)
		if len(parts) < 2 {
			return nil, nil, fmt.Errorf("spec.Volume %q: need <host-src>:<ct-dst>[:ro]", v)
		}
		src, dst := parts[0], parts[1]
		ro := false
		if len(parts) == 3 {
			if parts[2] == "ro" {
				ro = true
			} else if parts[2] != "rw" && parts[2] != "" {
				return nil, nil, fmt.Errorf("spec.Volume %q: only :ro/:rw opts supported", v)
			}
		}
		if !strings.HasPrefix(src, "/") {
			return nil, nil, fmt.Errorf("spec.Volume %q: named volumes are not supported; use an absolute host path", v)
		}
		if !strings.HasPrefix(dst, "/") || strings.Contains(dst, "..") {
			return nil, nil, fmt.Errorf("spec.Volume %q: container path must be absolute without '..'", v)
		}
		if len(src) > 512 || len(dst) > 512 {
			return nil, nil, fmt.Errorf("spec.Volume %q: path exceeds the 512B field cap", v)
		}
		line := "bind " + src + ":" + dst
		if ro {
			line += ":ro"
		}
		lines = append(lines, line)
	}

	// --- tmpfs: docker --tmpfs /path[:size=N] ---
	for _, t := range spec.Tmpfs {
		path := t
		size := ""
		if before, after, ok := strings.Cut(t, ":"); ok {
			path = before
			opts := strings.Split(after, ",")
			for _, o := range opts {
				switch {
				case strings.HasPrefix(o, "size="):
					size = strings.TrimPrefix(o, "size=")
				case o == "ro" || o == "rw" || o == "":
					// accepted silently; :ro on tmpfs is rare and not
					// worth a hard error
				default:
					addNote("tmpfs %q: option %q ignored (only size= is supported)", path, o)
				}
			}
		}
		if !strings.HasPrefix(path, "/") || strings.Contains(path, "..") {
			return nil, nil, fmt.Errorf("spec.Tmpfs %q: path must be absolute without '..'", t)
		}
		if size != "" {
			if _, err := parseDockerSize(size); err != nil {
				return nil, nil, fmt.Errorf("spec.Tmpfs %q: %v", t, err)
			}
			lines = append(lines, "tmpfs "+path+":size="+size)
		} else {
			lines = append(lines, "tmpfs "+path)
		}
	}

	// --- shm: docker --shm-size -> shm <size> (same K/M/G grammar) ---
	if spec.ShmSize != "" {
		if _, err := parseDockerSize(spec.ShmSize); err != nil {
			return nil, nil, fmt.Errorf("spec.ShmSize %q: %v", spec.ShmSize, err)
		}
		lines = append(lines, "shm "+spec.ShmSize)
	}

	// --- ulimits: only nofile/nproc are understood by the C parser ---
	sawNproc := false
	for _, u := range spec.Ulimits {
		name, _, ok := strings.Cut(u, "=")
		if !ok {
			return nil, nil, fmt.Errorf("spec.Ulimit %q: need <name>=<soft>[:<hard>]", u)
		}
		switch name {
		case "nofile", "nproc":
			if name == "nproc" {
				sawNproc = true
			}
			lines = append(lines, "ulimit "+u)
		default:
			addNote("ulimit %q skipped: MINICT_SPEC supports nofile and nproc only", u)
		}
	}

	// --- seccomp allowlist (D59, issue #139): embed the profile's syscall
	// numbers as seccomp_allow chunks; the C side installs the D11 blocklist
	// floor + allowlist + default EPERM in both the payload child and the
	// setns exec child (MD0 parity). Unknown profile = hard error, never a
	// silent fallback to blocklist-only.
	if spec.SeccompProfile != "" {
		nrs, err := LoadSeccompProfile(spec.SeccompProfile)
		if err != nil {
			return nil, nil, err
		}
		lines = append(lines, seccompAllowLines(nrs)...)
		addNote("seccomp allowlist %q: %d syscalls (D11 blocklist stays a floor; trusted workloads only, manifest §3)", spec.SeccompProfile, len(nrs))
	}

	// --- PidsLimit: cgroups v2 pids.max when available (CG1/S2), rlimit
	// nproc approximation otherwise ---
	// S2 measured on the runner (CI 44700961938): a shared per-uid nproc
	// of 16 starves the shim itself (the runner user owns many unrelated
	// processes), killing the payload before it starts — exactly the gap
	// S2 documents. With cgroups v2 available the scoped pids.max is the
	// only pid bound; the shared nproc rlimit is dropped.
	if spec.PidsLimit > 0 {
		if cgAvail {
			addNote("PidsLimit=%d enforced via cgroups v2 pids.max (CG1/S2 — shared per-uid rlimit nproc dropped)", spec.PidsLimit)
		} else if !sawNproc {
			v := fmt.Sprintf("%d", spec.PidsLimit)
			lines = append(lines, "ulimit nproc="+v+":"+v)
			addNote("PidsLimit=%d mapped to rlimit nproc (per-uid approximation, not cgroups pids.max)", spec.PidsLimit)
		}
	}

	// --- preview relays: one webport directive per port ---
	for _, p := range ports {
		lines = append(lines, fmt.Sprintf("webport %d /.minict-proxy-%d.sock", p, p))
	}

	// --- D66 (issue #162): overlayfs rootfs, env-gated MINICT_OVERLAY=1 ---
	if ovl != nil {
		if err := validateOverlay(ovl); err != nil {
			return nil, nil, err
		}
		lines = append(lines, ovl.specLine())
		addNote("overlayfs rootfs (D66): lower=%s (shared, read-only via copy-up) upper=%s — no private copy", ovl.Lower, ovl.Upper)
	}

	// --- documented gaps: recorded as notes, never enforced ---
	if spec.Network != "" {
		addNote("Network=%q ignored: minict netns is loopback-only (D17) — no external network either way", spec.Network)
	}
	if spec.Userns != "" {
		addNote("Userns=%q ignored: minict ALWAYS runs inside an unprivileged user namespace", spec.Userns)
	}
	if spec.Runtime != "" {
		addNote("Runtime=%q ignored: minict is its own runtime (gVisor/runc not applicable)", spec.Runtime)
	}
	if spec.ReadOnly {
		lines = append(lines, "readonly")
		addNote("ReadOnly=true enforced via Landlock ro-root (GĐ2 mục 6, D61; kernel >=5.13 — <5.13 warn fail-soft, pivot_root remains primary)")
	}
	for _, c := range spec.CapDrop {
		if c != "ALL" {
			addNote("CapDrop=%q is moot: minict always drops ALL caps in its own userns", c)
		}
	}
	for _, so := range spec.SecurityOpt {
		if !strings.EqualFold(so, "no-new-privileges") && !strings.EqualFold(so, "no-new-privileges:true") {
			addNote("SecurityOpt=%q ignored: minict always sets PR_SET_NO_NEW_PRIVS", so)
		}
	}
	if spec.CPUShares > 0 {
		if cgAvail {
			addNote("CPUShares=%d enforced via cgroups v2 cpu.weight=%d (CG1/S2)", spec.CPUShares, sharesToWeight(spec.CPUShares))
		} else {
			addNote("CPUShares=%d not enforced (no cgroups in this build, CG1)", spec.CPUShares)
		}
	}
	if spec.CPUs != "" {
		if cgAvail {
			addNote("CPUs=%q enforced via cgroups v2 cpu.max (quota x %d period, CG1/S2)", spec.CPUs, cgroupCPUPeriod)
		} else {
			addNote("CPUs=%q not enforced (no cgroups in this build, CG1)", spec.CPUs)
		}
	}
	if spec.Memory != "" || spec.MemorySwap != "" {
		if cgAvail {
			addNote("Memory=%q enforced via cgroups v2 memory.max (CG1/S2)", spec.Memory)
		} else {
			addNote("Memory=%q MemorySwap=%q not enforced (no cgroups in this build, CG1)", spec.Memory, spec.MemorySwap)
		}
	}
	if spec.MemorySwap != "" && cgAvail {
		addNote("MemorySwap=%q not mapped (v2 memory.swap.max is a separate knob; memory.max is set, CG1/S2)", spec.MemorySwap)
	}
	if spec.User != "" {
		addNote("User=%q ignored: minict runs the payload as the userns-mapped root uid", spec.User)
	}
	if spec.Restart != "" {
		addNote("Restart=%q ignored: minictd does not auto-restart payloads", spec.Restart)
	}
	return lines, notes, nil
}

// payloadArgs returns the minict argv for the payload. Empty Cmd falls back
// to the same PID-1 sleep the PoC CLI uses (documented default, not magic:
// the control plane always passes a command for real sandboxes).
func payloadArgs(spec *contract.RunSpec) []string {
	if len(spec.Cmd) > 0 {
		return append([]string(nil), spec.Cmd...)
	}
	return []string{"/bin/sh", "-c", "sleep 31536000"}
}

// spawnPayload starts the minict runtime binary for the instance. The
// process is reaped by a goroutine (prevents zombies); liveness of the
// recorded pid is derived from /proc with pid-reuse protection, so the
// reaper goroutine only logs — it never writes state.json concurrently.
func (s *Store) spawnPayload(name string, dir string, spec *contract.RunSpec, cgDir string) (pid int, startTicks uint64, err error) {
	args := append([]string{s.MinictBin, filepath.Join(dir, "rootfs")}, payloadArgs(spec)...)
	var cmd *exec.Cmd
	if cgDir != "" {
		// CG1/S2 migrate-before-exec (D48): a plain sh launcher writes its
		// own pid into the instance cgroup and execs the runtime, so the
		// shim AND its payload are born inside the cgroup (post-spawn
		// migration races minict's payload fork — measured on CI runs
		// 44702053556/44703016076). exec keeps the pid, so state.json
		// records the same host pid either way.
		launcher := `cgdir="$1"; shift
echo "$$" > "$cgdir/cgroup.procs" || exit 125
exec "$@"`
		launchArgs := append([]string{"sh", "-c", launcher, "sh", cgDir}, args...)
		cmd = exec.Command(launchArgs[0], launchArgs[1:]...)
	} else {
		cmd = exec.Command(args[0], args[1:]...)
	}
	cmd.Env = append(childEnv(), "MINICT_SPEC="+filepath.Join(dir, fMspec))
	logf, err := os.OpenFile(filepath.Join(dir, fLog), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return 0, 0, fmt.Errorf("open payload log: %w", err)
	}
	defer logf.Close()
	cmd.Stdout = logf
	cmd.Stderr = logf
	cmd.Stdin = nil // /dev/null semantics
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return 0, 0, fmt.Errorf("start minict payload: %w", err)
	}
	pid = cmd.Process.Pid
	if _, ticks, ok := procStat(pid); ok {
		startTicks = ticks
	}
	go func() {
		err := cmd.Wait()
		if err != nil {
			s.Log.Warn("payload exited", "name", name, "pid", pid, "err", err.Error())
		} else {
			s.Log.Info("payload exited", "name", name, "pid", pid)
		}
	}()
	return pid, startTicks, nil
}

// spawnProxies starts one host-side webproxy.py per port and waits until
// the bridge answers (same 2.5s probe loop as minict.sh web).
func (s *Store) spawnProxies(name string, dir string, ports []int, wait time.Duration, overlay bool) (map[int]int, []string) {
	var notes []string
	pids := make(map[int]int)
	if len(ports) == 0 {
		return pids, notes
	}
	if s.WebProxyPy == "" || s.Python == "" {
		notes = append(notes, "preview ports requested but --webproxy/--python not configured; relays not started")
		return pids, notes
	}
	rootfs := filepath.Join(dir, "rootfs")
	if overlay {
		// D66 (issue #162): the shim's relay socket is created INSIDE the
		// sandbox after pivot — with an overlay rootfs it copy-ups into
		// UPPER, not the empty mountpoint host-side webproxy would find
		// under rootfs/. Same symlink-proof openat semantics, different
		// host directory.
		rootfs = filepath.Join(dir, "upper")
	}
	for _, p := range ports {
		// S1 (issue #41, D31): pass --root/--sock so webproxy resolves the
		// relay socket symlink-proof (openat O_NOFOLLOW chain + connect via
		// /proc/self/fd) instead of following payload-controlled symlinks.
		sockInRoot := fmt.Sprintf("/.minict-proxy-%d.sock", p)
		cmd := exec.Command(s.Python, s.WebProxyPy, "--root", rootfs,
			"--sock", sockInRoot, "--port", fmt.Sprintf("%d", p))
		cmd.Stdin = nil
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		logf, err := os.OpenFile(filepath.Join(dir, fmt.Sprintf("proxy-%d.log", p)), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			notes = append(notes, fmt.Sprintf("proxy %d: open log: %v", p, err))
			continue
		}
		cmd.Stdout, cmd.Stderr = logf, logf
		if err := cmd.Start(); err != nil {
			logf.Close()
			notes = append(notes, fmt.Sprintf("proxy %d: start: %v", p, err))
			continue
		}
		pid := cmd.Process.Pid
		pids[p] = pid
		go func(port, proxyPID int, lf *os.File) {
			defer lf.Close()
			if err := cmd.Wait(); err != nil {
				s.Log.Warn("webproxy exited", "name", name, "port", port, "pid", proxyPID, "err", fmt.Sprint(err))
			}
		}(p, pid, logf)
		notes = append(notes, fmt.Sprintf("preview port %d via unix-socket bridge (pid %d)", p, pid))
	}
	// TCP readiness probe (best-effort, minict.sh parity).
	deadline := time.Now().Add(wait)
	for _, p := range ports {
		if _, ok := pids[p]; !ok {
			continue
		}
		ready := false
		for time.Now().Before(deadline) {
			c, err := dialTimeout(fmt.Sprintf("127.0.0.1:%d", p), 500*time.Millisecond)
			if err == nil {
				_ = c.Close()
				ready = true
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		if !ready {
			notes = append(notes, fmt.Sprintf("preview port %d not TCP-ready within %s (check %s)", p, wait, filepath.Join(dir, fmt.Sprintf("proxy-%d.log", p))))
		}
	}
	return pids, notes
}

// killTreeBestEffort SIGTERMs then SIGKILLs a pid (the payload shim kills
// its own process group via PDEATHSIG + group-kill teardown; the daemon
// only needs to stop the shim and the host-side webproxies).
func (s *Store) killBestEffort(pid int, grace time.Duration) {
	if pid <= 0 {
		return
	}
	_ = syscall.Kill(pid, syscall.SIGTERM)
	deadline := time.Now().Add(grace)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(pid, 0); err != nil {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	_ = syscall.Kill(pid, syscall.SIGKILL)
}

// childEnv is the COMPLETE environment handed to minict / minict-exec.
// minict.c only overrides PATH/HOME/PS1/MINICT before exec'ing the
// payload and inherits everything else, so passing os.Environ() leaked the
// daemon's own environment (MINICTD_TOKEN, SSH_AUTH_SOCK, cloud creds...)
// into every sandbox and every exec (review finding R2, 2026-11-18).
// Payload env comes ONLY from RunSpec.Env / ExecV2Request.Env.
func childEnv() []string {
	return []string{"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"}
}

// ReconcileAll proactively runs the lazy liveness check (alive) over every
// instance dir: a state=running instance whose payload died outside a query
// (kernel OOM-kill, payload exit, operator kill -9) is healed to stopped
// right here instead of on the next Inspect. A3 (issue #91, D47). Never
// spawns anything — Start/autostart are explicit daemon decisions; broken
// state files are skipped (Inspect surfaces them per instance later).
//
// SIA PR #92 (medium): the whole heal holds the same per-instance flock as
// Run/Start/Stop/Remove. Without it, a concurrent Start could spawn a fresh
// payload while reconcile read the stale state.json and our saveState(stopped)
// would overwrite the just-written running state — leaving the daemon with a
// wrong liveness view (exec/stop 409) and a possible second spawn from
// autostart. A busy lock means an op is in flight: skip + warn, the next
// tick re-checks.
func (s *Store) ReconcileAll() ([]string, error) {
	names, err := s.ListByNamePrefix(context.Background(), "")
	if err != nil {
		return nil, err
	}
	var healed []string
	for _, name := range names {
		lctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
		unlock, err := s.lock(lctx, name)
		cancel()
		if err != nil {
			s.Log.Warn("minictd reconcile: lock busy, skipping", "name", name)
			continue
		}
		st, err := s.loadState(name)
		if err != nil {
			unlock()
			s.Log.Warn("minictd reconcile: unreadable state, skipping", "name", name, "err", err.Error())
			continue
		}
		if st.State == StateRunning && !s.pidAlive(st.PID, st.StartTicks) {
			s.alive(st)
			healed = append(healed, name)
		}
		unlock()
	}
	// G3 (issue #97): a healed instance must lose its route; rebuild the
	// whole route dir from current reality (idempotent, cheap).
	if err := s.SyncAllRoutes(); err != nil {
		s.Log.Warn("minictd routes: sync failed", "err", err.Error())
	}
	return healed, nil
}

// AutostartNames lists instances that were running when the daemon went
// away (state=running in state.json): the reboot / daemon-restart recovery
// set for --autostart (issue #91, D47). Includes dead-but-marked-running
// instances on purpose: Start() heals then spawns from spec.json
// (wake-recreate). Instances stopped by the operator stay stopped — a
// policy choice recorded in D47, matching docker's --restart=unless-stopped
// semantics observed from the daemon's point of view.
func (s *Store) AutostartNames() ([]string, error) {
	names, err := s.ListByNamePrefix(context.Background(), "")
	if err != nil {
		return nil, err
	}
	var want []string
	for _, name := range names {
		st, err := s.loadState(name)
		if err != nil {
			continue
		}
		if st.State == StateRunning {
			want = append(want, name)
		}
	}
	return want, nil
}
