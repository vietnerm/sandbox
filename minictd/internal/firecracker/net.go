// Package firecracker — net.go (GĐ4 mục 1b.c, issue #237).
//
// Rootless TAP device management for Firecracker microVM networking.
// The daemon owns a netns via userns (CAP_NET_ADMIN available inside).
// TAP devices are created/deleted inside that netns using ip(8) commands
// via nsenter(1).
//
// Firecracker connects to the TAP as a host_dev_name in its
// /network-interfaces API. The guest sees a virtio-net device backed by
// the TAP on the host side.
//
// Compiles without KVM. Tests create a temporary netns (unshare -n) and
// verify TAP lifecycle — SKIP if userns/ip/nsenter not available.

package firecracker

import (
	"fmt"
	"os/exec"
	"strings"
)

// CreateTAPInNetns creates a TAP device named tapName inside the netns
// of process pid (the jail instance). The daemon must own the netns
// (via userns) to have CAP_NET_ADMIN.
func CreateTAPInNetns(pid int, tapName string) error {
	// ip tuntap add <tapName> mode tap
	if out, err := nsenterNetns(pid, "ip", "tuntap", "add", tapName, "mode", "tap"); err != nil {
		return fmt.Errorf("firecracker net: create TAP %s: %w: %s", tapName, err, out)
	}
	// ip link set <tapName> up
	if out, err := nsenterNetns(pid, "ip", "link", "set", tapName, "up"); err != nil {
		return fmt.Errorf("firecracker net: up TAP %s: %w: %s", tapName, err, out)
	}
	return nil
}

// DeleteTAPInNetns deletes a TAP device inside the netns of process pid.
// Idempotent: no error if the device doesn't exist.
func DeleteTAPInNetns(pid int, tapName string) error {
	_, err := nsenterNetns(pid, "ip", "link", "del", tapName)
	if err != nil {
		// Check if it's already gone
		if !VerifyTAPInNetns(pid, tapName) {
			return nil // already deleted — idempotent
		}
		return fmt.Errorf("firecracker net: delete TAP %s: %w", tapName, err)
	}
	return nil
}

// VerifyTAPInNetns returns true if a TAP device named tapName exists
// inside the netns of process pid.
func VerifyTAPInNetns(pid int, tapName string) bool {
	out, err := nsenterNetns(pid, "ip", "link", "show", tapName)
	if err != nil {
		return false
	}
	return strings.Contains(out, tapName)
}

// nsenterNetns runs a command inside the netns of process pid.
// Returns combined stdout+stderr.
func nsenterNetns(pid int, name string, args ...string) (string, error) {
	cmdArgs := append([]string{"-t", fmt.Sprintf("%d", pid), "-n", "--", name}, args...)
	out, err := exec.Command("nsenter", cmdArgs...).CombinedOutput()
	return string(out), err
}

// netToolsAvailable returns true if nsenter and ip are on PATH.
func netToolsAvailable() bool {
	for _, tool := range []string{"nsenter", "ip"} {
		if _, err := exec.LookPath(tool); err != nil {
			return false
		}
	}
	return true
}

// canUnshareNetns returns true if `unshare -n` works (userns not restricted).
func canUnshareNetns() bool {
	// Try to create a temporary netns. If unshare fails, userns is restricted.
	cmd := exec.Command("unshare", "-n", "true")
	return cmd.Run() == nil
}
