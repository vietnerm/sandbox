package minict

// CG1/S2 (issue #4) — cgroups v2 rootless resource enforcement.
//
// Design (researched 2026-11-20, sources in journal/2026-11-20/research.md
// Phiên 13):
//
//   - systemd.io/CGROUP_DELEGATION: on a delegated subtree ("islands"
//     model) the daemon must enable controllers itself by writing
//     "+memory"/"+cpu"/"+pids" into cgroup.subtree_control — delegation
//     only EXPOSES the controllers, it never enables them.
//   - kernel cgroup-v2.rst "No Internal Process Constraint": a domain
//     cgroup holding processes cannot have domain controllers enabled on
//     its own subtree_control, so the daemon first moves itself into a
//     dedicated sub-cgroup (minictd-daemon/) and creates instances under
//     a process-free sub-tree (minictd-apps/).
//   - kernel cgroup-v2.rst "Delegation Containment": a delegatee can only
//     migrate processes whose source/destination common ancestor it can
//     write. Everything minictd spawns lives inside its own cgroup, so
//     moving the payload shim into a sibling instance cgroup is allowed —
//     pulling in a process from OUTSIDE the subtree is not. The probe
//     therefore refuses any base the daemon does not already sit in.
//
// Fail-soft contract (decided in research.md): when no writable v2 base
// exists (cgroups v1 dev host, no systemd delegation, no bootstrap), every
// op degrades to the pre-CG1 behaviour — Notes + rlimit nproc — and the
// reason is recorded as a note. Enforcement is strictly additive: the
// 124-case baseline must stay green on hosts without v2.
//
// The payload shim is migrated AFTER spawn (child inherits the daemon's
// cgroup at fork; writing its pid into the instance cgroup.procs is a
// plain sibling move). Stats prefer cgroup accounting (memory.current,
// pids.current, cpu.stat) over the /proc RSS tree when enforcement is on.

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/vietnerm/sandbox/minictd/contract"
)

const (
	// cgroupDaemonDir is where the daemon parks itself so minictd-apps/ can
	// carry domain controllers without violating no-internal-process.
	cgroupDaemonDir = "minictd-daemon"
	// cgroupAppsDir holds one sub-cgroup per sandbox instance.
	cgroupAppsDir = "minictd-apps"
	// cpu period used for cpu.max when a quota is set (100ms, docker parity).
	cgroupCPUPeriod = 100000
)

// CgroupLimits is the v2 file mapping of the RunSpec resource fields.
// Empty string = file not written (controller untouched).
type CgroupLimits struct {
	MemoryMax string // memory.max: bytes or "max"
	CPUMax    string // cpu.max:   "<quota_us> 100000" or "max 100000"
	CPUWeight string // cpu.weight: 1..10000 (docker shares mapped)
	PidsMax   string // pids.max:  n or "max"
}

// cgroupManager owns the daemon's delegated v2 subtree. One per Store,
// resolved lazily and cached: after the self-move the daemon's
// /proc/self/cgroup path changes, so re-resolving would find a different
// base.
type cgroupManager struct {
	Base    string   // delegated base dir (daemon's own cgroup)
	Apps    string   // Base/minictd-apps (process-free container tree)
	Avail   bool     // true when enforcement is possible
	Enabled []string // controllers actually enabled on Base
	reason  string   // when !Avail, the measured why (goes into notes)
	once    sync.Once
}

// cgroupSelfPath reads the v2 unified entry of /proc/self/cgroup.
// Returns "" when there is none (cgroups v1 host).
func cgroupSelfPath() string {
	b, err := os.ReadFile("/proc/self/cgroup")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(b), "\n") {
		// v2 entry is the only "0::<path>" line.
		if rest, ok := strings.CutPrefix(line, "0::"); ok {
			return strings.TrimSpace(rest)
		}
	}
	return ""
}

// cgResolveBase picks the enforcement base. Pure over (override,
// selfPath) so unit tests can inject a fake self path.
//
// Rules (fail-closed with a measured reason):
//   - override wins when set; it must contain a cgroup.controllers file
//     (v2 marker) and the daemon's own cgroup must already sit inside it
//     (Delegation Containment — a base outside the daemon's subtree
//     would let us create cgroups but never migrate the shim into them).
//   - without override the base is the daemon's own v2 cgroup dir, and
//     only when it is not the hierarchy root: a root-sitting daemon has
//     no delegated subtree to write into at all (the runner case — the
//     suite bootstraps one with sudo and passes it as the override).
func cgResolveBase(override, selfPath string) (base string, ok bool, reason string) {
	if override != "" {
		if _, err := os.Stat(filepath.Join(override, "cgroup.controllers")); err != nil {
			return "", false, fmt.Sprintf("override %q is not a cgroup v2 dir (no cgroup.controllers): %v", override, err)
		}
		if selfPath == "" {
			return "", false, "no v2 entry in /proc/self/cgroup while an override is set"
		}
		if !sameOrInside(selfPath, overrideRel(override)) {
			return "", false, fmt.Sprintf("daemon cgroup %q is not inside override %q — shim migration would violate delegation containment", selfPath, override)
		}
		return override, true, ""
	}
	if selfPath == "" {
		return "", false, "no cgroup v2 unified entry in /proc/self/cgroup (cgroups v1 host)"
	}
	if selfPath == "/" {
		return "", false, "daemon sits at the v2 hierarchy root without a delegated subtree (no systemd delegation; bootstrap with MINICTD_CGROUP_ROOT)"
	}
	return filepath.Join("/sys/fs/cgroup", selfPath), true, ""
}

// overrideRel normalises an override path to the /proc/self/cgroup shape
// ("/sys/fs/cgroup" prefix stripped, "/"-relative).
func overrideRel(p string) string {
	return strings.TrimPrefix(p, "/sys/fs/cgroup")
}

// sameOrInside reports whether cgroup path self (from /proc/self/cgroup,
// "/"-relative) equals rel or lies below it.
func sameOrInside(self, rel string) bool {
	if rel == "" || rel == "/" {
		return self == "/" || strings.HasPrefix(self, "/")
	}
	rel = strings.TrimSuffix(rel, "/")
	return self == rel || strings.HasPrefix(self, rel+"/")
}

// cgroupManager returns the Store's cached manager, running cgEnsureApps
// exactly once. Never fails: !Avail carries the reason.
func (s *Store) cgroupManager() *cgroupManager {
	if s.cg == nil {
		s.cgMu.Lock()
		if s.cg == nil {
			s.cg = &cgroupManager{}
		}
		s.cgMu.Unlock()
	}
	m := s.cg
	m.once.Do(func() {
		selfPath := cgroupSelfPath()
		base, ok, reason := cgResolveBase(s.CgroupRoot, selfPath)
		if !ok {
			m.reason = reason
			return
		}
		m.Base = base
		if err := m.ensureApps(os.Getpid()); err != nil {
			m.reason = err.Error()
			m.Base, m.Apps = "", ""
			return
		}
		m.Avail = true
	})
	return m
}

// ensureApps builds the process-free instance tree under base and enables
// the controllers the spec needs. Pure filesystem work against the (real
// or fake) v2 files, so unit tests can drive it over a temp dir.
//
// Steps:
//  1. mkdir base/minictd-daemon and, when the daemon pid still sits in
//     base itself, migrate it there (no-internal-process).
//  2. mkdir base/minictd-apps.
//  3. Enable every available controller of interest on base by writing
//     "+name" into base/cgroup.subtree_control (systemd.io: delegation
//     exposes controllers, the delegatee enables them).
func (m *cgroupManager) ensureApps(selfPID int) error {
	daemonDir := filepath.Join(m.Base, cgroupDaemonDir)
	if err := os.MkdirAll(daemonDir, 0755); err != nil {
		return fmt.Errorf("mkdir %s: %w", daemonDir, err)
	}
	// Self-migrate only when the daemon still sits directly in base.
	// After the first call the daemon lives in minictd-daemon/ and this
	// is a no-op (re-entrant: daemon restarts re-run the same check).
	inBase, err := procsContain(filepath.Join(m.Base, "cgroup.procs"), selfPID)
	if err != nil {
		return fmt.Errorf("read %s cgroup.procs: %w", m.Base, err)
	}
	if inBase {
		if err := writeCgroupFile(filepath.Join(daemonDir, "cgroup.procs"), strconv.Itoa(selfPID)); err != nil {
			return fmt.Errorf("self-migrate daemon pid into %s: %w", daemonDir, err)
		}
	} else {
		// Re-entrant path: confirm the daemon really is parked under the
		// subtree instead of somewhere else entirely.
		parked, err := procsContain(filepath.Join(daemonDir, "cgroup.procs"), selfPID)
		if err != nil || !parked {
			return fmt.Errorf("daemon pid %d is not inside base %s (delegation containment blocks shim migration)", selfPID, m.Base)
		}
	}

	apps := filepath.Join(m.Base, cgroupAppsDir)
	if err := os.MkdirAll(apps, 0755); err != nil {
		return fmt.Errorf("mkdir %s: %w", apps, err)
	}
	m.Apps = apps

	ctrlRaw, err := os.ReadFile(filepath.Join(m.Base, "cgroup.controllers"))
	if err != nil {
		return fmt.Errorf("read %s cgroup.controllers: %w", m.Base, err)
	}
	have := map[string]bool{}
	for _, c := range strings.Fields(string(ctrlRaw)) {
		have[c] = true
	}
	for _, want := range []string{"memory", "cpu", "pids"} {
		if !have[want] {
			continue
		}
		if err := writeCgroupFile(filepath.Join(m.Base, "cgroup.subtree_control"), "+"+want); err != nil {
			return fmt.Errorf("enable controller %s on %s: %w", want, m.Base, err)
		}
		m.Enabled = append(m.Enabled, want)
	}
	// Second hop (measured on the CI runner, run 44702053556): interface
	// files like memory.max appear in a cgroup only when the controller is
	// enabled on ITS PARENT's subtree_control. Enabling on base alone gives
	// minictd-apps the files; the per-instance sandbox-* cgroups need the
	// same set enabled on minictd-apps/cgroup.subtree_control. apps holds
	// no processes directly, so no-internal-process is satisfied there.
	for _, want := range m.Enabled {
		if err := writeCgroupFile(filepath.Join(m.Apps, "cgroup.subtree_control"), "+"+want); err != nil {
			return fmt.Errorf("enable controller %s on %s: %w", want, m.Apps, err)
		}
	}
	return nil
}

// procsContain reports whether the given cgroup.procs file holds pid.
func procsContain(path string, pid int) (bool, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	for _, f := range strings.Fields(string(b)) {
		if f == strconv.Itoa(pid) {
			return true, nil
		}
	}
	return false, nil
}

// writeCgroupFile writes one trimmed line to a cgroup interface file.
// Append mode: on the real kernel every write to an interface file is a
// standalone command (enable controller, migrate pid, set value) and the
// kernel authors the file content; with O_APPEND a FAKE cgroup root (unit
// tests) keeps the history of those commands instead of truncating on
// each write. Single writes are unaffected either way.
func writeCgroupFile(path, line string) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(strings.TrimSpace(line) + "\n")
	return err
}

// sharesToWeight maps docker CPU shares onto the v2 cpu.weight scale
// (linear: 1024 docker default == 100 v2 default), clamped to [1, 10000].
func sharesToWeight(shares int) int {
	w := shares * 100 / 1024
	if w < 1 {
		return 1
	}
	if w > 10000 {
		return 10000
	}
	return w
}

// limitsFromSpec maps the docker-shaped RunSpec fields onto cgroup v2
// files. Mapping (documented in journal decisions/cgroups-v2.md):
//
//	Memory     -> memory.max (parseDockerSize bytes)
//	CPUs "1.5" -> cpu.max "150000 100000" (quota_us = CPUs * 100000)
//	CPUShares  -> cpu.weight = sharesToWeight(shares)
//	PidsLimit  -> pids.max
//
// MemorySwap deliberately maps to nothing (v2 memory.swap.max is a
// different knob; noted in renderSpec instead of silently ignored).
func limitsFromSpec(spec *contract.RunSpec) CgroupLimits {
	var l CgroupLimits
	if spec.Memory != "" {
		if b, err := parseDockerSize(spec.Memory); err == nil && b > 0 {
			l.MemoryMax = strconv.FormatInt(b, 10)
		}
	}
	if spec.CPUs != "" {
		if f, err := strconv.ParseFloat(strings.TrimSpace(spec.CPUs), 64); err == nil && f > 0 {
			quota := int64(f * float64(cgroupCPUPeriod))
			l.CPUMax = fmt.Sprintf("%d %d", quota, cgroupCPUPeriod)
		}
	}
	if spec.CPUShares > 0 {
		l.CPUWeight = strconv.Itoa(sharesToWeight(spec.CPUShares))
	}
	if spec.PidsLimit > 0 {
		l.PidsMax = strconv.Itoa(spec.PidsLimit)
	}
	return l
}

// cgApplyInto writes the limits and migrates the shim pid into a fresh
// per-instance cgroup under apps. Returns one note per action so the
// state.json record shows exactly what was enforced. Instance dirs are
// named sandbox-<name>; <name> passed validName, so the path cannot
// escape apps (same reasoning as the state dir, R1).
func (m *cgroupManager) cgApplyInto(name string, spec *contract.RunSpec, pid int) []string {
	l := limitsFromSpec(spec)
	if l == (CgroupLimits{}) {
		return nil
	}
	dir := filepath.Join(m.Apps, "sandbox-"+name)
	var notes []string
	if err := os.MkdirAll(dir, 0755); err != nil {
		return []string{fmt.Sprintf("cgroups v2: mkdir %s failed: %v (limits NOT enforced, CG1 fallback)", dir, err)}
	}
	set := func(file, val, desc string) {
		if val == "" {
			return
		}
		if err := writeCgroupFile(filepath.Join(dir, file), val); err != nil {
			notes = append(notes, fmt.Sprintf("cgroups v2: %s (%s) write failed: %v — partial enforcement", file, desc, err))
		}
	}
	set("memory.max", l.MemoryMax, fmt.Sprintf("Memory=%s", spec.Memory))
	set("cpu.max", l.CPUMax, fmt.Sprintf("CPUs=%s", spec.CPUs))
	set("cpu.weight", l.CPUWeight, fmt.Sprintf("CPUShares=%d", spec.CPUShares))
	set("pids.max", l.PidsMax, fmt.Sprintf("PidsLimit=%d", spec.PidsLimit))
	if err := writeCgroupFile(filepath.Join(dir, "cgroup.procs"), strconv.Itoa(pid)); err != nil {
		notes = append(notes, fmt.Sprintf("cgroups v2: shim pid %d migration failed: %v — limits set but process NOT in cgroup", pid, err))
	}
	notes = append(notes, fmt.Sprintf(
		"cgroups v2 enforced for %s: memory.max=%s cpu.max=%s cpu.weight=%s pids.max=%s under %s (CG1/S2)",
		name, orUnset(l.MemoryMax), orUnset(l.CPUMax), orUnset(l.CPUWeight), orUnset(l.PidsMax), dir))
	return notes
}

func orUnset(v string) string {
	if v == "" {
		return "unset"
	}
	return v
}

// cgroupApply is the Store-level entry used by Run/Start after the shim
// spawned: ensure the manager (once), then enforce. Fail-soft — notes
// always, never an error, because the pre-CG1 contract treats resource
// limits as best-effort.
func (s *Store) cgroupApply(name string, spec *contract.RunSpec, pid int) []string {
	m := s.cgroupManager()
	if !m.Avail {
		return []string{fmt.Sprintf("cgroups v2 unavailable: %s (CG1 fallback: notes + rlimit nproc)", m.reason)}
	}
	return m.cgApplyInto(name, spec, pid)
}

// cgroupInstancePath returns the instance's enforcing cgroup dir, or ""
// when enforcement is unavailable (state stays notes-only).
func (s *Store) cgroupInstancePath(name string) string {
	m := s.cgroupManager()
	if !m.Avail || m.Apps == "" {
		return ""
	}
	return filepath.Join(m.Apps, "sandbox-"+name)
}

// cgroupCleanup rmdirs the instance cgroup after the payload is gone.
// Best-effort: a busy cgroup (lingering process) or an already-removed
// one is silently ignored — the kernel reclaims empty cgroups and the
// next Run with the same name recreates the dir anyway.
func (s *Store) cgroupCleanup(cgPath string) {
	if cgPath == "" {
		return
	}
	_ = os.Remove(cgPath)
}

// cgReadInt reads one integer-valued cgroup file ("1075200\n").
func cgReadInt(path string) (int64, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	v, err := strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// cgCPUStatUsage reads cpu.stat and returns usage_usec (0 when absent).
func cgCPUStatUsage(cgPath string) int64 {
	b, err := os.ReadFile(filepath.Join(cgPath, "cpu.stat"))
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(b), "\n") {
		if usec, ok := strings.CutPrefix(line, "usage_usec "); ok {
			v, err := strconv.ParseInt(strings.TrimSpace(usec), 10, 64)
			if err == nil {
				return v
			}
		}
	}
	return 0
}

// cgStats fills the cgroup-backed slice of contract.Stats. Returns false
// when the cgroup files are unreadable (caller falls back to /proc).
func cgStats(cgPath string, window time.Duration) (memUsage int64, pids int, cpuPercent float64, ok bool) {
	memUsage, ok = cgReadInt(filepath.Join(cgPath, "memory.current"))
	if !ok {
		return 0, 0, 0, false
	}
	if p, okp := cgReadInt(filepath.Join(cgPath, "pids.current")); okp {
		pids = int(p)
	}
	u1 := cgCPUStatUsage(cgPath)
	time.Sleep(window)
	u2 := cgCPUStatUsage(cgPath)
	if window > 0 && u2 > u1 {
		cpuPercent = float64(u2-u1) / float64(window.Microseconds()) * 100.0
	}
	return memUsage, pids, cpuPercent, true
}
