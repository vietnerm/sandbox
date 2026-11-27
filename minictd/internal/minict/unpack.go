package minict

import (
	"archive/tar"
	"fmt"
	"io"
	"path"
	"strings"
	"syscall"
	"unsafe"
)

// secureUnpack extracts an untrusted tar stream into dst with a fail-closed
// posture that does NOT depend on the host tar flavor/version (issue #104,
// D50). Measured baseline (journal/2026-11-21/research.md): GNU tar 1.35
// refuses ".." members and blocks write-through-symlink, but busybox tar and
// older GNU tar do neither — oci:/tar: images must not lean on that.
//
// Guarantees (by construction, no external process, no host tar):
//   - member names: reject absolute paths and any ".." component (reject,
//     never sanitize — an image containing "../x" is refused outright).
//   - link targets (symlink/hardlink): same validation; a symlink planted by
//     the archive can only point at paths INSIDE dst, so it can never reach
//     host files. Absolute symlink targets (e.g. busybox --install -s pattern
//     `/bin/busybox`) are REFUSED — image builders must use relative links or
//     copies (D70, issue #109, owner 2026-11-26: keep refusal, do not sanitize).
//   - every create/walk goes through openat(O_NOFOLLOW) per path component:
//     the extractor never resolves through a symlink the archive planted
//     (TOCTOU-free; D31 pattern from the webproxy fix, issue #41).
//   - duplicates are refused (fresh dst + O_EXCL) — a second entry re-using a
//     name cannot overwrite anything; only a re-mention of an already-created
//     DIRECTORY is tolerated (docker images repeat "dir/" markers).
//   - setuid/setgid/sticky bits are stripped (capdrop+NNP neuter them inside
//     the sandbox anyway; they have no legitimate rootfs-boot role here).
//   - FIFOs are created (rootless-safe); character/block devices are refused
//     with an explicit error — mknod needs CAP_MKNOD and always fails EPERM
//     under the project's rootless invariant (PROMPT.MD §0).
//
// dst must exist and must be private to this extractor: prepareRootfs runs
// inside a freshly created instance dir under the create-lock, and Run rolls
// the dir back on any error (R4), so nothing else writes there concurrently.
func secureUnpack(r io.Reader, dst string) error {
	root, err := syscall.Open(dst, syscall.O_RDONLY|syscall.O_DIRECTORY, 0)
	if err != nil {
		return fmt.Errorf("secureUnpack: open dst %q: %w", dst, err)
	}
	defer syscall.Close(root)

	tr := tar.NewReader(r)
	buf := make([]byte, 256*1024) // io.CopyBuffer — parity with C tar on big layers
	n := 0
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("secureUnpack: tar stream corrupt after %d entries: %w", n, err)
		}
		n++
		if err := extractEntry(root, hdr, tr, buf); err != nil {
			return fmt.Errorf("secureUnpack: member %q: %w", hdr.Name, err)
		}
	}
	return nil
}

// checkName validates a member name or link target: no leading "/", no ".."
// component anywhere. Returns the clean component slice ("" and "." are the
// archive "root" marker — refused).
func checkName(name string) ([]string, error) {
	if name == "" {
		return nil, fmt.Errorf("empty name")
	}
	if strings.HasPrefix(name, "/") {
		return nil, fmt.Errorf("absolute path")
	}
	comps := strings.Split(path.Clean(name), "/")
	for _, c := range comps {
		if c == ".." {
			return nil, fmt.Errorf("path contains %q", "..")
		}
	}
	if len(comps) == 1 && comps[0] == "." {
		return nil, fmt.Errorf("root entry")
	}
	return comps, nil
}

// openParent walks comps[0:len-1] under startDirFD with O_NOFOLLOW per
// component, creating missing intermediate directories when create is set.
// Closes startDirFD when it descends. Returns the fd of the final parent
// directory and the base name (caller closes the fd).
func openParent(startDirFD int, comps []string, create bool) (int, string, error) {
	dirfd := startDirFD
	for i := 0; i < len(comps)-1; i++ {
		next, err := syscall.Openat(dirfd, comps[i], syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW, 0)
		if err != nil && create && err == syscall.ENOENT {
			// MEASURED (kernel 5.10 dev, 2026-11-21): openat with
			// O_CREAT|O_DIRECTORY fails ENOTDIR on Linux — directories are
			// NOT creatable through openat flags. mkdirat + reopen instead.
			if mkerr := syscall.Mkdirat(dirfd, comps[i], 0755); mkerr != nil && mkerr != syscall.EEXIST {
				syscall.Close(dirfd)
				return 0, "", fmt.Errorf("mkdir %q: %w", comps[i], mkerr)
			}
			next, err = syscall.Openat(dirfd, comps[i], syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW, 0)
		}
		if err != nil {
			syscall.Close(dirfd)
			return 0, "", fmt.Errorf("component %q: %w", comps[i], err)
		}
		syscall.Close(dirfd)
		dirfd = next
	}
	return dirfd, comps[len(comps)-1], nil
}

// symlinkat / linkat: the stdlib syscall package exposes openat and mkdirat
// but not the *at variants for links — raw syscalls (amd64: 266/265) keep the
// O_NOFOLLOW openat discipline uniform for every create.
func symlinkat(target string, newdirfd int, newpath string) error {
	tptr, err := syscall.BytePtrFromString(target)
	if err != nil {
		return err
	}
	nptr, err := syscall.BytePtrFromString(newpath)
	if err != nil {
		return err
	}
	if _, _, errno := syscall.Syscall6(syscall.SYS_SYMLINKAT,
		uintptr(unsafe.Pointer(tptr)), uintptr(newdirfd), uintptr(unsafe.Pointer(nptr)), 0, 0, 0); errno != 0 {
		return errno
	}
	return nil
}

func linkat(oldfd int, oldpath string, newfd int, newpath string) error {
	optr, err := syscall.BytePtrFromString(oldpath)
	if err != nil {
		return err
	}
	nptr, err := syscall.BytePtrFromString(newpath)
	if err != nil {
		return err
	}
	if _, _, errno := syscall.Syscall6(syscall.SYS_LINKAT,
		uintptr(oldfd), uintptr(unsafe.Pointer(optr)), uintptr(newfd), uintptr(unsafe.Pointer(nptr)), 0, 0); errno != 0 {
		return errno
	}
	return nil
}

// dupFD clones an open fd (call sites treat EMFILE as any other failure).
func dupFD(fd int) (int, error) { return syscall.Dup(fd) }

// stripSpecialBits removes setuid/setgid/sticky from a tar mode: they have
// no boot role in this rootfs model, and capdrop+NNP neuter them inside the
// sandbox anyway. Pure function so the stripping is unit-verifiable even on
// hosts whose LSM strips the bits at create time (measured on the dev
// sandbox, 2026-11-21: openat-created files cannot carry 04000).
func stripSpecialBits(mode int64) int64 { return mode & 0777 }

// extractEntry dispatches one tar header to the create path for its type.
func extractEntry(root int, hdr *tar.Header, tr *tar.Reader, buf []byte) error {
	comps, err := checkName(hdr.Name)
	if err != nil {
		return fmt.Errorf("refused: %w", err)
	}
	perm := stripSpecialBits(hdr.Mode)

	switch hdr.Typeflag {
	case tar.TypeDir:
		rd, err := dupFD(root)
		if err != nil {
			return err
		}
		parentfd, base, err := openParent(rd, comps, true)
		if err != nil {
			return err
		}
		defer syscall.Close(parentfd)
		if err := syscall.Mkdirat(parentfd, base, uint32(perm|0700)); err != nil {
			// Already created by an earlier entry — tolerated ONLY if it
			// really is a directory (probed with O_NOFOLLOW: a symlink
			// planted under the same name does not pass).
			if probe, perr := syscall.Openat(parentfd, base, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW, 0); perr == nil {
				syscall.Close(probe)
				return nil
			}
			return err
		}
		return nil

	case tar.TypeReg, tar.TypeRegA:
		rd, err := dupFD(root)
		if err != nil {
			return err
		}
		parentfd, base, err := openParent(rd, comps, true)
		if err != nil {
			return err
		}
		defer syscall.Close(parentfd)
		fd, err := syscall.Openat(parentfd, base,
			syscall.O_WRONLY|syscall.O_CREAT|syscall.O_EXCL|syscall.O_NOFOLLOW, uint32(perm))
		if err != nil {
			return err
		}
		if _, err := io.CopyBuffer(fdWriter(fd), tr, buf); err != nil {
			syscall.Close(fd)
			return fmt.Errorf("content copy: %w", err)
		}
		if err := syscall.Close(fd); err != nil {
			return err
		}
		return nil

	case tar.TypeSymlink:
		if _, err := checkName(hdr.Linkname); err != nil {
			return fmt.Errorf("link target refused: %w", err)
		}
		rd, err := dupFD(root)
		if err != nil {
			return err
		}
		parentfd, base, err := openParent(rd, comps, true)
		if err != nil {
			return err
		}
		defer syscall.Close(parentfd)
		return symlinkat(hdr.Linkname, parentfd, base)

	case tar.TypeLink:
		srcComps, err := checkName(hdr.Linkname)
		if err != nil {
			return fmt.Errorf("link target refused: %w", err)
		}
		rd, err := dupFD(root)
		if err != nil {
			return err
		}
		parentfd, base, err := openParent(rd, comps, true)
		if err != nil {
			return err
		}
		defer syscall.Close(parentfd)
		srd, err := dupFD(root)
		if err != nil {
			return err
		}
		srcfd, srcbase, err := openParent(srd, srcComps, false)
		if err != nil {
			return fmt.Errorf("hardlink source: %w", err)
		}
		defer syscall.Close(srcfd)
		return linkat(srcfd, srcbase, parentfd, base)

	case tar.TypeFifo:
		rd, err := dupFD(root)
		if err != nil {
			return err
		}
		parentfd, base, err := openParent(rd, comps, true)
		if err != nil {
			return err
		}
		defer syscall.Close(parentfd)
		// MEASURED (2026-11-21): the O_CREAT|O_EXCL|O_NONBLOCK open trick
		// yields a REGULAR file on Linux — FIFOs must go through mknodat.
		// S_IFIFO needs no CAP_MKNOD (unlike char/block), so it stays
		// rootless-safe (PROMPT.MD §0).
		return syscall.Mknodat(parentfd, base, syscall.S_IFIFO|uint32(perm), 0)

	default:
		// TypeBlock/TypeChar and friends: mknod always fails EPERM rootless
		// (CAP_MKNOD) — refuse with the reason instead of shipping a
		// silently-broken rootfs. (PAX headers are consumed by tar.Next.)
		return fmt.Errorf("type %q refused: not creatable rootless / not needed for boot",
			string(rune(hdr.Typeflag)))
	}
}

// SecureUnpack is the exported entry point to secureUnpack, so the client
// layer (cmd/sandbox — Track M M3 template registry, issue #215/D81) can run
// the SAME fail-closed extractor as tar:/oci: images without copying it.
// Copied security code drifts (D25 lesson: the two seccomp copies needed a
// dedicated drift test); importing the single implementation keeps one
// source of truth. Guarantees: see secureUnpack above.
func SecureUnpack(r io.Reader, dst string) error { return secureUnpack(r, dst) }

// fdWriter adapts a raw fd to io.Writer without os.File (keeps the openat
// guarantees visible end-to-end).
type fdWriter int

func (f fdWriter) Write(p []byte) (int, error) {
	n := 0
	for n < len(p) {
		w, err := syscall.Write(int(f), p[n:])
		if w > 0 {
			n += w
		}
		if err == syscall.EINTR {
			continue
		}
		if err != nil {
			return n, err
		}
		if w == 0 {
			return n, io.ErrShortWrite
		}
	}
	return n, nil
}
