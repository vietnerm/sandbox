// Package firecracker — rootfs.go (GĐ4 mục 1, issue #193).
//
// ext4 rootfs image builder: creates an ext4 filesystem image from a
// directory tree. Firecracker requires a raw ext4 drive (not a directory).
// The builder uses mkfs.ext4 (host tool) + copy via loop mount OR
// debugfs (no root needed with ext4fuse/e2tools — but for simplicity,
// the initial implementation uses mkfs.ext4 + ext2fs copy which needs
// the host to have e2fsprogs).

package firecracker

import (
	"fmt"
	"os/exec"
	"path/filepath"
)

// BuildRootfs creates an ext4 image at dstPath from the contents of srcDir.
// sizeMib is the image size in MiB (minimum 64).
// Uses mkfs.ext4 + debugfs (no root, no loop mount — debugfs can copy files
// into an ext4 image without mounting).
func BuildRootfs(srcDir, dstPath string, sizeMib int) error {
	if sizeMib < 64 {
		sizeMib = 64
	}
	// 1. Create a sparse file of the right size.
	if err := exec.Command("truncate", "-s", fmt.Sprintf("%dM", sizeMib), dstPath).Run(); err != nil {
		return fmt.Errorf("firecracker rootfs: truncate: %w", err)
	}
	// 2. Format as ext4.
	if out, err := exec.Command("mkfs.ext4", "-q", "-F", "-d", srcDir, dstPath).CombinedOutput(); err != nil {
		return fmt.Errorf("firecracker rootfs: mkfs.ext4: %w: %s", err, out)
	}
	return nil
}

// RootfsPath returns the path to the ext4 rootfs image for one microVM.
func RootfsPath(stateDir, name string) string {
	return filepath.Join(stateDir, name, "rootfs.ext4")
}
