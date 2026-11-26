// Package firecracker — GĐ4 mục 1 (issue #193).
//
// Firecracker API client over unix socket. The Firecracker VMM exposes an
// HTTP API on a unix socket (per-microVM); this client wraps the PUT/GET
// calls needed to configure and control a microVM.
//
// Reference: https://github.com/firecracker-microvm/firecracker/blob/main/docs/api_requests.md
//
// This package compiles WITHOUT /dev/kvm — the API client is pure HTTP over
// unix socket. KVM is only needed at RUNTIME when InstanceStart is called.
package firecracker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"
)

// Client is the Firecracker API client (one per microVM).
type Client struct {
	socketPath string
	hc         *http.Client
}

// NewClient creates a client for the Firecracker API at socketPath.
func NewClient(socketPath string) *Client {
	return &Client{
		socketPath: socketPath,
		hc: &http.Client{
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
					var d net.Dialer
					return d.DialContext(ctx, "unix", socketPath)
				},
			},
			Timeout: 10 * time.Second,
		},
	}
}

// put sends a PUT request with a JSON body to the Firecracker API.
func (c *Client) put(ctx context.Context, path string, body any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("firecracker api: marshal %s: %w", path, err)
	}
	req, err := http.NewRequestWithContext(ctx, "PUT", "http://unix"+path, bytes.NewReader(b))
	if err != nil {
		return fmt.Errorf("firecracker api: request %s: %w", path, err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("firecracker api: do %s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("firecracker api: %s: status %d", path, resp.StatusCode)
	}
	return nil
}

// BootSource configures the kernel + boot args.
func (c *Client) BootSource(ctx context.Context, kernelPath, bootArgs string) error {
	return c.put(ctx, "/boot-source", map[string]string{
		"kernel_image_path": kernelPath,
		"boot_args":         bootArgs,
	})
}

// Drive configures a block device (rootfs).
func (c *Client) Drive(ctx context.Context, driveID, path string, readOnly bool) error {
	return c.put(ctx, "/drives/"+driveID, map[string]any{
		"drive_id":       driveID,
		"path_on_host":   path,
		"is_root_device": true,
		"is_read_only":   readOnly,
	})
}

// MachineConfig sets vCPU count + memory.
func (c *Client) MachineConfig(ctx context.Context, vcpuCount int, memSizeMib int) error {
	return c.put(ctx, "/machine-config", map[string]any{
		"vcpu_count":   vcpuCount,
		"mem_size_mib": memSizeMib,
	})
}

// NetworkInterface configures a TAP device for guest networking.
func (c *Client) NetworkInterface(ctx context.Context, ifaceID, tapDev, guestMac string) error {
	return c.put(ctx, "/network-interfaces/"+ifaceID, map[string]any{
		"iface_id":      ifaceID,
		"host_dev_name": tapDev,
		"guest_mac":     guestMac,
	})
}

// StartInstance boots the microVM (PUT /actions with InstanceStart).
func (c *Client) StartInstance(ctx context.Context) error {
	return c.put(ctx, "/actions", map[string]string{
		"action_type": "InstanceStart",
	})
}

// HaltInstance sends CtrlAltDel (graceful shutdown).
func (c *Client) HaltInstance(ctx context.Context) error {
	return c.put(ctx, "/actions", map[string]string{
		"action_type": "CtrlAltDel",
	})
}

// Logger configures the serial console output to a unix socket.
func (c *Client) Logger(ctx context.Context, socketPath string) error {
	return c.put(ctx, "/logger", map[string]string{
		"log_path":        socketPath,
		"level":           "Info",
		"show_level":      "true",
		"show_log_origin": "false",
	})
}
