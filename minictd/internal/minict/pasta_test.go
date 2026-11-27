package minict

// Unit tests for the pasta port-forward config side (GĐ2 mục 4, issue
// #165, D67). No pasta process is spawned here: parsing, argv rendering,
// gateway parsing and resolver placement are pure; the only Store method
// exercised (spawnPasta with a missing binary) fails BEFORE any process
// work. The e2e counterparts are MD47–MD49 in tests/test_minictd.sh
// (env-gated on /dev/net/tun, D34).

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestParsePastaPortsValid(t *testing.T) {
	fwds, err := parsePastaPorts([]string{
		"18080:8080", "tcp/18081:8081", "udp/15353:53",
		"public/18082:8082", "tcp/public/18083:8083", "udp/public/15354:54",
	})
	if err != nil {
		t.Fatalf("valid entries rejected: %v", err)
	}
	if len(fwds) != 6 {
		t.Fatalf("want 6 forwards, got %d: %v", len(fwds), fwds)
	}
	want := []pastaFwd{
		{Proto: pastaTCP, Host: 18080, NS: 8080},
		{Proto: pastaTCP, Host: 18081, NS: 8081},
		{Proto: pastaUDP, Host: 15353, NS: 53},
		{Proto: pastaTCP, Host: 18082, NS: 8082, Public: true},
		{Proto: pastaTCP, Host: 18083, NS: 8083, Public: true},
		{Proto: pastaUDP, Host: 15354, NS: 54, Public: true},
	}
	for i, w := range want {
		if fwds[i] != w {
			t.Errorf("fwd[%d] = %+v, want %+v", i, fwds[i], w)
		}
	}
	// Empty spec = no pasta (loopback-only default D17 stays).
	f, err := parsePastaPorts(nil)
	if err != nil || f != nil {
		t.Errorf("nil spec must yield nil forwards, got %v err %v", f, err)
	}
}

func TestParsePastaPortsInvalid(t *testing.T) {
	cases := map[string][]string{
		"reserved host port":    {"80:80"},
		"zero host port":        {"0:80"},
		"zero ns port":          {"18080:0"},
		"host port too big":     {"70000:80"},
		"ns port too big":       {"18080:70000"},
		"negative":              {"-1:80"},
		"not a number":          {"tcp/abc:def"},
		"missing ns":            {"18080:"},
		"missing host":          {":8080"},
		"missing colon":         {"tcp/18080"},
		"empty entry":           {"  "},
		"duplicate host bind":   {"18080:80", "tcp/18080:90"},
		"same host two protos":  {"18080:80", "udp/18080:53"}, // valid! different proto
		"range not supported":   {"18080-18090:80"},
		"bad protocol":          {"sctp/18080:80"},
		"spaces in spec":        {"18080 : 8080"},
		"too many entries 17th": make17(),
	}
	for name, in := range cases {
		fwds, err := parsePastaPorts(in)
		isValid := name == "same host two protos"
		if isValid && err != nil {
			t.Errorf("%s: expected valid (distinct proto), got %v", name, err)
			continue
		}
		if !isValid && err == nil {
			t.Errorf("%s: expected error, got forwards %v", name, fwds)
		}
	}
}

func make17() []string {
	// strictly increasing valid ports so the CAP (not a duplicate) trips
	out := make([]string, 0, 17)
	base := 20000
	for i := 0; i < 17; i++ {
		out = append(out, strconv.Itoa(base+i*2)+":80")
	}
	return out
}

// TestPastaArgs pins the exact argv D37/D42 parity requires (regression:
// a reordered or dropped --userns breaks rootless attach silently).
func TestPastaArgs(t *testing.T) {
	fwds := []pastaFwd{
		{Proto: pastaTCP, Host: 18080, NS: 8080},
		{Proto: pastaTCP, Host: 18081, NS: 8081},
		{Proto: pastaUDP, Host: 15353, NS: 53},
	}
	got := pastaArgs(4242, fwds, "172.17.0.1")
	want := []string{
		"--foreground",
		"--userns", "/proc/4242/ns/user",
		"--netns", "/proc/4242/ns/net",
		"--config-net",
		"--dns-forward", "172.17.0.1",
		"-t", "127.0.0.1/18080:8080,18081:8081",
		"-t", "127.0.0.1/auto",
		"-u", "127.0.0.1/15353:53",
		"-T", "none",
		"-U", "none",
		"--no-map-gw",
	}
	if len(got) != len(want) {
		t.Fatalf("argv length %d, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("argv[%d] = %q, want %q", i, got[i], want[i])
		}
	}
	// No default route (fail-soft D42): no --dns-forward, forwards only.
	got = pastaArgs(4242, fwds[:1], "")
	if hasArg(got, "--dns-forward") {
		t.Errorf("empty gw must omit --dns-forward: %v", got)
	}
	if !hasArg(got, "-t") {
		t.Errorf("tcp forward missing: %v", got)
	}
}

// TestPastaArgsFailClosed (#220/D82): with NO forwards declared the argv
// must still pin every pasta default that was measured as a hole: TCP auto
// loopback-bound, UDP auto killed (-u none), ns→host killed (-T/-U none),
// gateway mapping killed (--no-map-gw).
func TestPastaArgsFailClosed(t *testing.T) {
	got := pastaArgs(7, nil, "")
	want := []string{
		"--foreground",
		"--userns", "/proc/7/ns/user",
		"--netns", "/proc/7/ns/net",
		"--config-net",
		"-t", "127.0.0.1/auto",
		"-u", "none",
		"-T", "none",
		"-U", "none",
		"--no-map-gw",
	}
	if len(got) != len(want) {
		t.Fatalf("argv length %d, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("argv[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// TestPastaArgsPublicOptIn (#220/D82): "public/" entries must land in their
// OWN flag without the loopback prefix (one pasta flag binds ONE address
// for its whole comma list), local entries keep 127.0.0.1/, and UDP
// explicit spec must NOT carry an auto entry (explicit = scan disabled).
func TestPastaArgsPublicOptIn(t *testing.T) {
	fwds := []pastaFwd{
		{Proto: pastaTCP, Host: 18080, NS: 8080},
		{Proto: pastaTCP, Host: 9090, NS: 80, Public: true},
		{Proto: pastaUDP, Host: 15353, NS: 53},
		{Proto: pastaUDP, Host: 15354, NS: 54, Public: true},
	}
	got := pastaArgs(7, fwds, "")
	var flags []string
	for i, a := range got {
		if a == "-t" || a == "-u" {
			flags = append(flags, a, got[i+1])
		}
	}
	want := []string{
		"-t", "127.0.0.1/18080:8080",
		"-t", "127.0.0.1/auto",
		"-t", "9090:80",
		"-u", "127.0.0.1/15353:53",
		"-u", "15354:54",
	}
	if len(flags) != len(want) {
		t.Fatalf("forward flags %v, want %v", flags, want)
	}
	for i := range want {
		if flags[i] != want[i] {
			t.Errorf("flags[%d] = %q, want %q", i, flags[i], want[i])
		}
	}
}

func hasArg(args []string, a string) bool {
	for _, x := range args {
		if x == a {
			return true
		}
	}
	return false
}

// TestHostDefaultGW parses a /proc/net/route fixture. 010011AC little-
// endian = 172.17.0.1 (docker0-style default gw).
func TestHostDefaultGW(t *testing.T) {
	fx := "Iface\tDestination\tGateway\tFlags\tRefCnt\tUse\tMetric\tMask\tMTU\tWindow\tIRTT\n" +
		"eth0\t00000000\t010011AC\t0003\t0\t0\t0\t00000000\t0\t0\t0\n" +
		"eth0\t000011AC\t00000000\t0001\t0\t0\t0\t000FFFFF\t0\t0\t0\n"
	f := t.TempDir() + "/route"
	if err := os.WriteFile(f, []byte(fx), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := hostDefaultGW(f); got != "172.17.0.1" {
		t.Fatalf("gw = %q, want 172.17.0.1", got)
	}
	// No default row -> fail-soft empty.
	f2 := t.TempDir() + "/route2"
	if err := os.WriteFile(f2, []byte("Iface\tDestination\tGateway\neth0\t000011AC\t00000000\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := hostDefaultGW(f2); got != "" {
		t.Fatalf("no-default route must yield empty, got %q", got)
	}
	// Missing file -> empty (fail-soft), never a panic.
	if got := hostDefaultGW(t.TempDir() + "/nonexistent"); got != "" {
		t.Fatalf("missing file must yield empty, got %q", got)
	}
}

// TestPastaResolverDir pins the overlay split (D66): upper/etc for overlay
// instances, rootfs/etc for legacy — NEVER lower (shared zero-copy source).
func TestPastaResolverDir(t *testing.T) {
	dir := t.TempDir()
	if got := pastaResolverDir(dir, false); got != dir+"/rootfs/etc" {
		t.Errorf("legacy resolver dir = %q", got)
	}
	if got := pastaResolverDir(dir, true); got != dir+"/upper/etc" {
		t.Errorf("overlay resolver dir = %q", got)
	}
}

// TestBakeResolver: writes nameserver GW, records the note; no-GW is a
// fail-soft note (D42), never an error.
func TestBakeResolver(t *testing.T) {
	s := &Store{}
	dir := t.TempDir()
	notes := s.bakeResolver(dir, false, "10.0.0.1")
	rb, err := os.ReadFile(dir + "/rootfs/etc/resolv.conf")
	if err != nil {
		t.Fatal(err)
	}
	b := string(rb)
	if !strings.Contains(b, "nameserver 10.0.0.1") {
		t.Errorf("resolver content = %q", b)
	}
	if len(notes) != 1 || !strings.Contains(notes[0], "baked into instance resolver") {
		t.Errorf("notes = %v", notes)
	}
	notes = s.bakeResolver(dir, false, "")
	if len(notes) != 1 || !strings.Contains(notes[0], "no host default route") {
		t.Errorf("no-gw notes = %v", notes)
	}
}

// TestSpawnPastaBinMissing: fail-closed BEFORE any process/proc work —
// a declared pasta_ports with a bad --pasta-bin must die here with a
// message that names the fix (tools/fetch-pasta.sh).
func TestSpawnPastaBinMissing(t *testing.T) {
	s := &Store{PastaBin: t.TempDir() + "/no-such-pasta"}
	_, _, err := s.spawnPasta("x", t.TempDir(), 1, []pastaFwd{{Proto: pastaTCP, Host: 18080, NS: 80}}, "10.0.0.1")
	if err == nil {
		t.Fatal("missing pasta binary must fail")
	}
	if !strings.Contains(err.Error(), "fetch-pasta.sh") {
		t.Errorf("error must point at tools/fetch-pasta.sh, got: %v", err)
	}
}

// TestPastaFwdString pins the note grammar used in state.json notes.
func TestPastaFwdString(t *testing.T) {
	f := pastaFwd{Proto: pastaUDP, Host: 15353, NS: 53}
	if f.String() != "udp/15353->53" {
		t.Errorf("String() = %q", f.String())
	}
	if f.key() != "udp/15353" {
		t.Errorf("key() = %q", f.key())
	}
	if got := pastaFwdList([]pastaFwd{{Proto: pastaTCP, Host: 18080, NS: 8080}, f}); got != "tcp/18080->8080, udp/15353->53" {
		t.Errorf("pastaFwdList = %q", got)
	}
	p := pastaFwd{Proto: pastaTCP, Host: 9090, NS: 80, Public: true}
	if p.String() != "tcp/9090->80[public]" {
		t.Errorf("public String() = %q", p.String())
	}
}

// TestStopPastaKills drives stopPasta with a REAL dummy process (no pasta,
// no /dev/net/tun needed): the recorded pid must die, the pasta-pid file
// must be removed, and the state fields must be cleared. This is the
// mutant catcher for M-stop-pasta on hosts where MD48 is env-blocked
// (D34) — pasta holds host listening sockets, so a stop that fails to
// kill would leak the forward binds.
func TestStopPastaKills(t *testing.T) {
	dir := t.TempDir()
	s := &Store{Root: dir}
	cmd := exec.Command("/bin/sh", "-c", "sleep 30")
	if err := cmd.Start(); err != nil {
		t.Skipf("no /bin/sh: %v", err)
	}
	go cmd.Wait() // reap — a zombie would fool a plain kill(pid,0) liveness check
	pid := cmd.Process.Pid
	var ticks uint64
	if _, ticks, _ = procStat(pid); ticks == 0 {
		ticks = 0 // reuse guard off — pid from our own Start, no reuse risk
	}
	_ = os.MkdirAll(filepath.Join(dir, "x"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, "x", "pasta-pid"), []byte("1\n"), 0o644)
	st := &InstanceState{Name: "x", PastaPID: pid, PastaTicks: ticks}
	s.stopPasta(st)
	if st.PastaPID != 0 || st.PastaTicks != 0 {
		t.Errorf("stopPasta must clear state fields, got pid=%d ticks=%d", st.PastaPID, st.PastaTicks)
	}
	if _, err := os.Stat(filepath.Join(dir, "x", "pasta-pid")); !os.IsNotExist(err) {
		t.Errorf("pasta-pid file must be removed")
	}
	// the dummy process must actually die (TERM, 200ms grace, then KILL);
	// pidAlive is zombie-aware — a plain kill(pid,0) liveness check is not
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if !s.pidAlive(pid, ticks) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if s.pidAlive(pid, ticks) {
		_ = syscall.Kill(pid, syscall.SIGKILL)
		t.Errorf("stopPasta left pid %d alive", pid)
	}
}

// TestStopPastaNoopForZero: zero/unknown pid is a no-op, not a kill of pid 0.
func TestStopPastaNoopForZero(t *testing.T) {
	s := &Store{Root: t.TempDir()}
	st := &InstanceState{Name: "y"}
	s.stopPasta(st) // must not panic / not signal anything
	if st.PastaPID != 0 {
		t.Errorf("no-op stop must keep zero fields")
	}
}

// TestWaitNsDistinct exercises the D67 unshare-race guard with REAL
// namespace processes: self is never distinct (control), an unshare -Urn
// process is distinct immediately after exec, and a dead process errors.
func TestWaitNsDistinct(t *testing.T) {
	selfU, err := os.Readlink("/proc/self/ns/user")
	if err != nil {
		t.Skipf("no /proc/self/ns/user: %v", err)
	}
	// control: the test process itself — never distinct (the old bug made
	// pasta attach while the shim still pointed here → setns EINVAL)
	if err := waitNsDistinct(selfU, "net:[does-not-match]", os.Getpid(), 100*time.Millisecond); err == nil {
		t.Fatal("self ns must never count as distinct")
	}
	// real unshare: userns + netns (mount ns not needed here)
	if _, err := exec.LookPath("unshare"); err != nil {
		t.Skip("no unshare binary")
	}
	cmd := exec.Command("unshare", "-Urn", "sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Skipf("unshare blocked on this host: %v", err)
	}
	go cmd.Wait()
	if err := waitNsDistinct(selfU, "net:[does-not-match]", cmd.Process.Pid, 3*time.Second); err != nil {
		t.Errorf("unshared process must be distinct quickly: %v", err)
	}
	// dead process: Readlink ENOENT → the helper must report it, not spin
	cmd.Process.Kill()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Readlink(fmt.Sprintf("/proc/%d/ns/user", cmd.Process.Pid)); err != nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	err = waitNsDistinct(selfU, "net:[does-not-match]", cmd.Process.Pid, 300*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "gone") {
		t.Errorf("dead process must yield the gone-error, got %v", err)
	}
}
