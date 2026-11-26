package main

// listener_test.go — unit tests for the S4 fail-closed unix-socket listener
// (issue #116, GAPS S4). Every assertion here has a mutant twin in the PR
// (M1 loose socket chmod, M2 skip dir chmod, M3 symlink accepted) so the
// suite cannot pass vacuously.

import (
	"net"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func socketPerm(t *testing.T, path string) os.FileMode {
	t.Helper()
	fi, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("lstat %s: %v", path, err)
	}
	return fi.Mode().Perm()
}

func dirPerm(t *testing.T, path string) os.FileMode {
	t.Helper()
	fi, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("lstat dir %s: %v", path, err)
	}
	return fi.Mode().Perm()
}

// Happy path: dir 0700 even when pre-created looser, socket 0600 after
// listen (umask-proof: Go tests run with a wide umask), dial works.
func TestOpenUnixSocketHappy(t *testing.T) {
	dir := t.TempDir()
	// Pre-create the socket dir LOOSER (0755) — openUnixSocket must fix it.
	if err := os.MkdirAll(filepath.Join(dir, "run"), 0o755); err != nil {
		t.Fatal(err)
	}
	sock := filepath.Join(dir, "run", "minictd.sock")

	ln, err := openUnixSocket(sock)
	if err != nil {
		t.Fatalf("openUnixSocket: %v", err)
	}
	defer ln.Close()

	if got := socketPerm(t, sock); got != 0o600 {
		t.Fatalf("socket perm = %o, want 600", got)
	}
	if got := dirPerm(t, filepath.Join(dir, "run")); got != 0o700 {
		t.Fatalf("dir perm = %o, want 700", got)
	}

	c, err := net.Dial("unix", sock)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	c.Close()
}

// A stale socket from a previous run is replaced (unlink + rebind). The
// stale socket is created with raw bind (a real crashed-daemon leftover:
// net.Listen "unix" unlinks on Close in modern Go, which would NOT leave
// one behind — this is exactly the case the raw path covers).
func TestOpenUnixSocketStaleSocketReplaced(t *testing.T) {
	dir := t.TempDir()
	sock := filepath.Join(dir, "minictd.sock")

	fd, err := syscall.Socket(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := syscall.Bind(fd, &syscall.SockaddrUnix{Name: sock}); err != nil {
		syscall.Close(fd)
		t.Fatal(err)
	}
	syscall.Close(fd)
	if _, err := os.Lstat(sock); err != nil {
		t.Fatalf("stale socket missing: %v", err)
	}

	ln, err := openUnixSocket(sock)
	if err != nil {
		t.Fatalf("openUnixSocket over stale: %v", err)
	}
	defer ln.Close()
	if got := socketPerm(t, sock); got != 0o600 {
		t.Fatalf("socket perm = %o, want 600", got)
	}
}

// Symlink at the socket path is refused (hijack class, D31 posture).
func TestOpenUnixSocketRefusesSymlink(t *testing.T) {
	dir := t.TempDir()
	sock := filepath.Join(dir, "minictd.sock")
	if err := os.Symlink("/nonexistent/target", sock); err != nil {
		t.Fatal(err)
	}
	ln, err := openUnixSocket(sock)
	if err == nil {
		ln.Close()
		t.Fatal("symlink accepted — must refuse")
	}
}

// Non-socket file at the socket path is refused (no silent overwrite).
func TestOpenUnixSocketRefusesRegularFile(t *testing.T) {
	dir := t.TempDir()
	sock := filepath.Join(dir, "minictd.sock")
	if err := os.WriteFile(sock, []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	ln, err := openUnixSocket(sock)
	if err == nil {
		ln.Close()
		t.Fatal("regular file accepted — must refuse")
	}
	if fi, err := os.Lstat(sock); err != nil || fi.Mode().IsRegular() == false {
		t.Fatal("refused file must be left untouched")
	}
}

// Dir that is not a directory is refused.
func TestOpenUnixSocketRefusesFileAsDir(t *testing.T) {
	dir := t.TempDir()
	blocked := filepath.Join(dir, "blocked")
	if err := os.WriteFile(blocked, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := openUnixSocket(filepath.Join(blocked, "minictd.sock")); err == nil {
		t.Fatal("file-as-dir accepted — must refuse")
	}
}

// Empty path refuses (fail-closed — never guess a socket location).
func TestOpenUnixSocketEmptyPath(t *testing.T) {
	if _, err := openUnixSocket(""); err == nil {
		t.Fatal("empty path accepted — must refuse")
	}
}

// Issue #119: the socket is CREATED 0600 (umask around bind) — even with a
// fully open process umask, and the caller's umask is restored afterwards.
// Mutant twin: drop the Umask call -> socket 0777 &^ 0 = 0777 -> fails here.
func TestOpenUnixSocketUmaskIndependent(t *testing.T) {
	prev := syscall.Umask(0)
	defer syscall.Umask(prev)
	sock := filepath.Join(t.TempDir(), "minictd.sock")
	ln, err := openUnixSocket(sock)
	if err != nil {
		t.Fatalf("openUnixSocket: %v", err)
	}
	defer ln.Close()
	if got := socketPerm(t, sock); got != 0o600 {
		t.Fatalf("socket perm = %o, want 600 with umask 0", got)
	}
	if cur := syscall.Umask(0); cur != 0 {
		t.Fatalf("umask not restored: got %o, want 0", cur)
	}
}

// Issue #119: bind never follows/replaces an existing path, so a name
// planted between Remove and bind makes the listener fail closed. Simulated
// with a dangling symlink at the path AFTER the stale-socket check would run:
// net.ListenUnix itself must refuse it (EADDRINUSE).
func TestListenUnixRefusesPlantedSymlink(t *testing.T) {
	dir := t.TempDir()
	sock := filepath.Join(dir, "minictd.sock")
	if err := os.Symlink(filepath.Join(dir, "elsewhere.sock"), sock); err != nil {
		t.Fatal(err)
	}
	if ln, err := net.ListenUnix("unix", &net.UnixAddr{Name: sock, Net: "unix"}); err == nil {
		ln.Close()
		t.Fatal("bind over a planted symlink succeeded — #119 TOCTOU argument broken")
	}
	if _, err := os.Lstat(filepath.Join(dir, "elsewhere.sock")); err == nil {
		t.Fatal("bind followed the symlink and created the target")
	}
}
