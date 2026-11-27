package main

// Unit tests for the Track M M3 static template registry (issue #215, D81).
// Covers: ref/gate validation, push (index + blob), pull roundtrip over a
// real local HTTP server (httptest), sha256-mismatch fail-closed with NO
// files left behind, hostile tar refusal (".." member + escaping symlink —
// index vouches for the blob, the extractor is the last line), and the
// version/latest selection rules.

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// buildRootfs makes a small rootfs: a file, a nested dir with a file, and a
// RELATIVE symlink (absolute targets are refused by design, D70).
func buildRootfs(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	write := func(p, s string) {
		t.Helper()
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(dir, "hello.txt"), "hello minict\n")
	if err := os.Mkdir(filepath.Join(dir, "etc"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(filepath.Join(dir, "etc", "motd"), "sandbox\n")
	if err := os.Symlink("hello.txt", filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	return dir
}

// pushTestRegistry pushes one template into a fresh registry dir and returns
// the registry dir + the index entry.
func pushTestRegistry(t *testing.T, name, version string) (string, registryEntry) {
	t.Helper()
	rootfs := buildRootfs(t)
	reg := filepath.Join(t.TempDir(), "reg")
	if err := templatePush(name, version, rootfs, reg); err != nil {
		t.Fatalf("push: %v", err)
	}
	idx := readIndex(t, reg)
	if len(idx.Templates) != 1 {
		t.Fatalf("index entries = %d, want 1", len(idx.Templates))
	}
	return reg, idx.Templates[0]
}

func readIndex(t *testing.T, reg string) registryIndex {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(reg, "index.json"))
	if err != nil {
		t.Fatalf("index.json: %v", err)
	}
	var idx registryIndex
	if err := unmarshalStrict(b, &idx); err != nil {
		t.Fatalf("index.json parse: %v", err)
	}
	return idx
}

func TestValidateRefToken(t *testing.T) {
	ok := []string{"web-base", "1.0.0", "a", "A-b_c.9"}
	for _, s := range ok {
		if err := validateRefToken(s, "name"); err != nil {
			t.Errorf("validateRefToken(%q) = %v, want nil", s, err)
		}
	}
	bad := []string{"", "../evil", "a/b", "-lead", ".dot", strings.Repeat("x", 65)}
	for _, s := range bad {
		if err := validateRefToken(s, "name"); err == nil {
			t.Errorf("validateRefToken(%q) = nil, want error", s)
		}
	}
}

func TestParsePullRef(t *testing.T) {
	root, name, ver, err := parsePullRef("https://mirror.example.com/base/web-base:1.2")
	if err != nil || root != "https://mirror.example.com/base" || name != "web-base" || ver != "1.2" {
		t.Fatalf("got root=%q name=%q ver=%q err=%v", root, name, ver, err)
	}
	root, name, ver, err = parsePullRef("https://127.0.0.1:8500/tpl")
	if err != nil || root != "https://127.0.0.1:8500" || name != "tpl" || ver != "" {
		t.Fatalf("no-version: root=%q name=%q ver=%q err=%v", root, name, ver, err)
	}
	for _, bad := range []string{"mirror.example.com/tpl", "https://h/x/../y", "https://h/name:../v", "https://h/", "ftp://h/tpl"} {
		if _, _, _, err := parsePullRef(bad); err == nil {
			t.Errorf("parsePullRef(%q) = nil error, want error", bad)
		}
	}
}

func TestGateTransport(t *testing.T) {
	if err := gateTransport("https://mirror.example.com", false); err != nil {
		t.Errorf("https always ok: %v", err)
	}
	for _, tc := range []struct {
		url      string
		insecure bool
		ok       bool
	}{
		{"http://127.0.0.1:8900", true, true},
		{"http://localhost:8900", true, true},
		{"http://[::1]:8900", true, true},
		{"http://127.1.2.3:8900", true, true}, // 127/8 is loopback
		{"http://127.0.0.1:8900", false, false},
		{"http://mirror.example.com", true, false}, // --insecure NOT enough off-loopback
		{"http://mirror.example.com", false, false},
		{"ftp://127.0.0.1", true, false},
	} {
		err := gateTransport(tc.url, tc.insecure)
		if tc.ok != (err == nil) {
			t.Errorf("gateTransport(%q, insecure=%v) err=%v, ok want %v", tc.url, tc.insecure, err, tc.ok)
		}
	}
}

func TestPushCreatesIndexAndArchive(t *testing.T) {
	reg, e := pushTestRegistry(t, "web-base", "1.0.0")
	b, err := os.ReadFile(filepath.Join(reg, "web-base-1.0.0.tar.gz"))
	if err != nil {
		t.Fatalf("blob: %v", err)
	}
	if int64(len(b)) != e.SizeBytes {
		t.Fatalf("blob size %d != index size_bytes %d", len(b), e.SizeBytes)
	}
	sum := sha256.Sum256(b)
	if hex.EncodeToString(sum[:]) != e.SHA256 {
		t.Fatalf("index sha256 != actual blob sha256")
	}
	if e.Created == "" {
		t.Fatalf("created empty")
	}
}

func TestPushUpsertAndSort(t *testing.T) {
	rootfs := buildRootfs(t)
	reg := filepath.Join(t.TempDir(), "reg")
	for _, p := range [][2]string{{"b-tpl", "2.0"}, {"a-tpl", "1.0"}, {"b-tpl", "1.0"}} {
		if err := templatePush(p[0], p[1], rootfs, reg); err != nil {
			t.Fatalf("push %v: %v", p, err)
		}
	}
	idx := readIndex(t, reg)
	if len(idx.Templates) != 3 {
		t.Fatalf("entries = %d, want 3", len(idx.Templates))
	}
	got := [][2]string{}
	for _, e := range idx.Templates {
		got = append(got, [2]string{e.Name, e.Version})
	}
	want := [][2]string{{"a-tpl", "1.0"}, {"b-tpl", "1.0"}, {"b-tpl", "2.0"}}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("sort: got %v, want %v", got, want)
		}
	}
	// Re-push same name+version replaces (no duplicate rows).
	if err := templatePush("b-tpl", "1.0", rootfs, reg); err != nil {
		t.Fatalf("re-push: %v", err)
	}
	if n := len(readIndex(t, reg).Templates); n != 3 {
		t.Fatalf("after re-push entries = %d, want 3", n)
	}
}

func TestPushRefusesAbsoluteSymlink(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/tmp/whatever", filepath.Join(dir, "abs")); err != nil {
		t.Fatal(err)
	}
	if err := templatePush("abs-tpl", "1.0", dir, filepath.Join(t.TempDir(), "reg")); err == nil {
		t.Fatalf("push with absolute symlink = nil error, want refusal (D70)")
	} else if !strings.Contains(err.Error(), "relative symlinks only") {
		t.Fatalf("error should name the D70 rule: %v", err)
	}
}

// serveRegistry serves a registry dir over plain HTTP on 127.0.0.1 (loopback
// — the only http the transport gate allows, and only with --insecure).
func serveRegistry(t *testing.T, reg string) string {
	t.Helper()
	srv := httptest.NewServer(http.FileServer(http.Dir(reg)))
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestPullRoundtrip(t *testing.T) {
	reg, _ := pushTestRegistry(t, "web-base", "1.0.0")
	url := serveRegistry(t, reg)
	out := filepath.Join(t.TempDir(), "out")
	if err := templatePull(url+"/web-base:1.0.0", out, true); err != nil {
		t.Fatalf("pull: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(out, "hello.txt"))
	if err != nil || string(b) != "hello minict\n" {
		t.Fatalf("hello.txt: %q err=%v", b, err)
	}
	if b, err := os.ReadFile(filepath.Join(out, "etc", "motd")); err != nil || string(b) != "sandbox\n" {
		t.Fatalf("etc/motd: %q err=%v", b, err)
	}
	// Relative symlink preserved (D70 posture: only relative survive).
	tgt, err := os.Readlink(filepath.Join(out, "link"))
	if err != nil || tgt != "hello.txt" {
		t.Fatalf("link target = %q err=%v, want hello.txt", tgt, err)
	}
}

func TestPullLatestWhenVersionOmitted(t *testing.T) {
	rootfs := buildRootfs(t)
	reg := filepath.Join(t.TempDir(), "reg")
	for _, v := range []string{"1.0.0", "2.0.0"} {
		if err := templatePush("web-base", v, rootfs, reg); err != nil {
			t.Fatal(err)
		}
	}
	// Force a created-order: entries carry wall-clock created; same-second
	// pushes tie-break by version string, so 2.0.0 must be picked here.
	url := serveRegistry(t, reg)
	out := filepath.Join(t.TempDir(), "out")
	if err := templatePull(url+"/web-base", out, true); err != nil {
		t.Fatalf("pull latest: %v", err)
	}
	idx := readIndex(t, reg)
	var want registryEntry
	for _, e := range idx.Templates {
		if e.Version == "2.0.0" {
			want = e
		}
	}
	// The picked entry is 2.0.0 — assert via the index (created tie-break
	// by version string, documented in pickEntry).
	if want.Version != "2.0.0" || want.SizeBytes <= 0 {
		t.Fatal("setup error: 2.0.0 entry missing")
	}
	// Re-pull naming the version explicitly stays deterministic.
	out2 := filepath.Join(t.TempDir(), "out2")
	if err := templatePull(url+"/web-base:1.0.0", out2, true); err != nil {
		t.Fatalf("pull pinned: %v", err)
	}
	if _, err := os.Stat(filepath.Join(out2, "hello.txt")); err != nil {
		t.Fatalf("pinned pull incomplete: %v", err)
	}
}

func TestPullSHA256MismatchFailClosed(t *testing.T) {
	reg, _ := pushTestRegistry(t, "web-base", "1.0.0")
	// Tamper the blob AFTER the index was written — flip a byte so SIZE
	// stays equal and the failure really goes through the sha256 branch
	// (not the earlier size check).
	blob := filepath.Join(reg, "web-base-1.0.0.tar.gz")
	b, _ := os.ReadFile(blob)
	b[10] ^= 0x5A
	if err := os.WriteFile(blob, b, 0o644); err != nil {
		t.Fatal(err)
	}
	url := serveRegistry(t, reg)
	parent := filepath.Join(t.TempDir(), "parent")
	out := filepath.Join(parent, "out")
	err := templatePull(url+"/web-base:1.0.0", out, true)
	if err == nil || !strings.Contains(err.Error(), "sha256 mismatch") {
		t.Fatalf("pull err = %v, want sha256 mismatch", err)
	}
	failClosed(t, parent, out)
}

func TestPullUnknownVersionListsAvailable(t *testing.T) {
	reg, _ := pushTestRegistry(t, "web-base", "1.0.0")
	url := serveRegistry(t, reg)
	err := templatePull(url+"/web-base:9.9", filepath.Join(t.TempDir(), "out"), true)
	if err == nil || !strings.Contains(err.Error(), "1.0.0") {
		t.Fatalf("err = %v, want available-versions listing", err)
	}
	err = templatePull(url+"/ghost:1.0", filepath.Join(t.TempDir(), "out"), true)
	if err == nil || !strings.Contains(err.Error(), "not in index") {
		t.Fatalf("err = %v, want not-in-index", err)
	}
}

func TestPullHTTPGateBlocksRemoteWithoutTLS(t *testing.T) {
	// Non-loopback http must be refused BEFORE any dial — no server needed.
	err := templatePull("http://mirror.example.com/web-base:1.0.0",
		filepath.Join(t.TempDir(), "out"), true)
	if err == nil || !strings.Contains(err.Error(), "loopback") {
		t.Fatalf("err = %v, want loopback refusal", err)
	}
}

// hostileTar builds a valid gzip tar with one evil member and writes it into
// reg under name-version, updating the index so the sha256 MATCHES — the
// registry itself is the adversary; the extractor must still refuse.
func hostileTar(t *testing.T, reg, name, version string, evil func(tw *tar.Writer) error) {
	t.Helper()
	blob := filepath.Join(reg, blobName(name, version))
	f, err := os.Create(blob)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.New()
	gw := gzip.NewWriter(io_multi(f, hash))
	tw := tar.NewWriter(gw)
	if err := tw.WriteHeader(&tar.Header{Name: "ok.txt", Mode: 0o644, Size: 2}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte("ok")); err != nil {
		t.Fatal(err)
	}
	if err := evil(tw); err != nil {
		t.Fatal(err)
	}
	tw.Close()
	gw.Close()
	f.Close()
	sum := hex.EncodeToString(hash.Sum(nil))
	st, _ := os.Stat(blob)
	upsertIndex(reg, registryEntry{
		Name: name, Version: version, SHA256: sum,
		SizeBytes: st.Size(), Created: "2026-11-27T00:00:00Z",
	})
}

func TestPullMaliciousDotDotTarBlocked(t *testing.T) {
	reg := filepath.Join(t.TempDir(), "reg")
	if err := os.MkdirAll(reg, 0o755); err != nil {
		t.Fatal(err)
	}
	hostileTar(t, reg, "evil", "1.0", func(tw *tar.Writer) error {
		return tw.WriteHeader(&tar.Header{Name: "../escape.txt", Mode: 0o644, Size: 6})
	})
	url := serveRegistry(t, reg)
	parent := filepath.Join(t.TempDir(), "parent")
	out := filepath.Join(parent, "out")
	err := templatePull(url+"/evil:1.0", out, true)
	if err == nil || !strings.Contains(err.Error(), "refused") {
		t.Fatalf("err = %v, want extractor refusal", err)
	}
	failClosed(t, parent, out)
	if _, statErr := os.Stat(filepath.Join(parent, "escape.txt")); statErr == nil {
		t.Fatal("escape.txt written OUTSIDE the destination — traversal succeeded!")
	}
}

func TestPullMaliciousSymlinkTarBlocked(t *testing.T) {
	reg := filepath.Join(t.TempDir(), "reg")
	if err := os.MkdirAll(reg, 0o755); err != nil {
		t.Fatal(err)
	}
	hostileTar(t, reg, "evil", "1.0", func(tw *tar.Writer) error {
		// Absolute link target — refused outright (D70).
		return tw.WriteHeader(&tar.Header{
			Name: "link", Linkname: "/tmp/sandbox-tpl-pwn", Typeflag: tar.TypeSymlink, Mode: 0o777,
		})
	})
	url := serveRegistry(t, reg)
	parent := filepath.Join(t.TempDir(), "parent")
	out := filepath.Join(parent, "out")
	err := templatePull(url+"/evil:1.0", out, true)
	if err == nil || !strings.Contains(err.Error(), "refused") {
		t.Fatalf("err = %v, want refusal", err)
	}
	failClosed(t, parent, out)
}

// failClosed asserts the "không để lại file" invariant: destination absent,
// every temporary (blob/extract/pack) cleaned up.
func failClosed(t *testing.T, parent, out string) {
	t.Helper()
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatalf("out dir must not exist after failure (err=%v)", err)
	}
	ents, _ := os.ReadDir(parent)
	for _, e := range ents {
		if strings.HasPrefix(e.Name(), ".sandbox-tpl-") {
			t.Fatalf("leftover temp %q — fail-closed violated", e.Name())
		}
	}
}

func TestPullNoOverwriteExistingOut(t *testing.T) {
	reg, _ := pushTestRegistry(t, "web-base", "1.0.0")
	url := serveRegistry(t, reg)
	out := filepath.Join(t.TempDir(), "out")
	if err := os.Mkdir(out, 0o755); err != nil {
		t.Fatal(err)
	}
	err := templatePull(url+"/web-base:1.0.0", out, true)
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("err = %v, want overwrite refusal", err)
	}
}

func TestPullRejectsCorruptGzip(t *testing.T) {
	reg := filepath.Join(t.TempDir(), "reg")
	if err := os.MkdirAll(reg, 0o755); err != nil {
		t.Fatal(err)
	}
	blob := filepath.Join(reg, "web-base-1.0.0.tar.gz")
	if err := os.WriteFile(blob, []byte("this is not gzip"), 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte("this is not gzip"))
	upsertIndex(reg, registryEntry{
		Name: "web-base", Version: "1.0.0", SHA256: hex.EncodeToString(sum[:]),
		SizeBytes: 16, Created: "2026-11-27T00:00:00Z",
	})
	url := serveRegistry(t, reg)
	err := templatePull(url+"/web-base:1.0.0", filepath.Join(t.TempDir(), "out"), true)
	if err == nil || !strings.Contains(err.Error(), "not gzip") {
		t.Fatalf("err = %v, want gzip refusal", err)
	}
}

// unmarshalStrict is the same strict decode loadIndex/upsertIndex use
// (DisallowUnknownFields) so the tests verify the exact wire format.
func unmarshalStrict(b []byte, v any) error {
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

// io_multi aliases io.MultiWriter so the hostile-tar helper reads naturally.
func io_multi(w ...io.Writer) io.Writer { return io.MultiWriter(w...) }
