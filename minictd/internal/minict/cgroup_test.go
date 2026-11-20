package minict

// Unit tests for CG1/S2 cgroups v2 enforcement (issue #4). They run on a
// FAKE cgroup filesystem (a temp dir seeded with the v2 interface files),
// so the mapping/enable/migrate logic is testable on any host — including
// the cgroups-v1 dev machine. The e2e truth on a real v2 host lives in
// tests/test_cg.sh (env-gated; PASS on the CI runner).

import (
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/vietnerm/sandbox/minictd/contract"
)

// testLogger is the quiet logger the Store tests use.
func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

// fakeCgroupRoot seeds a cgroup-v2-shaped temp dir:
//
//	<dir>/cgroup.controllers  "memory cpu pids"
//	<dir>/cgroup.procs        optional seeded pids
//	<dir>/cgroup.subtree_control  empty (kernel created)
func fakeCgroupRoot(t *testing.T, procs []int) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "cgroup.controllers"), []byte("memory cpu pids\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "cgroup.subtree_control"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	var pids []string
	for _, p := range procs {
		pids = append(pids, strconv.Itoa(p))
	}
	if err := os.WriteFile(filepath.Join(dir, "cgroup.procs"), []byte(strings.Join(pids, "\n")+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func readFileStr(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return strings.TrimSpace(string(b))
}

// TestCgResolveBaseOverrideInsideSubtree: override accepted when the
// (fake) daemon cgroup sits inside it.
func TestCgResolveBaseOverrideInsideSubtree(t *testing.T) {
	base := "/sys/fs/cgroup/test.slice"
	ok, reason := func() (bool, string) {
		// cgResolveBase needs the real filesystem for the override marker;
		// point it at a temp dir with cgroup.controllers.
		dir := fakeCgroupRoot(t, nil)
		b, okk, r := cgResolveBase(dir, strings.TrimPrefix(dir, "/sys/fs/cgroup"))
		base = b
		return okk, r
	}()
	if !ok {
		t.Fatalf("override inside subtree should be OK, reason=%s", reason)
	}
	if base == "" {
		t.Fatal("base empty")
	}
}

// TestCgResolveBaseOverrideOutside: the override must contain the daemon's
// own cgroup (Delegation Containment) — a foreign base is refused.
func TestCgResolveBaseOverrideOutside(t *testing.T) {
	dir := fakeCgroupRoot(t, nil)
	_, ok, reason := cgResolveBase(dir, "/some/other/cgroup/path")
	if ok {
		t.Fatal("foreign override must be refused")
	}
	if !strings.Contains(reason, "containment") {
		t.Fatalf("reason should mention containment, got: %s", reason)
	}
}

// TestCgResolveBaseNoV2: cgroups v1 host — no 0:: entry.
func TestCgResolveBaseNoV2(t *testing.T) {
	_, ok, reason := cgResolveBase("", "")
	if ok {
		t.Fatal("v1 host must not resolve")
	}
	if !strings.Contains(reason, "v2") {
		t.Fatalf("reason should mention v2, got: %s", reason)
	}
}

// TestCgResolveBaseRoot: daemon at the hierarchy root without delegation.
func TestCgResolveBaseRoot(t *testing.T) {
	_, ok, reason := cgResolveBase("", "/")
	if ok {
		t.Fatal("root-sitting daemon must not resolve")
	}
	if !strings.Contains(reason, "root") {
		t.Fatalf("reason should mention root, got: %s", reason)
	}
}

// TestCgEnsureAppsFull: daemon pid in base → self-migrate + apps tree +
// controllers enabled on base. Mutation-sensitive: a manager that skips
// the self-move or the subtree_control writes fails these asserts.
func TestCgEnsureAppsFull(t *testing.T) {
	base := fakeCgroupRoot(t, []int{os.Getpid()})
	m := &cgroupManager{Base: base}
	if err := m.ensureApps(os.Getpid()); err != nil {
		t.Fatalf("ensureApps: %v", err)
	}
	// daemon parked
	if got := readFileStr(t, filepath.Join(base, "minictd-daemon", "cgroup.procs")); got != strconv.Itoa(os.Getpid()) {
		t.Fatalf("daemon pid not migrated: %q", got)
	}
	// apps tree
	if m.Apps != filepath.Join(base, "minictd-apps") {
		t.Fatalf("apps path = %q", m.Apps)
	}
	if _, err := os.Stat(m.Apps); err != nil {
		t.Fatalf("apps dir missing: %v", err)
	}
	// controllers enabled (mutant M-CG-1 target: skip subtree_control)
	if got := readFileStr(t, filepath.Join(base, "cgroup.subtree_control")); !strings.Contains(got, "+memory") {
		t.Fatalf("memory controller not enabled: %q", got)
	}
	// second hop: apps must distribute the controllers to sandbox-*
	if got := readFileStr(t, filepath.Join(m.Apps, "cgroup.subtree_control")); !strings.Contains(got, "+memory") {
		t.Fatalf("apps subtree_control missing +memory: %q", got)
	}
	if len(m.Enabled) != 3 {
		t.Fatalf("Enabled = %v, want memory+cpu+pids", m.Enabled)
	}
}

// TestCgEnsureAppsAlreadyParked: re-entrant — daemon already in
// minictd-daemon must not error (second ensureApps after self-move).
func TestCgEnsureAppsAlreadyParked(t *testing.T) {
	base := fakeCgroupRoot(t, nil)
	if err := os.MkdirAll(filepath.Join(base, cgroupDaemonDir), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, cgroupDaemonDir, "cgroup.procs"), []byte(strconv.Itoa(os.Getpid())+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	m := &cgroupManager{Base: base}
	if err := m.ensureApps(os.Getpid()); err != nil {
		t.Fatalf("re-entrant ensureApps: %v", err)
	}
	if m.Apps == "" {
		t.Fatal("apps not set")
	}
}

// TestCgEnsureAppsForeignDaemon: daemon NOT inside base at all → refuse
// (containment; the suite bootstraps with sudo and passes the override).
func TestCgEnsureAppsForeignDaemon(t *testing.T) {
	base := fakeCgroupRoot(t, nil)
	m := &cgroupManager{Base: base}
	err := m.ensureApps(os.Getpid())
	if err == nil {
		t.Fatal("foreign daemon must be refused")
	}
	if !strings.Contains(err.Error(), "not inside base") {
		t.Fatalf("wrong error: %v", err)
	}
}

// TestLimitsFromSpec: docker-shaped spec → v2 files.
func TestLimitsFromSpec(t *testing.T) {
	l := limitsFromSpec(&contract.RunSpec{
		Memory: "64m", CPUs: "1.5", CPUShares: 1024, PidsLimit: 256,
	})
	if l.MemoryMax != strconv.FormatInt(64*1024*1024, 10) {
		t.Fatalf("MemoryMax = %q", l.MemoryMax)
	}
	if l.CPUMax != "150000 100000" {
		t.Fatalf("CPUMax = %q", l.CPUMax)
	}
	if l.CPUWeight != "100" {
		t.Fatalf("CPUWeight = %q", l.CPUWeight)
	}
	if l.PidsMax != "256" {
		t.Fatalf("PidsMax = %q", l.PidsMax)
	}
	// empty spec → no enforcement
	if empty := limitsFromSpec(&contract.RunSpec{}); empty != (CgroupLimits{}) {
		t.Fatalf("empty spec produced limits %+v", empty)
	}
}

// TestSharesToWeight: linear mapping + clamps.
func TestSharesToWeight(t *testing.T) {
	if got := sharesToWeight(1024); got != 100 {
		t.Fatalf("1024 shares -> %d", got)
	}
	if got := sharesToWeight(0); got != 1 {
		t.Fatalf("0 shares -> %d (want clamp 1)", got)
	}
	if got := sharesToWeight(102400000); got != 10000 {
		t.Fatalf("huge shares -> %d (want clamp 10000)", got)
	}
}

// TestCgApplyInto: files land with the right values, shim pid migrated,
// note records the enforcement. Mutant targets: skipping any write or the
// pid migration fails these asserts.
func TestCgApplyInto(t *testing.T) {
	base := fakeCgroupRoot(t, nil)
	m := &cgroupManager{Base: base, Apps: filepath.Join(base, cgroupAppsDir), Avail: true}
	if err := os.MkdirAll(m.Apps, 0755); err != nil {
		t.Fatal(err)
	}
	spec := &contract.RunSpec{Memory: "32m", PidsLimit: 16}
	notes := m.cgApplyInto("cg-app", spec, 424242)
	dir := filepath.Join(m.Apps, "sandbox-cg-app")
	if got := readFileStr(t, filepath.Join(dir, "memory.max")); got != strconv.Itoa(32*1024*1024) {
		t.Fatalf("memory.max = %q", got)
	}
	if got := readFileStr(t, filepath.Join(dir, "pids.max")); got != "16" {
		t.Fatalf("pids.max = %q", got)
	}
	if got := readFileStr(t, filepath.Join(dir, "cgroup.procs")); got != "424242" {
		t.Fatalf("cgroup.procs = %q (shim not migrated)", got)
	}
	if len(notes) == 0 || !strings.Contains(notes[len(notes)-1], "cgroups v2 enforced") {
		t.Fatalf("missing enforcement note: %v", notes)
	}
	// no limits → nothing applied
	if n2 := m.cgApplyInto("cg-noop", &contract.RunSpec{}, 1); n2 != nil {
		t.Fatalf("empty spec should apply nothing, got %v", n2)
	}
	if _, err := os.Stat(filepath.Join(m.Apps, "sandbox-cg-noop")); !os.IsNotExist(err) {
		t.Fatal("noop instance must not create a cgroup dir")
	}
}

// TestCgApplyIntoPartial: a failed limit write degrades to a note, not an
// error (fail-soft contract; partial enforcement recorded).
func TestCgApplyIntoPartial(t *testing.T) {
	base := fakeCgroupRoot(t, nil)
	m := &cgroupManager{Base: base, Apps: filepath.Join(base, cgroupAppsDir), Avail: true}
	if err := os.MkdirAll(m.Apps, 0755); err != nil {
		t.Fatal(err)
	}
	// read-only memory.max → write fails
	dir := filepath.Join(m.Apps, "sandbox-part")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "memory.max"), []byte("max\n"), 0444); err != nil {
		t.Fatal(err)
	}
	notes := m.cgApplyInto("part", &contract.RunSpec{Memory: "8m"}, 1)
	found := false
	for _, n := range notes {
		if strings.Contains(n, "partial enforcement") {
			found = true
		}
	}
	if !found {
		t.Fatalf("partial-enforcement note missing: %v", notes)
	}
}

// TestCgStats: memory.current/pids.current/cpu.stat reads over fake files.
func TestCgStats(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "memory.current"), []byte("5242880\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pids.current"), []byte("3\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "cpu.stat"), []byte("usage_usec 4200000\nuser_usec 3000000\n"), 0644); err != nil {
		t.Fatal(err)
	}
	mem, pids, _, ok := cgStats(dir, 0) // zero window: no sleep, cpu% 0
	if !ok {
		t.Fatal("cgStats should succeed")
	}
	if mem != 5242880 || pids != 3 {
		t.Fatalf("mem=%d pids=%d", mem, pids)
	}
	if u := cgCPUStatUsage(dir); u != 4200000 {
		t.Fatalf("usage_usec = %d", u)
	}
	// missing memory.current → not ok
	if _, _, _, ok := cgStats(t.TempDir(), 0); ok {
		t.Fatal("missing files must not report ok")
	}
}

// TestCgManagerFailSoft: a Store without a resolvable base degrades to
// notes, never errors (the dev cgroups-v1 path).
func TestCgManagerFailSoft(t *testing.T) {
	s := &Store{Root: t.TempDir(), Log: testLogger()}
	// No override; the test process's real /proc/self/cgroup decides. In
	// this sandbox (cgroups v1) there is no 0:: entry → fail-soft. On a v2
	// host running the tests the daemon sits at root or in a foreign path
	// → also refused. Either way: notes, not panic, and no enforcement
	// claim.
	notes := s.cgroupApply("x", &contract.RunSpec{Memory: "8m"}, 1)
	if len(notes) == 0 {
		t.Fatal("fail-soft must still produce a note")
	}
	if strings.Contains(notes[0], "enforced for") {
		t.Fatalf("must not claim enforcement on a foreign base: %v", notes)
	}
}

// TestCgInstancePathAndCleanup: Store-level path resolution + cleanup
// (mutant target M-CG-5: a no-op cleanup leaves the dir behind).
func TestCgInstancePathAndCleanup(t *testing.T) {
	base := fakeCgroupRoot(t, nil)
	if err := os.MkdirAll(filepath.Join(base, cgroupDaemonDir), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, cgroupDaemonDir, "cgroup.procs"), []byte(strconv.Itoa(os.Getpid())+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	s := &Store{Root: t.TempDir(), Log: testLogger(), CgroupRoot: base}
	// NOTE: the real /proc/self/cgroup of the test process decides whether
	// the override is accepted (daemon must sit inside the base). On the
	// v1 dev host this resolution fails soft — so drive the manager
	// directly instead of through the Store cache.
	m := &cgroupManager{Base: base}
	if err := m.ensureApps(os.Getpid()); err != nil {
		t.Fatalf("ensureApps: %v", err)
	}
	inst := filepath.Join(m.Apps, "sandbox-x")
	if err := os.MkdirAll(inst, 0755); err != nil {
		t.Fatal(err)
	}
	// cleanup removes an existing dir
	s.cgroupCleanup(inst)
	if _, err := os.Stat(inst); !os.IsNotExist(err) {
		t.Fatal("cleanup must remove the instance cgroup dir")
	}
	// cleanup is idempotent (missing dir -> no error, no panic)
	s.cgroupCleanup(inst)
	// empty path is a no-op
	s.cgroupCleanup("")
}
