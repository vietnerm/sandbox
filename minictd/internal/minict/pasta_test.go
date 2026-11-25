package minict

// Unit tests for the pasta port-forward config side (GĐ2 mục 4, issue
// #165, D67). No pasta process is spawned here: parsing, argv rendering,
// gateway parsing and resolver placement are pure; the only Store method
// exercised (spawnPasta with a missing binary) fails BEFORE any process
// work. The e2e counterparts are MD47–MD49 in tests/test_minictd.sh
// (env-gated on /dev/net/tun, D34).

import (
	"os"
	"strconv"
	"strings"
	"testing"
)

func TestParsePastaPortsValid(t *testing.T) {
	fwds, err := parsePastaPorts([]string{"18080:8080", "tcp/18081:8081", "udp/15353:53"})
	if err != nil {
		t.Fatalf("valid entries rejected: %v", err)
	}
	if len(fwds) != 3 {
		t.Fatalf("want 3 forwards, got %d: %v", len(fwds), fwds)
	}
	want := []pastaFwd{
		{Proto: pastaTCP, Host: 18080, NS: 8080},
		{Proto: pastaTCP, Host: 18081, NS: 8081},
		{Proto: pastaUDP, Host: 15353, NS: 53},
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
		"-t", "18080:8080,18081:8081",
		"-u", "15353:53",
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
}
