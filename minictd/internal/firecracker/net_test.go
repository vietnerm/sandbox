// Package firecracker — net_test.go (GĐ4 mục 1b.c, issue #237).
//
// Tests for rootless TAP device management. Creates a temporary netns via
// `unshare -n`, verifies TAP create/verify/delete lifecycle. SKIP if
// nsenter/ip not available or userns restricted (dev 5.10), PASS on CI
// kernel 7.x with userns.

package firecracker

import (
	"os/exec"
	"strings"
	"testing"
	"time"
)

// startTempNetns creates a temporary netns by running `unshare -n sleep 30 &`.
// Returns the PID of the process holding the netns open, or skips the test.
func startTempNetns(t *testing.T) int {
	t.Helper()
	if !netToolsAvailable() {
		t.Skip("nsenter/ip not on PATH — SKIP (env-gated)")
	}
	if !canUnshareNetns() {
		t.Skip("unshare -n fails — userns restricted (dev 5.10) — SKIP")
	}
	cmd := exec.Command("unshare", "-n", "sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Skipf("unshare start failed: %v — SKIP", err)
	}
	pid := cmd.Process.Pid
	// Give the netns a moment to stabilize
	time.Sleep(100 * time.Millisecond)
	// Register cleanup
	t.Cleanup(func() {
		cmd.Process.Kill()
		cmd.Wait()
	})
	return pid
}

func TestCreateAndDeleteTAP(t *testing.T) {
	pid := startTempNetns(t)
	tapName := "fc-test0"

	// Create TAP
	if err := CreateTAPInNetns(pid, tapName); err != nil {
		t.Fatalf("CreateTAP: %v", err)
	}

	// Verify TAP exists
	if !VerifyTAPInNetns(pid, tapName) {
		t.Errorf("TAP %s not found after create", tapName)
	}

	// Delete TAP
	if err := DeleteTAPInNetns(pid, tapName); err != nil {
		t.Fatalf("DeleteTAP: %v", err)
	}

	// Verify TAP gone
	if VerifyTAPInNetns(pid, tapName) {
		t.Errorf("TAP %s still exists after delete", tapName)
	}
}

func TestDeleteTAPIdempotent(t *testing.T) {
	pid := startTempNetns(t)
	tapName := "fc-idempotent0"

	// Delete a TAP that was never created — should not error
	if err := DeleteTAPInNetns(pid, tapName); err != nil {
		t.Errorf("DeleteTAP (never created) should be idempotent, got: %v", err)
	}
}

func TestVerifyTAPNonExistent(t *testing.T) {
	pid := startTempNetns(t)
	if VerifyTAPInNetns(pid, "fc-nonexistent0") {
		t.Error("VerifyTAP returned true for nonexistent device")
	}
}

func TestNsenterNetns(t *testing.T) {
	pid := startTempNetns(t)
	// Verify we can run commands in the netns
	out, err := nsenterNetns(pid, "ip", "link", "show")
	if err != nil {
		t.Fatalf("nsenter ip link show: %v: %s", err, out)
	}
	// A fresh netns should have at least the loopback interface
	if !strings.Contains(out, "lo") {
		t.Errorf("netns should have loopback, got: %s", out)
	}
}
