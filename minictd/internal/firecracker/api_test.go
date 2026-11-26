package firecracker

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestAPIClientPut verifies the API client sends correct PUT requests over
// a real unix socket (no KVM needed — pure HTTP).
func TestAPIClientPut(t *testing.T) {
	dir := t.TempDir()
	sock := filepath.Join(dir, "api.sock")

	// Start a mock Firecracker API server on the unix socket.
	lis, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer lis.Close()

	var lastPath string
	var lastBody string
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /boot-source", func(w http.ResponseWriter, r *http.Request) {
		lastPath = "/boot-source"
		buf := make([]byte, 1024)
		n, _ := r.Body.Read(buf)
		lastBody = string(buf[:n])
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("PUT /machine-config", func(w http.ResponseWriter, r *http.Request) {
		lastPath = "/machine-config"
		buf := make([]byte, 1024)
		n, _ := r.Body.Read(buf)
		lastBody = string(buf[:n])
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("PUT /actions", func(w http.ResponseWriter, r *http.Request) {
		lastPath = "/actions"
		buf := make([]byte, 1024)
		n, _ := r.Body.Read(buf)
		lastBody = string(buf[:n])
		w.WriteHeader(http.StatusNoContent)
	})
	go http.Serve(lis, mux)

	c := NewClient(sock)
	ctx := context.Background()

	// BootSource
	if err := c.BootSource(ctx, "/path/vmlinux", "console=ttyS0"); err != nil {
		t.Fatalf("BootSource: %v", err)
	}
	if lastPath != "/boot-source" {
		t.Errorf("lastPath: got %q, want /boot-source", lastPath)
	}
	if !strings.Contains(lastBody, "vmlinux") || !strings.Contains(lastBody, "ttyS0") {
		t.Errorf("boot body: %q", lastBody)
	}

	// MachineConfig
	if err := c.MachineConfig(ctx, 2, 256); err != nil {
		t.Fatalf("MachineConfig: %v", err)
	}
	if !strings.Contains(lastBody, "256") || !strings.Contains(lastBody, "vcpu_count") {
		t.Errorf("machine body: %q", lastBody)
	}

	// StartInstance
	if err := c.StartInstance(ctx); err != nil {
		t.Fatalf("StartInstance: %v", err)
	}
	if !strings.Contains(lastBody, "InstanceStart") {
		t.Errorf("actions body: %q", lastBody)
	}
}

// TestJailerArgv verifies the jailer command line is correctly built.
func TestJailerArgv(t *testing.T) {
	a := JailerArgs{
		ID:            "vm1",
		ExecFile:      "/usr/bin/firecracker",
		UID:           1000,
		GID:           1000,
		ChrootBaseDir: "/srv/jailer",
		NetNS:         "/var/run/netns/vm1",
		APISocket:     "/run/vm1.sock",
	}
	argv := a.Argv()
	joined := strings.Join(argv, " ")
	checks := []string{
		"--id vm1",
		"--uid 1000",
		"--gid 1000",
		"--exec-file /usr/bin/firecracker",
		"--chroot-base-dir /srv/jailer",
		"--netns /var/run/netns/vm1",
		"--api-sock /run/vm1.sock",
	}
	for _, c := range checks {
		if !strings.Contains(joined, c) {
			t.Errorf("jailer argv missing %q: %s", c, joined)
		}
	}
}

// TestBackendNoKVM verifies the backend returns ErrNoKVM when /dev/kvm is
// not available (env-gated — this test PASSES on dev without KVM).
func TestBackendNoKVM(t *testing.T) {
	if kvmAvailable() {
		t.Skip("KVM available — skip no-KVM test")
	}
	b := NewFirecrackerBackend("/tmp/state", "/usr/bin/firecracker", "/usr/bin/jailer", "/path/vmlinux")
	ctx := context.Background()

	_, err := b.Run(ctx, "vm1", 1, 128, "/path/rootfs.ext4", "/path/vmlinux")
	if err != ErrNoKVM {
		t.Errorf("Run without KVM: got %v, want ErrNoKVM", err)
	}
	if err := b.Start(ctx, "vm1"); err != ErrNoKVM {
		t.Errorf("Start without KVM: got %v, want ErrNoKVM", err)
	}
	if err := b.Stop(ctx, "vm1"); err != ErrNoKVM {
		t.Errorf("Stop without KVM: got %v, want ErrNoKVM", err)
	}
}

// TestBackendRemoveWithoutKVM verifies Remove works even without KVM
// (cleanup-only — deletes files, no VMM interaction).
func TestBackendRemoveWithoutKVM(t *testing.T) {
	dir := t.TempDir()
	b := NewFirecrackerBackend(dir, "", "", "")
	// Create a fake instance dir
	instDir := filepath.Join(dir, "vm1")
	os.MkdirAll(instDir, 0755)
	if err := b.Remove(context.Background(), "vm1"); err != nil {
		t.Errorf("Remove: %v", err)
	}
	if _, err := os.Stat(instDir); !os.IsNotExist(err) {
		t.Errorf("instance dir should be gone after Remove")
	}
}
