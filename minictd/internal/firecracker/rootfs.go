// Package firecracker — rootfs.go (GĐ4 mục 1b.b, issue #235).
//
// ext4 rootfs image builder: creates an ext4 filesystem image from a
// directory tree. Firecracker requires a raw ext4 drive (not a directory).
//
// Rootless: uses `mkfs.ext4 -d srcDir` (populate at format time — no mount,
// no loop device, no root). Verification: `e2fsck -n -f` (integrity check,
// read-only) + `debugfs -R "ls /"` (root listing matches srcDir). Size
// limit: max 512 MiB default (configurable) — refuse oversized images.
//
// Compiles without KVM. Tools (mkfs.ext4, e2fsck, debugfs) from e2fsprogs —
// standard on ubuntu-24.04 CI runner. Dev machine without e2fsprogs →
// tests SKIP (env-gated).

package firecracker

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// MaxRootfsMib is the default maximum rootfs image size (512 MiB).
// Oversized images are refused to prevent disk exhaustion.
const MaxRootfsMib = 512

// MinRootfsMib is the minimum rootfs image size (Firecracker needs enough
// for the ext4 metadata + the rootfs content).
const MinRootfsMib = 64

// BuildRootfs creates an ext4 image at dstPath from the contents of srcDir.
// sizeMib is the image size in MiB (clamped to [MinRootfsMib, MaxRootfsMib]).
// Uses mkfs.ext4 -d (no mount, no root). Verifies with e2fsck + debugfs.
// Fails closed if any step fails — the image is removed on error.
func BuildRootfs(srcDir, dstPath string, sizeMib int) error {
	if sizeMib < MinRootfsMib {
		sizeMib = MinRootfsMib
	}
	if sizeMib > MaxRootfsMib {
		return fmt.Errorf("firecracker rootfs: size %d MiB exceeds max %d", sizeMib, MaxRootfsMib)
	}

	// Check tools are available.
	for _, tool := range []string{"truncate", "mkfs.ext4", "e2fsck", "debugfs"} {
		if _, err := exec.LookPath(tool); err != nil {
			return fmt.Errorf("firecracker rootfs: %s not found (install e2fsprogs)", tool)
		}
	}

	// Check srcDir exists and is a directory.
	fi, err := os.Stat(srcDir)
	if err != nil {
		return fmt.Errorf("firecracker rootfs: src dir: %w", err)
	}
	if !fi.IsDir() {
		return fmt.Errorf("firecracker rootfs: src %s is not a directory", srcDir)
	}

	// 1. Create a sparse file of the right size.
	if err := exec.Command("truncate", "-s", fmt.Sprintf("%dM", sizeMib), dstPath).Run(); err != nil {
		return fmt.Errorf("firecracker rootfs: truncate: %w", err)
	}

	// 2. Format as ext4 with contents from srcDir (-d populates at format time,
	//    no mount needed, no root).
	mkfs := exec.Command("mkfs.ext4", "-q", "-F", "-d", srcDir, dstPath)
	if out, err := mkfs.CombinedOutput(); err != nil {
		os.Remove(dstPath)
		return fmt.Errorf("firecracker rootfs: mkfs.ext4: %w: %s", err, out)
	}

	// 3. Verify: e2fsck -n -f (read-only integrity check).
	e2fsck := exec.Command("e2fsck", "-n", "-f", dstPath)
	if out, err := e2fsck.CombinedOutput(); err != nil {
		// e2fsck exits non-zero on errors (exit code 1 = errors found,
		// 2 = errors corrected, 4+ = fatal). -n means no corrections.
		os.Remove(dstPath)
		return fmt.Errorf("firecracker rootfs: e2fsck failed: %w: %s", err, out)
	}

	// 4. Verify: debugfs -R "ls /" (root directory listing).
	debugfs := exec.Command("debugfs", "-R", "ls /", dstPath)
	dbgOut, err := debugfs.CombinedOutput()
	if err != nil {
		os.Remove(dstPath)
		return fmt.Errorf("firecracker rootfs: debugfs failed: %w: %s", err, dbgOut)
	}
	// Verify at least one expected file from srcDir appears in the listing.
	entries, err := os.ReadDir(srcDir)
	if err == nil && len(entries) > 0 {
		listing := string(dbgOut)
		found := false
		for _, e := range entries {
			if strings.Contains(listing, e.Name()) {
				found = true
				break
			}
		}
		if !found {
			os.Remove(dstPath)
			return fmt.Errorf("firecracker rootfs: debugfs root listing does not match srcDir")
		}
	}

	return nil
}

// RootfsPath returns the path to the ext4 rootfs image for one microVM.
func RootfsPath(stateDir, name string) string {
	return filepath.Join(stateDir, name, "rootfs.ext4")
}

// toolsAvailable returns true if all ext4 tools are on PATH.
func toolsAvailable() bool {
	for _, tool := range []string{"truncate", "mkfs.ext4", "e2fsck", "debugfs"} {
		if _, err := exec.LookPath(tool); err != nil {
			return false
		}
	}
	return true
}
