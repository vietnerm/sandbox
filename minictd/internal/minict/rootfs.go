package minict

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// prepareRootfs materializes <dir>/rootfs from the RunSpec.Image. Accepted
// image forms (NO MAGIC DEFAULTS — anything else is a hard error):
//
//	base        -> copy --base-rootfs (sandbox/rootfs from mkrootfs.sh)
//	dir:<path>  -> cp -a <path> (host dir, absolute)
//	tar:<path>  -> secureUnpack (fail-closed Go extractor, issue #104/D50
//	               — was `tar -xf`, which leaned on incidental GNU tar
//	               1.35 protections; busybox/old tar extract ".." members)
//	oci:<ref>   -> crane export <ref> - | secureUnpack (same extractor)
//
// The OCI path needs the `crane` binary in PATH (P2 in the integration
// plan); everything else only needs cp. Untrusted content (oci: refs and
// operator-provided tars) NEVER goes through host tar again.
func (s *Store) prepareRootfs(ctx context.Context, dir, image string) error {
	dst := filepath.Join(dir, "rootfs")
	// Caller guarantees dst does not exist (Run wipes the dir first; Start
	// never calls prepareRootfs).

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
		return copyTree(ctx, s.BaseRootfs, dst)
	case "dir":
		if !filepath.IsAbs(arg) {
			return fmt.Errorf("image dir: path must be absolute, got %q", arg)
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
