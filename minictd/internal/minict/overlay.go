package minict

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// overlay.go — GĐ2 mục 3 residual (issue #162, D66): rootfs overlayfs
// rootless, env-gated. When the daemon runs with MINICT_OVERLAY=1 every NEW
// instance mounts an overlayfs (lower = shared source, upper/work =
// per-instance) instead of a private `cp -a` copy — the ~2 MB per-instance
// disk cost and the copy I/O at cold start go away for base/dir/template
// images (lower points AT the source, zero copy).
//
// The mount itself happens in the C shim INSIDE its userns (kernel >= 5.11
// mounts overlayfs unprivileged with `userxattr`; workdir must share
// upper's fs — guaranteed by layout: <dir>/upper + <dir>/work). The daemon
// only renders the `overlay lower=… upper=… work=…` directive and prepares
// the directories; both sides validate fail-closed (renderSpec + spec_overlay
// in minict.c).
//
// The gate is ENV, not a CLI flag, and NOT the default: dev machines on
// kernel < 5.11 refuse the userns overlay mount (measured: EPERM on 5.10),
// so a default-on gate would boot nothing there. CI (ubuntu-24.04, kernel
// 6.x) exercises the real e2e path; dev keeps the legacy copy path with the
// same binary. No silent fallback in the shim — an overlay mount failure
// dies loudly (D66 rationale 4).

const (
	// OverlayEnv is the daemon-side gate. Values "1", "true", "yes"
	// enable overlayfs rootfs for new instances.
	OverlayEnv = "MINICT_OVERLAY"
	// overlayPathCap matches the C parser's field size (spec_overlay
	// lower/upper/work are char[512]) — a longer path would be refused
	// there anyway, so refuse here first with a clearer message.
	overlayPathCap = 512
)

// overlayEnabled reports the MINICT_OVERLAY gate state. Read per call
// (not cached at daemon start): tests set the env per-daemon process and
// Go units can flip it in-process; a restart picks up operator changes.
func overlayEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(OverlayEnv))) {
	case "1", "true", "yes":
		return true
	default:
		return false
	}
}

// overlayConfig carries the resolved overlay paths for one instance.
// nil (pointer) everywhere means "overlay off — legacy private copy".
type overlayConfig struct {
	// Lower is the shared read-only source tree. For base/dir/template
	// images it points DIRECTLY at the source (no copy); for tar/oci/
	// backup images prepareRootfs unpacks the archive into <dir>/lower
	// and this field names that unpacked tree.
	Lower string
	// Upper/Work are the per-instance writable layer and overlayfs
	// scratch dir. Same filesystem by construction (both under <dir>).
	Upper string
	Work  string
	// zeroCopy marks the image forms whose lower IS the source tree
	// (base/dir/template/bare-dir) — prepareRootfs skips the copy and
	// overlayFor validates the source exists up front.
	zeroCopy bool
}

// specLine renders the MINICT_SPEC directive the C parser consumes.
func (o *overlayConfig) specLine() string {
	return "overlay lower=" + o.Lower + " upper=" + o.Upper + " work=" + o.Work
}

// unpackTarget is where prepareRootfs materializes archive images
// (tar/oci/backup). Legacy mode unpacks into rootfs itself; overlay mode
// unpacks into <dir>/lower (rootfs stays the empty mountpoint).
func (o *overlayConfig) unpackTarget(dir string) string {
	if o != nil {
		return filepath.Join(dir, "lower")
	}
	return filepath.Join(dir, "rootfs")
}

// validateOverlay enforces the invariants BOTH sides need. The C parser
// re-checks (fail-closed twin), but the daemon must refuse with a clear
// error before any state is written. Rules mirror spec_overlay():
// absolute paths, no ".." substring, length cap, distinct dirs, and lower
// must not be an ancestor of upper/work (overlay copy-up would then write
// through to the shared source — exactly what overlayfs exists to prevent).
func validateOverlay(o *overlayConfig) error {
	for name, p := range map[string]string{
		"lower": o.Lower, "upper": o.Upper, "work": o.Work,
	} {
		if !filepath.IsAbs(p) {
			return fmt.Errorf("overlay %s must be an absolute path, got %q", name, p)
		}
		if len(p) >= overlayPathCap {
			return fmt.Errorf("overlay %s path too long (cap %d)", name, overlayPathCap)
		}
		if strings.Contains(p, "..") {
			return fmt.Errorf("overlay %s must not contain '..': %q", name, p)
		}
	}
	if o.Upper == o.Work {
		return fmt.Errorf("overlay upper == work (kernel refuses)")
	}
	if o.Lower == o.Upper || o.Lower == o.Work {
		return fmt.Errorf("overlay lower must differ from upper/work")
	}
	for name, child := range map[string]string{"upper": o.Upper, "work": o.Work} {
		if strings.HasPrefix(child, o.Lower+"/") || (o.Lower == "/" && child != "/") {
			return fmt.Errorf("overlay lower is an ancestor of %s (copy-up would write into the shared source)", name)
		}
	}
	return nil
}

// overlayFor resolves the overlay paths for a new instance from the image
// form. Called by Run BEFORE renderSpec (the directive must be in the
// .mspec the payload reads) and by prepareRootfs (which must prepare the
// SAME layout — both sides derive from dir/image deterministically, so no
// shared mutable state is needed).
//
// Lower resolution per image form:
//
//	base / base:      -> s.BaseRootfs           (zero copy)
//	dir:<abs>         -> <abs>                  (zero copy)
//	template:<name>   -> <templates>/<name>/rootfs (zero copy)
//	tar:/oci:/backup: -> <dir>/lower            (unpack target)
func (s *Store) overlayFor(dir, image string) (*overlayConfig, error) {
	o := &overlayConfig{
		Upper: filepath.Join(dir, "upper"),
		Work:  filepath.Join(dir, "work"),
	}
	switch {
	case image == "base", strings.HasPrefix(image, "base:"):
		if s.BaseRootfs == "" {
			return nil, fmt.Errorf("image %q needs --base-rootfs (run sandbox/mkrootfs.sh first)", image)
		}
		o.Lower = s.BaseRootfs
		o.zeroCopy = true
	case strings.HasPrefix(image, "dir:"):
		o.Lower = image[len("dir:"):]
		o.zeroCopy = true
	case strings.HasPrefix(image, "/"):
		// Bare absolute path: the daemon mirrors prepareRootfs' dir/tar
		// decision — a dir is shared read-only, a tar unpacks into lower.
		fi, err := os.Stat(image)
		if err != nil {
			return nil, fmt.Errorf("image path %q: %w", image, err)
		}
		if fi.IsDir() {
			o.Lower = image
			o.zeroCopy = true
		} else {
			o.Lower = filepath.Join(dir, "lower")
		}
	case strings.HasPrefix(image, "tar:"),
		strings.HasPrefix(image, "oci:"),
		strings.HasPrefix(image, "backup:"):
		o.Lower = filepath.Join(dir, "lower")
	case strings.HasPrefix(image, "template:"):
		arg := image[len("template:"):]
		if arg == "" {
			return nil, fmt.Errorf("image template: missing name")
		}
		if err := validName(arg); err != nil {
			return nil, fmt.Errorf("image template: name: %w", err)
		}
		o.Lower = filepath.Join(s.templatesDir(), arg, "rootfs")
		o.zeroCopy = true
	default:
		return nil, fmt.Errorf("unsupported image %q: use base, dir:<path>, tar:<path> or oci:<ref>", image)
	}
	// Zero-copy forms point at an EXISTING tree — a missing source must
	// fail before spawn (renderSpec's directive names a real tree).
	// Archive forms unpack later (prepareRootfs); their lower dir is
	// created by the unpack, not before it.
	if o.zeroCopy {
		if _, err := os.Stat(o.Lower); err != nil {
			return nil, fmt.Errorf("overlay lower %q: %w", o.Lower, err)
		}
	}
	if err := validateOverlay(o); err != nil {
		return nil, err
	}
	return o, nil
}

// prepareOverlayDirs creates the per-instance overlay layout: the empty
// rootfs MOUNTPOINT (the shim mounts overlayfs on it inside the userns),
// the writable upper and the work dir. 0700 upper/work: the payload runs
// in a userns where root==this uid, so no looser mode is needed, and the
// instance dir is 0700-adjacent territory already (state.json holds notes,
// never secrets — but the upper layer holds payload-written files).
func prepareOverlayDirs(dir string) error {
	for _, d := range []string{
		filepath.Join(dir, "rootfs"),
		filepath.Join(dir, "upper"),
		filepath.Join(dir, "work"),
	} {
		if err := os.MkdirAll(d, 0700); err != nil {
			return fmt.Errorf("overlay dir %s: %w", d, err)
		}
	}
	return nil
}
