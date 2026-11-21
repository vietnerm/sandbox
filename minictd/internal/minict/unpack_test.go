package minict

import (
	"archive/tar"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// buildTar assembles an in-memory tar from a list of entries. Every case in
// this file mirrors a MEASURED probe from journal/2026-11-21/research.md —
// the hostile entries are the exact vectors tar 1.35 was measured against.
type tentry struct {
	name, data, link string
	typ              byte
	mode             int64
}

func buildTar(t *testing.T, entries []tentry) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, e := range entries {
		typ := e.typ
		if typ == 0 {
			typ = tar.TypeReg
		}
		hdr := &tar.Header{
			Name:     e.name,
			Mode:     e.mode,
			Typeflag: typ,
			Linkname: e.link,
			Size:     int64(len(e.data)),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("write header %q: %v", e.name, err)
		}
		if len(e.data) > 0 && typ == tar.TypeReg {
			if _, err := tw.Write([]byte(e.data)); err != nil {
				t.Fatalf("write data %q: %v", e.name, err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	return buf.Bytes()
}

func unpackTo(t *testing.T, entries []tentry) (dst string, err error) {
	t.Helper()
	dst = t.TempDir()
	err = secureUnpack(bytes.NewReader(buildTar(t, entries)), dst)
	return dst, err
}

// --- hostile: member names (measured: GNU tar 1.35 refuses ".."; busybox tar does NOT) ---
// Every refusal assertion targets OUR error message (provenance): the dev
// sandbox's LSM also blocks several of these operations (measured 2026-11-21:
// SYS_SYMLINK killed with "Creating symbolic links is not allowed.", suid
// stripped at create, re-open-for-write EACCES) — asserting our exact
// messages keeps the mutants caught HERE, not only on vanilla CI.

func TestUnpackRefusesDotdotMember(t *testing.T) { // U1 / mutant M1
	_, err := unpackTo(t, []tentry{{name: "../md104-escaped", data: "pwned"}})
	if err == nil || !strings.Contains(err.Error(), "path contains") {
		t.Fatalf("want our checkName refusal, got err=%v", err)
	}
}

func TestUnpackRefusesAbsoluteMember(t *testing.T) { // U2
	_, err := unpackTo(t, []tentry{{name: "/tmp/md104-abs-pwned", data: "pwned"}})
	if err == nil || !strings.Contains(err.Error(), "absolute path") {
		t.Fatalf("want our checkName refusal, got err=%v", err)
	}
}

func TestUnpackRefusesNestedDotdot(t *testing.T) { // U1b (inside subdir, harder to spot)
	_, err := unpackTo(t, []tentry{{name: "a/b/../../../md104-escaped", data: "pwned"}})
	if err == nil || !strings.Contains(err.Error(), "path contains") {
		t.Fatalf("want our checkName refusal, got err=%v", err)
	}
}

// --- hostile: symlink targets (measured: tar 1.35 creates dotdot-target symlinks rc=0) ---

func TestUnpackRefusesAbsoluteSymlinkTarget(t *testing.T) { // U3
	dst, err := unpackTo(t, []tentry{{name: "lnk", typ: tar.TypeSymlink, link: "/etc/passwd"}})
	if err == nil || !strings.Contains(err.Error(), "link target refused") {
		t.Fatalf("want our link-target refusal, got err=%v", err)
	}
	if _, lerr := os.Lstat(filepath.Join(dst, "lnk")); !os.IsNotExist(lerr) {
		t.Fatalf("symlink must not be created on refusal, lerr=%v", lerr)
	}
}

func TestUnpackRefusesDotdotSymlinkTarget(t *testing.T) { // U4 / mutant M2
	dst, err := unpackTo(t, []tentry{{name: "lnk", typ: tar.TypeSymlink, link: "../../md104-outside"}})
	if err == nil || !strings.Contains(err.Error(), "link target refused") {
		t.Fatalf("want our link-target refusal, got err=%v", err)
	}
	if _, lerr := os.Lstat(filepath.Join(dst, "lnk")); !os.IsNotExist(lerr) {
		t.Fatalf("symlink must not be created on refusal, lerr=%v", lerr)
	}
}

func TestUnpackRefusesWriteThroughPlantedSymlink(t *testing.T) { // U5 / mutant M3
	// fully LEGAL symlink entry (relative, in-tree) followed by a file THROUGH
	// it: the walk must fail on the symlink component (O_NOFOLLOW), never
	// write through it. Security property asserted directly: real/planted
	// must NOT exist. (Dev-sandbox note: the local LSM refuses the same
	// open — refusal reason differs by environment, the property does
	// not; on vanilla CI the M3 mutant is caught by the missing error.)
	dst, err := unpackTo(t, []tentry{
		{name: "real", typ: tar.TypeDir},
		{name: "real/lnk", typ: tar.TypeSymlink, link: "real"}, // legal target
		{name: "real/lnk/planted", data: "pwned"},              // refused
	})
	if err == nil || !strings.Contains(err.Error(), "component") {
		t.Fatalf("want component refusal, got err=%v", err)
	}
	if _, lerr := os.Lstat(filepath.Join(dst, "real", "planted")); !os.IsNotExist(lerr) {
		t.Fatalf("file must not exist through symlink, lerr=%v", lerr)
	}
}

func TestUnpackRefusesWriteThroughRelativeSymlinkDir(t *testing.T) { // U5b
	// relative in-rootfs symlink + write through it (the GNU-tar-1.35-blocked
	// vector) — here the SECOND entry must fail because walking a/lnk with
	// O_NOFOLLOW on a symlinked dir component is refused by construction.
	_, err := unpackTo(t, []tentry{
		{name: "real", typ: tar.TypeDir},
		{name: "real/lnk", typ: tar.TypeSymlink, link: "real/sub"}, // legal target
		{name: "real/sub/x", data: "ok"},                           // dir created first
		{name: "real/lnk/y", data: "through-symlink"},              // refused
	})
	if err == nil || !strings.Contains(err.Error(), "component") {
		t.Fatalf("want component refusal, got err=%v", err)
	}
}

// --- hostile: duplicates & overwrite (measured: tar 1.35 silently skips same-name REG after SYM) ---

func TestUnpackRefusesDuplicateFileOverwrite(t *testing.T) { // U6 / mutant M5
	// "file exists" (EEXIST) is OUR O_EXCL — the dev LSM ALSO denies
	// re-open-for-write with EACCES, so asserting the exact message
	// keeps the M5 mutant caught here, not only on vanilla CI.
	_, err := unpackTo(t, []tentry{
		{name: "f", data: "first"},
		{name: "f", data: "second"},
	})
	if err == nil || !strings.Contains(err.Error(), "file exists") {
		t.Fatalf("want EEXIST from our O_EXCL, got err=%v", err)
	}
}

// --- hostile: device nodes (rootless: mknod EPERM) ---

func TestUnpackRefusesCharDevice(t *testing.T) { // U11 / mutant M6
	_, err := unpackTo(t, []tentry{{name: "dev/console", typ: tar.TypeChar, mode: 0666}})
	if err == nil || !strings.Contains(err.Error(), "not creatable rootless") {
		t.Fatalf("want our device refusal, got err=%v", err)
	}
}

// --- hardened posture: setuid stripped (U7 / mutant M4) ---

func TestStripSpecialBits(t *testing.T) { // pure function — catches M4 even where
	// the host LSM strips suid at create time (dev sandbox, measured)
	for in, want := range map[int64]int64{
		04755: 0755, // setuid stripped
		02755: 0755, // setgid stripped
		01777: 0777, // sticky stripped (dirs use perm|0700 path anyway)
		0644:  0644,
		0755:  0755,
	} {
		if got := stripSpecialBits(in); got != want {
			t.Fatalf("stripSpecialBits(%#o) = %#o, want %#o", in, got, want)
		}
	}
}

func TestUnpackStripsSetuidSetgid(t *testing.T) {
	dst, err := unpackTo(t, []tentry{{name: "bin/su", data: "x", mode: 04755}})
	if err != nil {
		t.Fatalf("unpack: %v", err)
	}
	st, err := os.Stat(filepath.Join(dst, "bin", "su"))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if st.Mode()&os.ModeSetuid != 0 || st.Mode()&os.ModeSetgid != 0 {
		t.Fatalf("setuid/setgid must be stripped, got %v", st.Mode())
	}
}

// --- positive control: a normal rootfs tree round-trips byte-identical ---

func TestUnpackPositiveControl(t *testing.T) { // U8
	dst, err := unpackTo(t, []tentry{
		{name: "bin", typ: tar.TypeDir, mode: 0755},
		{name: "bin/sh", data: "\x7fELF-busybox", mode: 0755},
		{name: "etc", typ: tar.TypeDir, mode: 0755},
		{name: "etc/passwd", data: "root:x:0:0:root:/root:/bin/sh", mode: 0644},
		{name: "bin/busybox-link", typ: tar.TypeSymlink, link: "sh"}, // in-tree relative
		{name: "bin/hard-to-sh", typ: tar.TypeLink, link: "bin/sh"},  // hardlink to earlier file
		{name: "var/run", typ: tar.TypeFifo, mode: 0644},
	})
	if err != nil {
		t.Fatalf("unpack positive control: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dst, "etc", "passwd"))
	if err != nil || string(got) != "root:x:0:0:root:/root:/bin/sh" {
		t.Fatalf("content mismatch: %q err=%v", got, err)
	}
	sh, err := os.Stat(filepath.Join(dst, "bin", "sh"))
	if err != nil || sh.Mode().Perm() != 0755 {
		t.Fatalf("bin/sh mode: %v err=%v", sh, err)
	}
	// symlink resolves INSIDE dst only
	tgt, err := os.Readlink(filepath.Join(dst, "bin", "busybox-link"))
	if err != nil || tgt != "sh" {
		t.Fatalf("symlink: %q err=%v", tgt, err)
	}
	hard, err := os.Stat(filepath.Join(dst, "bin", "hard-to-sh"))
	if err != nil || hard.Size() != int64(len("\x7fELF-busybox")) {
		t.Fatalf("hardlink: %v err=%v", hard, err)
	}
	if st, err := os.Stat(filepath.Join(dst, "var", "run")); err != nil || st.Mode()&os.ModeNamedPipe == 0 {
		t.Fatalf("fifo: %v err=%v", st, err)
	}
}

func TestUnpackPositiveControlDeeperDirs(t *testing.T) { // U8b
	dst, err := unpackTo(t, []tentry{
		{name: "usr/lib/nginx/modules/empty.so", data: "m"},
		{name: "usr/lib/nginx/conf", typ: tar.TypeDir},
	})
	if err != nil {
		t.Fatalf("unpack: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dst, "usr/lib/nginx/modules", "empty.so")); err != nil {
		t.Fatalf("deep file: %v", err)
	}
	if fi, err := os.Stat(filepath.Join(dst, "usr/lib/nginx/conf")); err != nil || !fi.IsDir() {
		t.Fatalf("deep dir: %v err=%v", fi, err)
	}
}

// --- negative control of the test harness itself (anti-vacuous): a broken
// buildTar (no Close) must produce a corrupt-stream error, proving the
// assertions actually consume the stream. ---

func TestUnpackCorruptStreamRefused(t *testing.T) { // U12 (harness negative control)
	dst := t.TempDir()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	_ = tw.WriteHeader(&tar.Header{Name: "half", Size: 100, Typeflag: tar.TypeReg, Mode: 0644})
	_, _ = tw.Write([]byte("too-short")) // size lies: stream corrupt by design
	// intentionally NOT closing the writer
	err := secureUnpack(bytes.NewReader(buf.Bytes()), dst)
	if err == nil {
		t.Fatalf("want refusal on corrupt/truncated stream, got nil")
	}
}

// guard: checkName refuses exactly the measured vectors, accepts legal names.
func TestCheckNameMatrix(t *testing.T) {
	bad := []string{"../x", "/x", "a/../../x", "", "."}
	for _, b := range bad {
		if _, err := checkName(b); err == nil {
			t.Fatalf("checkName(%q) must refuse", b)
		}
	}
	good := []string{"bin/sh", "usr/lib/x.so", "./bin/sh", "a//b", "..."}
	for _, g := range good {
		if _, err := checkName(g); err != nil {
			t.Fatalf("checkName(%q) must accept, got %v", g, err)
		}
	}
}

// guard: fd discipline — a full unpack must not leak fds beyond the root fd
// (openParent closes the walk chain; extractEntry closes parents).
func TestUnpackFdDiscipline(t *testing.T) {
	before, err := countFds()
	if err != nil {
		t.Skipf("cannot count fds: %v", err)
	}
	dst := t.TempDir()
	entries := make([]tentry, 0, 60)
	for i := 0; i < 20; i++ {
		entries = append(entries,
			tentry{name: filepath.Join("d", string(rune('a'+i%26)), "f"+string(rune('0'+i/10)), "x"+string(rune('0'+i%10)), "file"), data: "v"})
	}
	if err := secureUnpack(bytes.NewReader(buildTar(t, entries)), dst); err != nil {
		t.Fatalf("unpack: %v", err)
	}
	after, err := countFds()
	if err != nil {
		t.Skipf("cannot count fds: %v", err)
	}
	if after-before > 2 {
		t.Fatalf("fd leak: before=%d after=%d", before, after)
	}
}

func countFds() (int, error) {
	entries, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		return 0, err
	}
	return len(entries), nil
}
