// Package firecracker — jailer.go (GĐ4 mục 1, issue #193).
//
// Jailer spawn: the Firecracker jailer isolates the VMM process (uid/gid,
// chroot, cgroup, netns). This file builds the jailer command line — it
// does NOT spawn the process (that happens in backend.go when KVM is
// available). Compiles without KVM.

package firecracker

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
)

// JailerArgs holds the arguments for the Firecracker jailer.
type JailerArgs struct {
	ID            string // microVM id (instance name)
	ExecFile      string // firecracker binary path
	UID           int    // drop to this uid
	GID           int    // drop to this gid
	ChrootBaseDir string // chroot base (jailer creates <base>/<id>/)
	NetNS         string // netns path (optional)
	APISocket     string // Firecracker API socket path (passed to firecracker)
}

// Command builds the jailer exec.Cmd from JailerArgs.
func (a JailerArgs) Command() *exec.Cmd {
	args := []string{
		"--id", a.ID,
		"--exec-file", a.ExecFile,
		"--uid", strconv.Itoa(a.UID),
		"--gid", strconv.Itoa(a.GID),
	}
	if a.ChrootBaseDir != "" {
		args = append(args, "--chroot-base-dir", a.ChrootBaseDir)
	}
	if a.NetNS != "" {
		args = append(args, "--netns", a.NetNS)
	}
	// Extra args passed to Firecracker itself (after --)
	args = append(args, "--",
		"--api-sock", a.APISocket,
	)
	return exec.Command("jailer", args...)
}

// Argv returns the jailer command line as a string slice (for test pinning).
func (a JailerArgs) Argv() []string {
	return a.Command().Args
}

// ChrootDir returns the chroot directory the jailer creates for this microVM.
func (a JailerArgs) ChrootDir() string {
	return filepath.Join(a.ChrootBaseDir, "firecracker", a.ID)
}

// String returns a human-readable description (for logging).
func (a JailerArgs) String() string {
	return fmt.Sprintf("jailer --id %s --uid %d --gid %d --exec-file %s", a.ID, a.UID, a.GID, a.ExecFile)
}
