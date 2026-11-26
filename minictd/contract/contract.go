// Package contract is the stable seam between the sandboxd control plane and
// a sandbox runtime.
//
// Every type below is copied 1:1 from sandboxd's
// control-plane/internal/docker/docker.go — identical field names, identical
// JSON tags, identical doc semantics — so a runtime swap changes nothing on
// the wire. The expected merge in sandboxd is:
//
//	docker.Client  -> satisfies contract.Runtime unchanged (types aliased)
//	minictd client -> contract.NewHTTPClient(base, token) satisfies it too
//
// Design rule inherited from internal/docker: NO MAGIC DEFAULTS here. Callers
// pass the full RunSpec; each runtime maps what it can and reports the rest
// through its own notes surface.
package contract

import (
	"context"
	"errors"
	"io"
)

// Version is the minictd release. It is reported by ServerVersion/Info.
const Version = "0.1.0"

// ErrNotFound is returned when the runtime reports the target does not
// exist. Same sentinel semantics as docker.ErrNotFound.
var ErrNotFound = errors.New("container not found")

// Runtime is the 14-operation contract sandboxd needs from any sandbox
// runtime. docker.Client satisfies it directly; minictd satisfies it over
// HTTP (see HTTPClient in httpclient.go).
type Runtime interface {
	Run(ctx context.Context, spec RunSpec) (string, error)
	Inspect(ctx context.Context, name string) (*ContainerJSON, error)
	Remove(ctx context.Context, name string) error
	Start(ctx context.Context, name string) error
	Stop(ctx context.Context, name string, timeoutSec int) error
	Exec(ctx context.Context, name string, cmd []string) (ExecResult, error)
	ExecV2(ctx context.Context, name string, req ExecV2Request) (ExecV2Result, error)
	ExecV2Stream(ctx context.Context, name string, req ExecV2Request) (
		stdout, stderr io.ReadCloser, wait func() (ExecV2Result, error), err error)
	Logs(ctx context.Context, name string, tail int, follow bool) (io.ReadCloser, error)
	ListByNamePrefix(ctx context.Context, prefix string) ([]string, error)
	Stats(ctx context.Context, name string) (Stats, error)
	ServerVersion(ctx context.Context) (string, error)
	Info(ctx context.Context) (string, error)
	NetworkSubnets(ctx context.Context, network string) (subnets, gateways []string, err error)
}

// RunSpec carries every flag the Phase 2/3 sandbox-up script emitted.
// No defaults; callers fill all of these explicitly.
//
// Field count: 24 (Name..Cmd below, in source order).
type RunSpec struct {
	Name        string   `json:"name"`         // --name
	Hostname    string   `json:"hostname"`     // --hostname
	Network     string   `json:"network"`      // --network (OSS: shared sandboxd_net so Traefik can route)
	Userns      string   `json:"userns"`       // --userns (OSS: "host" for deterministic workspace ownership)
	Runtime     string   `json:"runtime"`      // --runtime ("runsc"=gVisor); "" = daemon default (runc)
	ReadOnly    bool     `json:"read_only"`    // --read-only
	CapDrop     []string `json:"cap_drop"`     // --cap-drop=ALL (passed once per entry)
	SecurityOpt []string `json:"security_opt"` // --security-opt=no-new-privileges
	CPUShares   int      `json:"cpu_shares"`   // --cpu-shares=100
	CPUs        string   `json:"cpus"`         // --cpus (hard CPU limit); "" = none
	Memory      string   `json:"memory"`       // --memory=10g
	MemorySwap  string   `json:"memory_swap"`  // --memory-swap=10g
	PidsLimit   int      `json:"pids_limit"`   // --pids-limit=1024
	Ulimits     []string `json:"ulimits"`      // --ulimit nofile=65536:65536
	Tmpfs       []string `json:"tmpfs"`        // --tmpfs /tmp:size=512m   (one --tmpfs flag per entry)
	Env         []string `json:"env"`          // --env KEY=VAL   (one --env flag per entry)
	Volumes     []string `json:"volumes"`      // -v <host>:<container>[:opts]
	Labels      []string `json:"labels"`       // --label key=value
	User        string   `json:"user"`         // --user uid:gid ("" = the image's user)
	ShmSize     string   `json:"shm_size"`     // --shm-size (e.g. 1g for a browser); "" = docker default
	Restart     string   `json:"restart"`      // --restart policy ("" = none)
	Image       string   `json:"image"`        // last positional
	Cmd         []string `json:"cmd"`          // optional CMD override

	// SeccompProfile names an embedded allowlist
	// (internal/minict/profiles/<name>.allow, D59, issue #139). "" keeps
	// the D11 blocklist default. The allowlist is a NARROWER filter than
	// the blocklist, never a replacement: floor entries stay denied. It
	// rides spec.json, so save-template carries it to template:<name>
	// runs automatically. Trusted workloads only (manifest §3).
	SeccompProfile string `json:"seccomp_profile,omitempty"` // --seccomp-profile

	// Backend selects the runtime backend (GĐ4, issue #193).
	// "" = "minict" (default, container); "firecracker" = microVM.
	// Additive field — old callers that do not set it get "minict".
	Backend string `json:"backend,omitempty"`

	// PastaPorts declares per-instance host→namespace port forwards
	// served by pasta (passt project) attached to the sandbox netns by
	// the daemon (GĐ2 mục 4, issue #165, D67). Entry grammar — "tcp"
	// is the default protocol:
	//
	//	"8080:80"       forward host TCP 8080 -> sandbox port 80
	//	"tcp/5353:53"   explicit protocol
	//	"udp/53:53"     UDP
	//
	// Entries are validated fail-closed host-side (host port
	// 1024..65535 — pasta is rootless and cannot bind below; namespace
	// port 1..65535; no duplicate proto+host bind; ≤ 16 entries; no
	// ranges in v1). pasta is spawned OUT-OF-BAND by the daemon after
	// the payload is up (D37 mechanics: --userns/--netns via
	// /proc/<pid>/ns/*; the C shim is unaware of it, same host-side
	// posture as webproxy.py). Empty keeps the loopback-only netns
	// default (D17). It rides spec.json, so save-template carries the
	// forwards to template:<name> runs automatically.
	PastaPorts []string `json:"pasta_ports,omitempty"` // host:ns port forwards via pasta
}

// LabelMinictWebports is the reserved label key a control plane sets to ask
// minictd for preview relays: a comma-separated list of container ports
// ("3000,5173"). minictd exposes each as 127.0.0.1:<same port> on the host
// through the unix-socket bridge (minict D16/D24 + tools/webproxy.py).
const LabelMinictWebports = "minict.webport"

// ContainerJSON is the slice of `docker inspect` output the control plane
// cares about (identical shape to docker.ContainerJSON).
type ContainerJSON struct {
	ID    string `json:"Id"`
	State struct {
		Status  string `json:"Status"`
		Running bool   `json:"Running"`
		Pid     int    `json:"Pid"`
	} `json:"State"`
	Config struct {
		Image  string            `json:"Image"`
		Labels map[string]string `json:"Labels"`
		User   string            `json:"User"`
		Cmd    []string          `json:"Cmd"`
	} `json:"Config"`
	// Mounts lists what is mounted into the sandbox (bind / tmpfs).
	Mounts []Mount `json:"Mounts"`
	// NetworkSettings is the subset the wake-readiness probe needs.
	// minictd sandboxes have NO bridge IP (loopback-only netns, D17): both
	// fields stay empty and BridgeIP() returns "" — callers must treat an
	// empty IP as "probe via the preview relay" instead.
	NetworkSettings struct {
		IPAddress string                          `json:"IPAddress"`
		Networks  map[string]*NetworkEndpointInfo `json:"Networks"`
	} `json:"NetworkSettings"`
	// HostConfig carries the resource limits set on Run (docker inspect
	// parity). Populated from spec.json so the console can display what
	// was allocated (GĐ3 mục 1, issue #179). Added field — old callers
	// that do not read HostConfig are unaffected.
	HostConfig struct {
		NanoCpus  int64 `json:"NanoCpus"`  // spec.CPUs * 1e9 (1 CPU = 1e9 nano)
		Memory    int64 `json:"Memory"`    // spec.Memory in bytes
		PidsLimit int64 `json:"PidsLimit"` // spec.PidsLimit
	} `json:"HostConfig"`
}

// BridgeIP returns the sandbox bridge IP. For minictd it is always "".
// (Kept for interface parity with docker.ContainerJSON.)
func (cj *ContainerJSON) BridgeIP() string {
	if cj.NetworkSettings.IPAddress != "" {
		return cj.NetworkSettings.IPAddress
	}
	if n, ok := cj.NetworkSettings.Networks["bridge"]; ok && n != nil && n.IPAddress != "" {
		return n.IPAddress
	}
	for _, n := range cj.NetworkSettings.Networks {
		if n != nil && n.IPAddress != "" {
			return n.IPAddress
		}
	}
	return ""
}

// NetworkEndpointInfo carries per-network endpoint info.
type NetworkEndpointInfo struct {
	IPAddress string `json:"IPAddress"`
}

// Mount is one entry of `docker inspect .Mounts`.
type Mount struct {
	Type        string `json:"Type"` // "bind" | "tmpfs"
	Source      string `json:"Source"`
	Destination string `json:"Destination"`
	Mode        string `json:"Mode"`
	RW          bool   `json:"RW"`
}

// ExecResult carries the stdout/stderr/exit-code of a non-interactive exec.
type ExecResult struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
}

// ExecV2Request is the extended exec call: working directory, extra env,
// stdin, a hard timeout, and a bounded output buffer. Semantics identical
// to docker.ExecV2Request (see sandboxd internal/docker for the full
// rationale). One deviation under minictd: the timeout is enforced by the
// daemon (TERM to the exec process group, SIGKILL after kill-after 2s)
// instead of the in-container GNU `timeout` coreutils wrapper.
type ExecV2Request struct {
	Cmd []string `json:"cmd"`
	// WorkDir is passed as docker exec -w. "" = sandbox default (/).
	WorkDir string `json:"work_dir"`
	// Env entries passed as docker exec -e K=V. The API layer validates
	// them (no PATH/HOME/LD_* overrides); this layer only encodes.
	Env []string `json:"env"`
	// Stdin (optional) is fed to `docker exec -i`. nil = no stdin.
	Stdin []byte `json:"stdin,omitempty"`
	// TimeoutS > 0 arms the daemon-side watchdog: TERM to the whole exec
	// process group at expiry, SIGKILL 2s later; the result reports
	// TimedOut=true and ExitCode=124 (docker parity).
	TimeoutS int `json:"timeout_s"`
	// MaxOutputBytes > 0 bounds stdout+stderr independently: over the
	// limit the head and tail of the stream are kept (75%/25%) and
	// Truncated reports it. 0 = unbounded (legacy behaviour).
	MaxOutputBytes int64 `json:"max_output_bytes"`
}

// ExecV2Result adds timeout/truncation/duration to ExecResult.
type ExecV2Result struct {
	Stdout     string `json:"stdout"`
	Stderr     string `json:"stderr"`
	ExitCode   int    `json:"exit_code"`
	TimedOut   bool   `json:"timed_out"` // the daemon-side watchdog fired
	Truncated  bool   `json:"truncated"` // output exceeded MaxOutputBytes (head+tail kept)
	DurationMs int64  `json:"duration_ms"`
}

// ExecTimeoutExit mirrors GNU timeout's "command timed out" exit code —
// kept for docker-parity in the TimedOut mapping.
const ExecTimeoutExit = 124

// Stats is one resource sample (identical shape to docker.Stats).
// Under minictd: MemUsage is the RSS sum over the shim's process tree
// (cgroups v2 unavailable in this build, CG1); Net/Block counters are 0
// (loopback-only netns, no cgroup accounting); CPUPercent is sampled over
// a 250ms window from /proc, normalized to one core.
type Stats struct {
	CPUPercent    float64 `json:"cpu_percent"`
	MemUsageBytes int64   `json:"mem_usage_bytes"`
	MemLimitBytes int64   `json:"mem_limit_bytes"`
	MemPercent    float64 `json:"mem_percent"`
	NetRxBytes    int64   `json:"net_rx_bytes"`
	NetTxBytes    int64   `json:"net_tx_bytes"`
	BlockRead     int64   `json:"block_read_bytes"`
	BlockWrite    int64   `json:"block_write_bytes"`
	PIDs          int     `json:"pids"`
}
