// Package firecracker — jail.go (GĐ4 mục 1b.a, issue #231, D85).
//
// The Firecracker VMM is jailed inside a MINICT INSTANCE instead of the
// upstream jailer: the jailer requires root (chroot, mknod, setuid, cgroup
// setup — docs/jailer.md), violating the §0 rootless invariant. minict already
// provides the same hardening rootless: userns + mountns + pivot_root +
// seccomp (D11 blocklist) + capdrop/no-new-privs + cgroups v2 delegation.
//
// What the jail instance gets (and nothing else):
//   - tools/bin bind :ro  → /fc-bin        (the static-pie VMM binary)
//   - per-jail state dir  → /fc-state      (API unix socket; host curls it
//                                           through the bind, MD72)
//   - /dev/kvm bind (only when present)    → VMM ioctls run unprivileged
//                                            (kvm udev mode 0666, CI job kvm)
//
// Compiles without KVM. The KVM path is exercised for real by the CI `kvm`
// job (go test ./internal/firecracker/... — TestJailSpecKVMDevice).

package firecracker

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/vietnerm/sandbox/minictd/contract"
)

// FcMountHostPath is the host directory holding the firecracker binary
// (tools/bin, populated by tools/fetch-firecracker.sh). Bound read-only
// into every jail at /fc-bin.
const FcMountHostPath = "tools/bin"

// JailPaths fixes the IN-JAIL layout (inside the minict instance). The VMM
// sees exactly this tree; everything else is outside its pivot_root.
const (
	FcBinInJail     = "/fc-bin"                   // ro bind of tools/bin
	FcStateInJail   = "/fc-state"                 // rw bind of the per-jail state dir
	FcAPISockInJail = FcStateInJail + "/api.sock" // VMM --api-sock target
	KVMInJail       = "/dev/kvm"                  // bind source when present
)

// JailSpec describes one Firecracker jail: a minict instance whose only
// extras are the two binds above plus /dev/kvm when the host has it.
type JailSpec struct {
	ID       string // unique jail id (becomes the instance name suffix)
	StateDir string // HOST path of the per-jail state dir (socket lives here)
	BinDir   string // HOST path of the dir holding the firecracker binary
	WithKVM  bool   // bind /dev/kvm into the jail (auto-checked by NewJailSpec)
}

// NewJailSpec builds a JailSpec. WithKVM is set automatically from host
// state: the bind is only requested when /dev/kvm is accessible — the same
// kvmAvailable() gate the backend uses, so the spec stays honest on hosts
// without KVM (no dangling bind source: Store.Run re-verifies bind sources
// and fails loudly, but here we simply do not ask).
func NewJailSpec(id, stateDir, binDir string) JailSpec {
	return JailSpec{
		ID:       id,
		StateDir: stateDir,
		BinDir:   binDir,
		WithKVM:  kvmAvailable(),
	}
}

// RunSpec renders the jail as a contract.RunSpec for the daemon's
// POST /v1/run (backend "minict" — the jail IS a container, the microVM is
// started inside it later via the API socket, mục 1b.e).
//
// Seccomp: default D11 blocklist (spec.SeccompProfile "") — firecracker
// installs its own seccompiler filter on top; blocklist mode is the honest
// baseline until a measured allowlist exists (1b.e).
func (s JailSpec) RunSpec() contract.RunSpec {
	volumes := []string{
		filepath.Join(s.BinDir) + ":" + FcBinInJail + ":ro",
		filepath.Join(s.StateDir) + ":" + FcStateInJail,
	}
	if s.WithKVM {
		volumes = append(volumes, "/dev/kvm:"+KVMInJail)
	}
	return contract.RunSpec{
		Name:     s.Name(),
		Hostname: s.Name(),
		Image:    "base",
		Tmpfs:    []string{"/tmp"},
		Volumes:  volumes,
		Cmd: []string{"/bin/sh", "-c",
			fmt.Sprintf("exec %s --api-sock %s", s.fcBinPath(), FcAPISockInJail)},
	}
}

// Name returns the instance name for this jail.
func (s JailSpec) Name() string { return "fc-jail-" + s.ID }

// fcBinPath returns the in-jail path of the VMM binary.
func (s JailSpec) fcBinPath() string { return FcBinInJail + "/firecracker" }

// APIHostPath returns the HOST-side path of the jail's API socket (the state
// dir bind target) — the daemon and tests curl the VMM through it (MD72).
func (s JailSpec) APIHostPath() string {
	return filepath.Join(s.StateDir, "api.sock")
}

// PrepareStateDir creates the per-jail state dir (0755) and fails loudly if
// the path is not empty-ish: a stale socket from a previous jail with the
// same id would make the VMM bind() fail with EADDRINUSE — remove it here so
// a re-run of the same jail id is deterministic.
func (s JailSpec) PrepareStateDir() error {
	if err := os.MkdirAll(s.StateDir, 0o755); err != nil {
		return fmt.Errorf("jail %s: state dir: %w", s.ID, err)
	}
	if err := os.Remove(s.APIHostPath()); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("jail %s: stale api socket: %w", s.ID, err)
	}
	return nil
}

// ArgvSummary returns a stable one-line description for logs and tests.
func (s JailSpec) ArgvSummary() string {
	kvm := ""
	if s.WithKVM {
		kvm = " +/dev/kvm"
	}
	return strings.TrimSpace(fmt.Sprintf("minict-instance %s binds=%s,%s%s",
		s.Name(), FcBinInJail+"(ro)", FcStateInJail, kvm))
}
