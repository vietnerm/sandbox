package main

// listener.go — S4 unix-socket listener with fail-closed permission hardening
// (issue #116, GAPS S4).
//
// Threat model (GAPS.md S4): the control-plane API on plain TCP lets any
// local user that knows the port connect (loopback is shared); the bearer
// token is the only gate and travels in clear. A pathname unix socket with
// socket mode 0600 inside a 0700 directory restricts connections to the
// daemon's own uid (unix(7): pathname sockets honor directory permissions;
// podman rootless service uses the same model — research.md Phiên 18).
//
// Fail-closed rules enforced here (every violation aborts the listener):
//   - the socket path must not be a symlink (hijack class, D31 O_NOFOLLOW
//     posture from issue #41) and must not be a non-socket file;
//   - a stale socket left by a previous daemon run is replaced (unlink + rebind);
//   - the parent directory must end up mode 0700 (chmod applied when looser;
//     chmod failure aborts);
//   - the socket file is CREATED 0600 (umask 0177 around bind — no chmod by
//     path afterwards) and verified via Lstat after listen: must be a socket,
//     not a symlink, mode 0600 (wrong final state aborts — never serve a
//     loose socket).
//
// TOCTOU (issue #119, SIA PR #118 E.1): between Lstat→Remove→bind a same-uid
// process could swap the path. bind(2) never follows or replaces an existing
// path (EADDRINUSE on any file, symlinks included), so a swap before bind
// makes the listener fail closed. The remaining window was the post-bind
// os.Chmod, which FOLLOWS symlinks (chmod 0600 on an attacker-chosen target);
// creating the socket with the right mode via umask removes that call. A swap
// after the final Lstat cannot redirect the already-bound fd (it only
// unlinks the name = same-uid DoS, outside the S4 threat model).
//
// The bearer token stays REQUIRED on every transport (R3, issue #116):
// same-uid processes can still connect to the socket, so auth remains the
// defense-in-depth layer and the single auth code path is kept.

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"syscall"
)

// openUnixSocket creates a hardened unix listener at path.
func openUnixSocket(path string) (*net.UnixListener, error) {
	if path == "" {
		return nil, fmt.Errorf("unix socket: empty path")
	}

	// Parent directory: exists as a dir, ends up 0700 (fail-closed chmod).
	dir := dirOf(path)
	fi, err := os.Lstat(dir)
	if err != nil {
		return nil, fmt.Errorf("unix socket: stat dir %s: %w", dir, err)
	}
	if !fi.IsDir() {
		return nil, fmt.Errorf("unix socket: %s is not a directory", dir)
	}
	if permOf(fi.Mode()) != 0o700 {
		if err := os.Chmod(dir, 0o700); err != nil {
			return nil, fmt.Errorf("unix socket: chmod dir %s 0700: %w", dir, err)
		}
		fi2, err := os.Lstat(dir)
		if err != nil {
			return nil, fmt.Errorf("unix socket: re-stat dir %s: %w", dir, err)
		}
		if permOf(fi2.Mode()) != 0o700 {
			return nil, fmt.Errorf("unix socket: dir %s did not end up 0700", dir)
		}
	}

	// Socket path hygiene: symlink/non-socket files are refused; a stale
	// socket from a previous run is replaced.
	if fi, err := os.Lstat(path); err == nil {
		switch {
		case fi.Mode()&os.ModeSymlink != 0:
			return nil, fmt.Errorf("unix socket: %s is a symlink — refusing (D31 posture)", path)
		case fi.Mode()&os.ModeSocket == 0:
			return nil, fmt.Errorf("unix socket: %s exists and is not a socket — refusing", path)
		default:
			// Stale socket: previous daemon died without cleanup. The
			// unlink is safe — we verified it IS a socket (no race with a
			// swap inside our own uid matters: the new listener rebinds
			// the same path).
			if err := os.Remove(path); err != nil {
				return nil, fmt.Errorf("unix socket: remove stale socket: %w", err)
			}
		}
	}

	// Socket file created 0600 directly (issue #119): umask is process-wide,
	// which is fine here — main opens listeners BEFORE any background work.
	old := syscall.Umask(0o177)
	ln, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	syscall.Umask(old)
	if err != nil {
		return nil, fmt.Errorf("unix socket: listen %s: %w", path, err)
	}
	// Until verified, a failure Close must not unlink by name: the path may
	// already point at someone else's file (swap after bind).
	ln.SetUnlinkOnClose(false)

	fi, err = os.Lstat(path)
	if err != nil {
		ln.Close()
		return nil, fmt.Errorf("unix socket: verify socket: %w", err)
	}
	if fi.Mode()&os.ModeSocket == 0 || fi.Mode()&os.ModeSymlink != 0 {
		ln.Close()
		return nil, fmt.Errorf("unix socket: %s is not our socket after bind — refusing", path)
	}
	if permOf(fi.Mode()) != 0o600 {
		ln.Close()
		return nil, fmt.Errorf("unix socket: socket did not end up 0600 (mode %o)", permOf(fi.Mode()))
	}
	ln.SetUnlinkOnClose(true)
	return ln, nil
}

func dirOf(p string) string {
	return filepath.Dir(p)
}

func permOf(m os.FileMode) os.FileMode { return m & os.ModePerm }
