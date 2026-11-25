package minict

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// prepareRootfs materializes the instance rootfs from the RunSpec.Image.
// Accepted image forms (NO MAGIC DEFAULTS — anything else is a hard error):
//
//	base        -> copy --base-rootfs (sandbox/rootfs from mkrootfs.sh)
//	dir:<path>  -> cp -a <path> (host dir, absolute)
//	tar:<path>  -> secureUnpack (fail-closed Go extractor, issue #104/D50
//	               — was `tar -xf`, which leaned on incidental GNU tar
//	               1.35 protections; busybox/old tar extract ".." members)
//	oci:<ref>   -> crane export <ref> - | secureUnpack (same extractor)
//	template:<name> -> copy <Root>/_templates/<name>/rootfs (GĐ2 mục 2, D56)
//	backup:<name>  -> gunzip + extractGzTar <Root>/_backups/<name>.tar.gz (GĐ2 mục 3, D57)
//
// The OCI path needs the `crane` binary in PATH (P2 in the integration
// plan); everything else only needs cp. Untrusted content (oci: refs and
// operator-provided tars) NEVER goes through host tar again.
//
// D66 (issue #162): ovl != nil switches to the overlayfs layout — rootfs
// stays an EMPTY MOUNTPOINT, upper/work are created, and the materialization
// target moves to <dir>/lower (archive forms) or is skipped entirely
// (base/dir/template: ovl.Lower points AT the source, zero copy).
func (s *Store) prepareRootfs(ctx context.Context, dir, image string, ovl *overlayConfig) error {
	dst := ovl.unpackTarget(dir)
	// Caller guarantees dst does not exist (Run wipes the dir first; Start
	// never calls prepareRootfs).
	if ovl != nil {
		if err := prepareOverlayDirs(dir); err != nil {
			return err
		}
	}

	var kind, arg string
	switch {
	case image == "base":
		kind = "base"
	case strings.HasPrefix(image, "base:"):
		kind, arg = "base", ""
	case strings.HasPrefix(image, "dir:"):
		kind, arg = "dir", image[len("dir:"):]
	case strings.HasPrefix(image, "tar:"):
		kind, arg = "tar", image[len("tar:"):]
	case strings.HasPrefix(image, "oci:"):
		kind, arg = "oci", image[len("oci:"):]
	case strings.HasPrefix(image, "template:"):
		kind, arg = "template", image[len("template:"):]
	case strings.HasPrefix(image, "backup:"):
		kind, arg = "backup", image[len("backup:"):]
	case strings.HasPrefix(image, "/"):
		// Bare absolute path: dir if a directory, tar if a file.
		fi, err := os.Stat(image)
		if err != nil {
			return fmt.Errorf("image path %q: %w", image, err)
		}
		if fi.IsDir() {
			kind, arg = "dir", image
		} else {
			kind, arg = "tar", image
		}
	default:
		return fmt.Errorf("unsupported image %q: use base, dir:<path>, tar:<path> or oci:<ref>", image)
	}

	switch kind {
	case "base":
		if s.BaseRootfs == "" {
			return fmt.Errorf("image %q needs --base-rootfs (run sandbox/mkrootfs.sh first)", image)
		}
		if _, err := os.Stat(s.BaseRootfs); err != nil {
			return fmt.Errorf("--base-rootfs %q: %w", s.BaseRootfs, err)
		}
		if ovl != nil {
			return nil // D66: lower IS the base rootfs — zero copy
		}
		return copyTree(ctx, s.BaseRootfs, dst)
	case "dir":
		if !filepath.IsAbs(arg) {
			return fmt.Errorf("image dir: path must be absolute, got %q", arg)
		}
		if ovl != nil {
			return nil // D66: lower IS the host dir — zero copy
		}
		return copyTree(ctx, arg, dst)
	case "tar":
		if !filepath.IsAbs(arg) {
			return fmt.Errorf("image tar: path must be absolute, got %q", arg)
		}
		if err := os.MkdirAll(dst, 0755); err != nil {
			return err
		}
		f, err := os.Open(arg)
		if err != nil {
			return fmt.Errorf("image tar: open %q: %w", arg, err)
		}
		defer f.Close()
		return secureUnpack(f, dst)
	case "oci":
		if arg == "" {
			return fmt.Errorf("image oci: missing reference")
		}
		if err := os.MkdirAll(dst, 0755); err != nil {
			return err
		}
		return unpackOCI(ctx, arg, dst)
	case "template":
		if arg == "" {
			return fmt.Errorf("image template: missing name")
		}
		if err := validName(arg); err != nil {
			return fmt.Errorf("image template: name: %w", err)
		}
		src := filepath.Join(s.templatesDir(), arg, "rootfs")
		if _, err := os.Stat(src); err != nil {
			return fmt.Errorf("image template:%s: not found (save-template first): %w", arg, err)
		}
		if ovl != nil {
			return nil // D66: lower IS the template tree — zero copy
		}
		return copyTree(ctx, src, dst)
	case "backup":
		if arg == "" {
			return fmt.Errorf("image backup: missing name")
		}
		if err := validName(arg); err != nil {
			return fmt.Errorf("image backup: name: %w", err)
		}
		archive := filepath.Join(s.backupsDir(), arg+".tar.gz")
		f, err := os.Open(archive)
		if err != nil {
			return fmt.Errorf("image backup:%s: not found (snapshot first): %w", arg, err)
		}
		defer f.Close()
		return extractGzTar(f, dst)
	}
	return fmt.Errorf("unsupported image form %q", image)
}

// copyTree shells out to cp -a: it preserves symlinks, mode and ownership
// with the exact semantics minict.sh relies on (PoC parity) and is far
// shorter than a correct Go tree-copy with symlink handling.
func copyTree(ctx context.Context, src, dst string) error {
	return run(ctx, "cp", "-a", src, dst)
}

// unpackOCI streams `crane export <ref> -` straight into secureUnpack
// (issue #104/D50 — was `tar -x --no-same-owner`, whose fail-closed posture
// depended on incidental GNU tar 1.35 behavior). No shell, no host tar; a
// refusal on the unpack side surfaces first and crane sees EPIPE.
func unpackOCI(ctx context.Context, ref, dst string) error {
	crane := exec.CommandContext(ctx, "crane", "export", ref, "-")
	stdout, err := crane.StdoutPipe()
	if err != nil {
		return fmt.Errorf("crane export pipe: %w", err)
	}
	if err := crane.Start(); err != nil {
		return fmt.Errorf("crane export: %w", err)
	}
	unpackErr := secureUnpack(stdout, dst)
	// Drain whatever remains of the stream before waiting. A mid-stream
	// refusal stops the reader while crane is still writing: its stdout
	// pipe fills (64 KB) and crane blocks in write(2) forever, so Wait()
	// (which only closes the pipe after the process exits) would
	// deadlock the Run handler until the request context cancels
	// (issue #108 — measured 2026-11-21 with a goroutine dump: crane in
	// syscall write on the full pipe, 22144 bytes queued unread). The
	// drain is a pure read — nothing is extracted — so it cannot weaken
	// the refusal that secureUnpack already returned.
	_, _ = io.Copy(io.Discard, stdout)
	// Always reap crane: on unpack refusal it dies of EPIPE (its error is
	// noise); on success its error is the real one.
	craneErr := crane.Wait()
	if unpackErr != nil {
		return fmt.Errorf("oci unpack %q: %w", ref, unpackErr)
	}
	if craneErr != nil {
		return fmt.Errorf("crane export %q: %w", ref, craneErr)
	}
	return nil
}

func run(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %w (%s)", name, strings.Join(args, " "),
			err, strings.TrimSpace(string(out)))
	}
	return nil
}
