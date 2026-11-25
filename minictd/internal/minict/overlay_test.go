package minict

// Unit tests for the overlayfs rootfs gate (GĐ2 mục 3 residual, issue #162,
// D66). No payloads are spawned: prepareRootfs runs against a fake base
// rootfs, renderSpec/render checks are pure, and the Snapshot/SaveTemplate
// refusals are checked on seeded state (reconcile_test.go pattern). The e2e
// counterparts are MD44–MD46 in tests/test_minictd.sh (env-gated: kernel
// >= 5.11 for the unprivileged overlay mount).

import (
        "context"
        "log/slog"
        "os"
        "path/filepath"
        "strings"
        "testing"

        "github.com/vietnerm/sandbox/minictd/contract"
)

// setOverlayEnv flips the MINICT_OVERLAY gate for one test.
func setOverlayEnv(t *testing.T, on bool) {
        t.Helper()
        if on {
                t.Setenv(OverlayEnv, "1")
        } else {
                t.Setenv(OverlayEnv, "")
        }
}

// fakeBaseRootfs builds a minimal busybox-style base: bin/sh must exist for
// overlayFor's lower existence check (the shim re-checks executability).
func fakeBaseRootfs(t *testing.T) string {
        t.Helper()
        base := t.TempDir()
        if err := os.MkdirAll(filepath.Join(base, "bin"), 0o755); err != nil {
                t.Fatal(err)
        }
        if err := os.WriteFile(filepath.Join(base, "bin", "sh"), []byte("#!/bin/sh\n"), 0o755); err != nil {
                t.Fatal(err)
        }
        return base
}

func newOverlayTestStore(t *testing.T, base string) *Store {
        t.Helper()
        return &Store{
                Root:       t.TempDir(),
                BaseRootfs: base,
                MinictBin:  "minict",
                Log:        slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
        }
}

func TestOverlayEnabledGate(t *testing.T) {
        for _, on := range []string{"1", "true", "YES", " yes "} {
                t.Setenv(OverlayEnv, on)
                if !overlayEnabled() {
                        t.Errorf("overlayEnabled(%q) = false, want true", on)
                }
        }
        for _, off := range []string{"", "0", "false", "off"} {
                t.Setenv(OverlayEnv, off)
                if overlayEnabled() {
                        t.Errorf("overlayEnabled(%q) = true, want false", off)
                }
        }
}

func TestOverlayForBaseZeroCopy(t *testing.T) {
        setOverlayEnv(t, true)
        base := fakeBaseRootfs(t)
        s := newOverlayTestStore(t, base)
        dir := filepath.Join(s.Root, "ovl")
        if err := os.MkdirAll(dir, 0o755); err != nil {
                t.Fatal(err)
        }

        ovl, err := s.overlayFor(dir, "base")
        if err != nil {
                t.Fatalf("overlayFor: %v", err)
        }
        if ovl.Lower != base {
                t.Errorf("lower = %q, want the base rootfs %q (zero copy)", ovl.Lower, base)
        }

        if err := s.prepareRootfs(context.Background(), dir, "base", ovl); err != nil {
                t.Fatalf("prepareRootfs(overlay): %v", err)
        }
        // The mountpoint must stay EMPTY — any copied file here means the
        // private-copy path leaked through (mutant M-ovl-copy target).
        entries, err := os.ReadDir(filepath.Join(dir, "rootfs"))
        if err != nil {
                t.Fatal(err)
        }
        if len(entries) != 0 {
                t.Errorf("overlay rootfs mountpoint not empty: %d entries", len(entries))
        }
        for _, d := range []string{"upper", "work"} {
                if fi, err := os.Stat(filepath.Join(dir, d)); err != nil || !fi.IsDir() {
                        t.Errorf("%s missing or not a dir: %v", d, err)
                }
        }
        // M-ovl-copy negative control (SCR Phiên 31): a regression that copies
        // anyway would copy into <dir>/lower (unpackTarget), NOT the mountpoint
        // — the empty-mountpoint assertion above would NOT catch it. The
        // zero-copy claim requires lower to be the SOURCE path, nothing
        // materialized in the instance dir (MD44's du/[[ -d lower ]] asserts
        // the same thing e2e on overlay-capable kernels).
        if _, err := os.Stat(filepath.Join(dir, "lower")); err == nil {
                t.Error("instance dir has a lower/ tree after base prepareRootfs — zero-copy broken (mutant M-ovl-copy)")
        }
}

func TestOverlayForArchiveTargetsLower(t *testing.T) {
        setOverlayEnv(t, true)
        base := fakeBaseRootfs(t)
        s := newOverlayTestStore(t, base)
        dir := filepath.Join(s.Root, "ovl")
        if err := os.MkdirAll(dir, 0o755); err != nil {
                t.Fatal(err)
        }

        ovl, err := s.overlayFor(dir, "tar:/nowhere/x.tar")
        if err != nil {
                t.Fatalf("overlayFor(tar:): %v", err)
        }
        want := filepath.Join(dir, "lower")
        if ovl.Lower != want {
                t.Errorf("tar: lower = %q, want %q", ovl.Lower, want)
        }
        if got := ovl.unpackTarget(dir); got != want {
                t.Errorf("unpackTarget = %q, want %q (rootfs stays the mountpoint)", got, want)
        }
}

func TestOverlayForRefusals(t *testing.T) {
        setOverlayEnv(t, true)
        base := fakeBaseRootfs(t)
        s := newOverlayTestStore(t, base)
        dir := filepath.Join(s.Root, "ovl")
        if err := os.MkdirAll(dir, 0o755); err != nil {
                t.Fatal(err)
        }

        // Missing source: base with no BaseRootfs configured.
        s2 := newOverlayTestStore(t, "")
        if _, err := s2.overlayFor(dir, "base"); err == nil {
                t.Error("base without --base-rootfs: want error")
        }
        // dir: form pointing INSIDE the instance dir (lower ancestor of upper).
        if _, err := s.overlayFor(dir, "dir:"+dir); err == nil {
                t.Error("dir:<instancedir> is an ancestor of upper — want refusal")
        }
        // Relative dir: source.
        if _, err := s.overlayFor(dir, "dir:relative/path"); err == nil {
                t.Error("relative dir: path — want refusal")
        }
        // Unknown image form.
        if _, err := s.overlayFor(dir, "docker:busybox"); err == nil {
                t.Error("unknown image form — want refusal")
        }
}

func TestValidateOverlayRules(t *testing.T) {
        cases := []struct {
                name string
                o    overlayConfig
                ok   bool
        }{
                {"clean", overlayConfig{Lower: "/a/l", Upper: "/a/u", Work: "/a/w"}, true},
                {"relative lower", overlayConfig{Lower: "l", Upper: "/u", Work: "/w"}, false},
                {"relative work", overlayConfig{Lower: "/l", Upper: "/u", Work: "w"}, false},
                {"dotdot", overlayConfig{Lower: "/l/../x", Upper: "/u", Work: "/w"}, false},
                {"upper==work", overlayConfig{Lower: "/l", Upper: "/u", Work: "/u"}, false},
                {"lower==upper", overlayConfig{Lower: "/u", Upper: "/u", Work: "/w"}, false},
                {"lower ancestor of upper", overlayConfig{Lower: "/l", Upper: "/l/u", Work: "/w"}, false},
                {"lower ancestor of work", overlayConfig{Lower: "/l", Upper: "/u", Work: "/l/w"}, false},
                {"shared parent is fine", overlayConfig{Lower: "/l", Upper: "/o/u", Work: "/o/w"}, true},
                {"lower root ancestor", overlayConfig{Lower: "/", Upper: "/u", Work: "/w"}, false},
        }
        for _, tc := range cases {
                err := validateOverlay(&tc.o)
                if tc.ok && err != nil {
                        t.Errorf("%s: unexpected error %v", tc.name, err)
                }
                if !tc.ok && err == nil {
                        t.Errorf("%s: want refusal, got nil", tc.name)
                }
        }
}

// runSpecForOverlay builds a minimal spec for renderSpec calls.
func runSpecForOverlay() *contract.RunSpec {
        return &contract.RunSpec{Name: "ovl", Image: "base"}
}

func TestRenderSpecOverlayDirective(t *testing.T) {
        ovl := &overlayConfig{Lower: "/b/rootfs", Upper: "/s/i/upper", Work: "/s/i/work"}
        lines, notes, err := renderSpec(runSpecForOverlay(), nil, false, ovl)
        if err != nil {
                t.Fatalf("renderSpec(overlay): %v", err)
        }
        want := "overlay lower=/b/rootfs upper=/s/i/upper work=/s/i/work"
        found := false
        for _, l := range lines {
                if l == want {
                        found = true
                }
        }
        if !found {
                t.Errorf("overlay directive missing\nlines: %v", lines)
        }
        if len(notes) == 0 || !strings.Contains(strings.Join(notes, "\n"), "no private copy") {
                t.Errorf("overlay note missing: %v", notes)
        }

        // Invalid paths must be refused even if overlayFor was bypassed.
        bad := &overlayConfig{Lower: "rel", Upper: "/u", Work: "/w"}
        if _, _, err := renderSpec(runSpecForOverlay(), nil, false, bad); err == nil {
                t.Error("renderSpec must re-validate (twin gates)")
        }
}

func TestRenderSpecNoOverlayRegression(t *testing.T) {
        // Gate off (nil ovl): no overlay directive may appear — the C shim
        // would try to mount garbage (or nothing at all).
        lines, _, err := renderSpec(runSpecForOverlay(), nil, false, nil)
        if err != nil {
                t.Fatalf("renderSpec(nil): %v", err)
        }
        for _, l := range lines {
                if strings.HasPrefix(l, "overlay ") {
                        t.Errorf("overlay directive leaked with gate off: %q", l)
                }
        }
}

func TestSnapshotAndSaveTemplateRefuseOverlay(t *testing.T) {
        s := newOverlayTestStore(t, fakeBaseRootfs(t))
        name := "ovl-snap"
        if err := os.MkdirAll(s.dir(name), 0o755); err != nil {
                t.Fatal(err)
        }
        st := &InstanceState{Name: name, State: StateStopped, Overlay: true}
        if err := s.saveState(name, st); err != nil {
                t.Fatal(err)
        }
        if err := s.writeJSON(filepath.Join(s.dir(name), fSpec), runSpecForOverlay()); err != nil {
                t.Fatal(err)
        }

        err := s.Snapshot(context.Background(), name, "snap-ovl")
        if err == nil || !strings.Contains(err.Error(), "D66 residual") {
                t.Errorf("Snapshot(overlay) = %v, want D66 residual refusal", err)
        }
        // No half-written archive may survive a refusal.
        if _, statErr := os.Stat(filepath.Join(s.backupsDir(), "snap-ovl.tar.gz")); statErr == nil {
                t.Error("refused snapshot left an archive behind")
        }

        err = s.SaveTemplate(context.Background(), name, "tmpl-ovl")
        if err == nil || !strings.Contains(err.Error(), "D66 residual") {
                t.Errorf("SaveTemplate(overlay) = %v, want D66 residual refusal", err)
        }
}

func TestPrepareRootfsLegacyRegression(t *testing.T) {
        // Gate off: prepareRootfs must behave exactly as before (private copy
        // into <dir>/rootfs) — MD4/MD16 and every existing e2e depend on it.
        setOverlayEnv(t, false)
        base := fakeBaseRootfs(t)
        s := newOverlayTestStore(t, base)
        dir := filepath.Join(s.Root, "copy")
        if err := os.MkdirAll(dir, 0o755); err != nil {
                t.Fatal(err)
        }
        if err := s.prepareRootfs(context.Background(), dir, "base", nil); err != nil {
                t.Fatalf("prepareRootfs(legacy): %v", err)
        }
        if _, err := os.Stat(filepath.Join(dir, "rootfs", "bin", "sh")); err != nil {
                t.Errorf("legacy copy missing bin/sh: %v", err)
        }
        if _, err := os.Stat(filepath.Join(dir, "upper")); err == nil {
                t.Error("legacy mode must not create upper/ (overlay layout leak)")
        }
}
