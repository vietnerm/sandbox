package minict

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// snapshot.go — trusted tar.gz backup/restore for GĐ2 mục 3 (issue #130, D57).
//
// Distinct from secureUnpack (D50, untrusted OCI/operator tar): snapshots are
// created by the daemon from its OWN rootfs and restored by the daemon. The
// rootfs legitimately contains ABSOLUTE symlinks (busybox applet pattern:
// /bin/ash -> /bin/busybox, 271 in the alpine base) which secureUnpack refuses
// (issue #109, untrusted absolute-symlink posture). The trusted path here
// allows absolute symlink TARGETS (they resolve inside the sandbox after
// pivot_root) while still refusing `..`/absolute member PATHS (path traversal
// defense — never write outside dst).
//
// gzip (Go stdlib compress/gzip) is used instead of zstd (DoD says tar.zst):
// the repo is stdlib-only (go.mod has no deps, D20 no-vendor pattern) and Go
// stdlib has no zstd. gzip is a deliberate, documented deviation; zstd needs a
// dep decision (deferred).

// tarGzTree writes a gzip-compressed tar of the src directory tree to w.
// Symlinks are preserved (Lstat, TypeSymlink + Linkname); regular files,
// directories, and their modes are preserved. Device/fifo/special entries are
// not expected (rootless rootfs has none) and skipped if present.
func tarGzTree(src string, w io.Writer) error {
	gw := gzip.NewWriter(w)
	defer gw.Close()
	tw := tar.NewWriter(gw)
	defer tw.Close()
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		rel = filepath.ToSlash(rel) // tar paths use forward slashes
		hdr, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return fmt.Errorf("tar header %s: %w", rel, err)
		}
		hdr.Name = rel
		// Symlinks: FileInfoHeader sets TypeSymlink but Linkname from the
		// passed "" arg; read the real target so the link survives round-trip.
		if info.Mode()&os.ModeSymlink != 0 {
			link, err := os.Readlink(path)
			if err != nil {
				return fmt.Errorf("readlink %s: %w", rel, err)
			}
			hdr.Linkname = link
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return fmt.Errorf("tar write %s: %w", rel, err)
		}
		if info.Mode().IsRegular() {
			f, err := os.Open(path)
			if err != nil {
				return fmt.Errorf("open %s: %w", rel, err)
			}
			_, copyErr := io.Copy(tw, f)
			f.Close()
			if copyErr != nil {
				return fmt.Errorf("tar copy %s: %w", rel, copyErr)
			}
		}
		return nil
	})
}

// extractGzTar gunzip-decompresses r and extracts the tar into dst (trusted
// path — see file header). Member paths are cleaned and must be relative
// without `..` (path traversal defense). Symlink targets are NOT validated
// (trusted content: the daemon created the snapshot; absolute targets like
// /bin/busybox are intentional and resolve inside the sandbox). Device/fifo
// entries are skipped (rootless can't create them; a trusted rootfs has none).
func extractGzTar(r io.Reader, dst string) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("snapshot gunzip: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("snapshot tar read: %w", err)
		}
		clean := filepath.Clean(hdr.Name)
		if strings.HasPrefix(clean, "/") || clean == ".." ||
			strings.HasPrefix(clean, "../") || strings.Contains(clean, "/../") {
			return fmt.Errorf("snapshot tar: unsafe member path %q", hdr.Name)
		}
		target := filepath.Join(dst, clean)
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(hdr.Mode)&0777); err != nil {
				return fmt.Errorf("snapshot mkdir %s: %w", clean, err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC,
				os.FileMode(hdr.Mode)&0777)
			if err != nil {
				return fmt.Errorf("snapshot create %s: %w", clean, err)
			}
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return fmt.Errorf("snapshot write %s: %w", clean, err)
			}
			f.Close()
		case tar.TypeSymlink:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			_ = os.Remove(target) // replace existing (idempotent restore)
			if err := os.Symlink(hdr.Linkname, target); err != nil {
				return fmt.Errorf("snapshot symlink %s -> %s: %w", clean, hdr.Linkname, err)
			}
		case tar.TypeLink:
			// Hard link — resolve within dst (trusted). Source may not yet be
			// extracted (ordering); skip non-fatally on error (rare in rootfs).
			linkSrc := filepath.Join(dst, filepath.Clean(hdr.Linkname))
			_ = os.Remove(target)
			_ = os.Link(linkSrc, target)
		default:
			// TypeCharDevice/TypeBlockDevice/TypeFifo/other: skip (rootless
			// rootfs has none; trusted content shouldn't carry them).
		}
	}
	return nil
}
