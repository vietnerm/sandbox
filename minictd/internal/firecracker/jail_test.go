package firecracker

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// TestJailSpecRunSpec pins the jail → RunSpec rendering (D85): name, the two
// binds, no KVM bind on a host without /dev/kvm, and the VMM cmdline.
func TestJailSpecRunSpec(t *testing.T) {
	dir := t.TempDir()
	state := filepath.Join(dir, "state")
	bin := filepath.Join(dir, "bin")

	spec := NewJailSpec("t1", state, bin)
	rs := spec.RunSpec()

	if spec.Name() != "fc-jail-t1" {
		t.Errorf("Name: got %q, want fc-jail-t1", spec.Name())
	}
	if rs.Name != "fc-jail-t1" || rs.Hostname != "fc-jail-t1" {
		t.Errorf("RunSpec name/hostname: got %q/%q", rs.Name, rs.Hostname)
	}
	if rs.Image != "base" {
		t.Errorf("Image: got %q, want base", rs.Image)
	}
	if rs.SeccompProfile != "" {
		t.Errorf("SeccompProfile: got %q, want \"\" (D11 blocklist default)", rs.SeccompProfile)
	}

	wantVol := []string{bin + ":" + FcBinInJail + ":ro", state + ":" + FcStateInJail}
	if len(rs.Volumes) != len(wantVol) {
		t.Fatalf("Volumes: got %v, want %v", rs.Volumes, wantVol)
	}
	for i, w := range wantVol {
		if rs.Volumes[i] != w {
			t.Errorf("Volumes[%d]: got %q, want %q", i, rs.Volumes[i], w)
		}
	}

	if spec.WithKVM {
		t.Log("host HAS /dev/kvm — KVM bind assertions covered by TestJailSpecKVMDevice")
	} else if strings.Join(rs.Volumes, " ") != strings.Join(wantVol, " ") {
		t.Error("no-KVM host must not add any extra bind")
	}

	// The VMM runs the static binary from the ro bind and puts the API
	// socket in the rw state bind — pinned so 1b.e (API client) can rely on it.
	cmd := strings.Join(rs.Cmd, " ")
	if !strings.Contains(cmd, FcBinInJail+"/firecracker") {
		t.Errorf("cmd must launch the VMM from the ro bind, got: %s", cmd)
	}
	if !strings.Contains(cmd, "--api-sock "+FcAPISockInJail) {
		t.Errorf("cmd must point --api-sock at the state bind, got: %s", cmd)
	}
}

// TestJailSpecPrepareStateDir: fresh dir created 0755; a stale socket from a
// previous jail with the same id is removed (VMM bind() would EADDRINUSE).
func TestJailSpecPrepareStateDir(t *testing.T) {
	dir := t.TempDir()
	state := filepath.Join(dir, "state")
	stale := filepath.Join(state, "api.sock")

	if err := os.MkdirAll(state, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stale, []byte("stale"), 0o600); err != nil {
		t.Fatal(err)
	}

	spec := JailSpec{ID: "p1", StateDir: state, BinDir: dir}
	if err := spec.PrepareStateDir(); err != nil {
		t.Fatalf("PrepareStateDir: %v", err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Errorf("stale socket still present: %v", err)
	}
	if fi, err := os.Stat(state); err != nil || fi.Mode().Perm() != 0o755 {
		t.Errorf("state dir: err=%v mode=%v", err, fi)
	}
}

// TestJailSpecKVMDevice — GĐ4 1b.a KVM half, REAL on the CI `kvm` job
// (ubuntu-24.04 with /dev/kvm udev 0666 for the runner user):
//   - the spec gains the /dev/kvm bind exactly when the host has KVM;
//   - the bind source is usable for real: open + KVM_GET_API_VERSION ioctl.
//
// On hosts without /dev/kvm (dev machine, build-test job) the test SKIPS —
// the kvm job "must not skip" per ci.yml, so a broken jail spec fails there.
func TestJailSpecKVMDevice(t *testing.T) {
	if !kvmAvailable() {
		t.Skip("/dev/kvm not available on this host (env-gated — runs for real on CI job kvm)")
	}

	dir := t.TempDir()
	spec := NewJailSpec("kv", filepath.Join(dir, "state"), filepath.Join(dir, "bin"))
	if !spec.WithKVM {
		t.Fatal("WithKVM must be true when /dev/kvm is accessible")
	}
	rs := spec.RunSpec()
	found := false
	for _, v := range rs.Volumes {
		if v == "/dev/kvm:"+KVMInJail {
			found = true
		}
	}
	if !found {
		t.Errorf("Volumes missing /dev/kvm bind, got %v", rs.Volumes)
	}

	// The same probe ci.yml runs on the host — proving the source the jail
	// binds is a working KVM device for an unprivileged user.
	f, err := os.OpenFile("/dev/kvm", os.O_RDWR, 0)
	if err != nil {
		t.Fatalf("open /dev/kvm: %v", err)
	}
	defer f.Close()
	const KVM_GET_API_VERSION = 0xAE00
	ver, _, errno := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), KVM_GET_API_VERSION, 0)
	if errno != 0 {
		t.Fatalf("KVM_GET_API_VERSION: %v", errno)
	}
	if ver != 12 {
		t.Errorf("KVM_GET_API_VERSION: got %d, want 12", ver)
	}
}
