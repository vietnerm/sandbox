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

// prepareRootfs materializes <dir>/rootfs from the RunSpec.Image. Accepted
// image forms (NO MAGIC DEFAULTS — anything else is a hard error):
//
//	base        -> copy --base-rootfs (sandbox/rootfs from mkrootfs.sh)
//	dir:<path>  -> cp -a <path> (host dir, absolute)
//	tar:<path>  -> tar -xf <path> --no-same-owner (tar/tgz/tar.gz)
//	oci:<ref>   -> crane export <ref> - | tar -x --no-same-owner
//
// The OCI path needs the `crane` binary in PATH (P2 in the integration
// plan); everything else only needs cp/tar.
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
		return run(ctx, "tar", "-xf", arg, "-C", dst, "--no-same-owner")
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

// unpackOCI pipes `crane export <ref> -` into `tar -x -C <dst>
// --no-same-owner` without a shell, wiring the pipe by hand so a failure
// on either side surfaces properly.
func unpackOCI(ctx context.Context, ref, dst string) error {
	crane := exec.CommandContext(ctx, "crane", "export", ref, "-")
	tar := exec.CommandContext(ctx, "tar", "-x", "-C", dst, "--no-same-owner")

	pr, pw := io.Pipe()
	crane.Stdout = pw
	tar.Stdin = pr

	craneErr := make(chan error, 1)
	if err := crane.Start(); err != nil {
		return fmt.Errorf("crane export: %w", err)
	}
	go func() { craneErr <- crane.Wait() }()

	if err := tar.Run(); err != nil {
		_ = pw.CloseWithError(err) // unblock crane
		<-craneErr
		return fmt.Errorf("tar unpack (oci %q): %w", ref, err)
	}
	if err := pw.Close(); err != nil {
		// crane may still be flushing; wait for it to surface the error
	}
	if err := <-craneErr; err != nil {
		return fmt.Errorf("crane export %q: %w", ref, err)
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
