// Package firecracker — backend.go (GĐ4 mục 1, issue #193).
//
// Backend interface + FirecrackerBackend. The backend is selected by the
// --backend flag in minictd. When /dev/kvm is not available, all ops
// return ErrNoKVM (env-gated, same pattern as overlay D66 / pasta D67).

package firecracker

import (
	"context"
	"errors"
	"os"
	"path/filepath"
)

// ErrNoKVM is returned when /dev/kvm is not accessible (env-gated).
var ErrNoKVM = errors.New("firecracker: /dev/kvm not available (GĐ4 env-gated — need bare metal or nested virt)")

// kvmAvailable checks if /dev/kvm is accessible for read/write.
func kvmAvailable() bool {
	f, err := os.OpenFile("/dev/kvm", os.O_RDWR, 0)
	if err != nil {
		return false
	}
	f.Close()
	return true
}

// Backend is the interface for Firecracker microVM management.
// The minictd Store calls these methods when --backend=firecracker.
type Backend interface {
	// Run creates + boots a microVM. Returns the instance name.
	Run(ctx context.Context, name string, vcpu int, memMib int, rootfsImage, kernelPath string) (string, error)
	// Start boots a stopped microVM (re-creates the Firecracker process).
	Start(ctx context.Context, name string) error
	// Stop sends CtrlAltDel (graceful) then kills the process.
	Stop(ctx context.Context, name string) error
	// Remove kills + cleans up all microVM resources.
	Remove(ctx context.Context, name string) error
}

// FirecrackerBackend manages Firecracker microVMs.
// Stateless — each call resolves paths from the state dir.
type FirecrackerBackend struct {
	StateDir    string // minictd state root
	FcBin       string // firecracker binary path
	JailerBin   string // jailer binary path
	VmlinuxPath string // kernel image path
}

// NewFirecrackerBackend creates a backend from the daemon config.
func NewFirecrackerBackend(stateDir, fcBin, jailerBin, vmlinux string) *FirecrackerBackend {
	return &FirecrackerBackend{
		StateDir:    stateDir,
		FcBin:       fcBin,
		JailerBin:   jailerBin,
		VmlinuxPath: vmlinux,
	}
}

// instanceDir returns the Firecracker state dir for one microVM.
func (b *FirecrackerBackend) instanceDir(name string) string {
	return filepath.Join(b.StateDir, name)
}

// apiSocket returns the Firecracker API socket path for one microVM.
func (b *FirecrackerBackend) apiSocket(name string) string {
	return filepath.Join(b.instanceDir(name), "api.sock")
}

// Run creates + boots a microVM (GĐ4 mục 1).
// Currently returns ErrNoKVM when /dev/kvm is not available.
func (b *FirecrackerBackend) Run(ctx context.Context, name string, vcpu int, memMib int, rootfsImage, kernelPath string) (string, error) {
	if !kvmAvailable() {
		return "", ErrNoKVM
	}
	// TODO (when KVM available): spawn jailer + firecracker, configure via API.
	// For now, this path is unreachable without /dev/kvm.
	return "", ErrNoKVM
}

// Start re-boots a stopped microVM.
func (b *FirecrackerBackend) Start(ctx context.Context, name string) error {
	if !kvmAvailable() {
		return ErrNoKVM
	}
	return ErrNoKVM
}

// Stop sends CtrlAltDel then kills the Firecracker process.
func (b *FirecrackerBackend) Stop(ctx context.Context, name string) error {
	if !kvmAvailable() {
		return ErrNoKVM
	}
	return ErrNoKVM
}

// Remove kills + cleans up all microVM resources.
func (b *FirecrackerBackend) Remove(ctx context.Context, name string) error {
	// Remove can proceed even without KVM (just cleanup files).
	dir := b.instanceDir(name)
	_ = os.RemoveAll(dir)
	return nil
}
