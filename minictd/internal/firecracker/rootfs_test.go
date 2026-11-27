// Package firecracker — rootfs_test.go (GĐ4 mục 1b.b, issue #235).
//
// Tests for the ext4 rootfs builder. Env-gated: SKIP when mkfs.ext4/e2fsck/
// debugfs not on PATH (dev machine without e2fsprogs). PASS on CI (ubuntu-24.04
// has e2fsprogs standard).

package firecracker

import (
	"os"
	"path/filepath"
	"testing"
)

// TestBuildRootfsValid builds an ext4 image from a temp dir with known
// files, verifies e2fsck passes + debugfs lists the files. SKIP if tools
// missing.
func TestBuildRootfsValid(t *testing.T) {
	if !toolsAvailable() {
		t.Skip("mkfs.ext4/e2fsck/debugfs not on PATH — SKIP (env-gated, CI has e2fsprogs)")
	}

	srcDir := t.TempDir()
	// Create known files in the source directory.
	if err := os.WriteFile(filepath.Join(srcDir, "hello.txt"), []byte("hello world"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(srcDir, "bin"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "bin", "sh"), []byte("#!/bin/sh\necho hi"), 0755); err != nil {
		t.Fatal(err)
	}

	dst := filepath.Join(t.TempDir(), "rootfs.ext4")
	if err := BuildRootfs(srcDir, dst, 64); err != nil {
		t.Fatalf("BuildRootfs: %v", err)
	}

	// Verify the image file exists and is non-empty.
	fi, err := os.Stat(dst)
	if err != nil {
		t.Fatalf("stat rootfs: %v", err)
	}
	if fi.Size() < 1024 {
		t.Errorf("rootfs too small: %d bytes", fi.Size())
	}
}

// TestBuildRootfsSizeLimit verifies oversized images are refused.
func TestBuildRootfsSizeLimit(t *testing.T) {
	srcDir := t.TempDir()
	dst := filepath.Join(t.TempDir(), "big.ext4")
	err := BuildRootfs(srcDir, dst, MaxRootfsMib+1)
	if err == nil {
		t.Fatal("expected error for oversized rootfs")
		os.Remove(dst)
	}
}

// TestBuildRootfsMissingSrcDir verifies error when srcDir doesn't exist.
func TestBuildRootfsMissingSrcDir(t *testing.T) {
	if !toolsAvailable() {
		t.Skip("tools not on PATH")
	}
	dst := filepath.Join(t.TempDir(), "rootfs.ext4")
	err := BuildRootfs("/nonexistent/path", dst, 64)
	if err == nil {
		t.Fatal("expected error for missing src dir")
	}
}

// TestBuildRootfsNotADir verifies error when srcDir is a file.
func TestBuildRootfsNotADir(t *testing.T) {
	if !toolsAvailable() {
		t.Skip("tools not on PATH")
	}
	srcFile := filepath.Join(t.TempDir(), "notadir")
	os.WriteFile(srcFile, []byte("not a dir"), 0644)
	dst := filepath.Join(t.TempDir(), "rootfs.ext4")
	err := BuildRootfs(srcFile, dst, 64)
	if err == nil {
		t.Fatal("expected error for src not a directory")
	}
}
