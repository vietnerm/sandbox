// Package minict — metrics_test.go (GĐ2 mục 7, issue #168, D68).
//
// Unit tests for the Prometheus renderer + cgroup parsers. Mutants
// (M-render-escape, M-render-order, M-io-stat-sum, M-no-cgroup-skip) must
// each break at least one assertion below (negative control for SIA).
package minict

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRenderPrometheusEmpty verifies HELP/TYPE are emitted even when there
// are zero instances (metric families declared), and the output ends with
// a trailing newline.
func TestRenderPrometheusEmpty(t *testing.T) {
	out := renderPrometheus(MetricsSnapshot{
		Daemon: DaemonMetrics{
			InstanceStates: map[string]int{},
			Version:        "test",
			BuildInfo:      map[string]string{},
		},
	})
	s := string(out)
	if !strings.HasSuffix(s, "\n") {
		t.Fatalf("output must end with newline; got tail %q", s[max(0, len(s)-20):])
	}
	// All 11 metric families declared.
	want := []string{
		"# HELP minict_instance_up",
		"# TYPE minict_instance_up gauge",
		"# HELP minict_instance_cpu_usage_seconds_total",
		"# TYPE minict_instance_cpu_usage_seconds_total counter",
		"# HELP minict_instance_memory_bytes",
		"# TYPE minict_instance_memory_bytes gauge",
		"# HELP minict_instance_memory_limit_bytes",
		"# TYPE minict_instance_memory_limit_bytes gauge",
		"# HELP minict_instance_io_read_bytes_total",
		"# TYPE minict_instance_io_read_bytes_total counter",
		"# HELP minict_instance_io_write_bytes_total",
		"# TYPE minict_instance_io_write_bytes_total counter",
		"# HELP minict_instance_pids",
		"# TYPE minict_instance_pids gauge",
		"# HELP minict_instance_pids_limit",
		"# TYPE minict_instance_pids_limit gauge",
		"# HELP minictd_instances",
		"# TYPE minictd_instances gauge",
		"# HELP minictd_process_resident_memory_bytes",
		"# TYPE minictd_process_resident_memory_bytes gauge",
		"# HELP minictd_build_info",
		"# TYPE minictd_build_info gauge",
	}
	for _, w := range want {
		if !strings.Contains(s, w) {
			t.Errorf("missing %q in output", w)
		}
	}
}

// TestRenderPrometheusInstanceWithCgroup verifies resource metrics are
// emitted (cpu/memory/io/pids) for an instance with cgroup accounting.
func TestRenderPrometheusInstanceWithCgroup(t *testing.T) {
	snap := MetricsSnapshot{
		Daemon: DaemonMetrics{InstanceStates: map[string]int{"running": 1}, Version: "v1"},
		Instances: []InstanceMetrics{
			{
				Name: "web", State: "running", Up: 1.0, HasCgroup: true,
				CPUUsageSec: 0.5, MemBytes: 1048576, MemLimit: 10485760,
				IORead: 1024, IOWrite: 2048, PIDs: 5, PIDsLimit: 100,
			},
		},
	}
	s := string(renderPrometheus(snap))
	checks := []string{
		`minict_instance_up{instance="web",state="running"} 1`,
		`minict_instance_cpu_usage_seconds_total{instance="web"} 0.5`,
		`minict_instance_memory_bytes{instance="web"} 1048576`,
		`minict_instance_memory_limit_bytes{instance="web"} 10485760`,
		`minict_instance_io_read_bytes_total{instance="web"} 1024`,
		`minict_instance_io_write_bytes_total{instance="web"} 2048`,
		`minict_instance_pids{instance="web"} 5`,
		`minict_instance_pids_limit{instance="web"} 100`,
		`minictd_instances{state="running"} 1`,
		`minictd_build_info{version="v1"} 1`,
	}
	for _, c := range checks {
		if !strings.Contains(s, c) {
			t.Errorf("missing sample %q", c)
		}
	}
}

// TestRenderPrometheusInstanceWithoutCgroup verifies fail-soft: an instance
// with HasCgroup=false emits ONLY up/state — no fake-0 resource metrics.
// Mutant M-no-cgroup-skip (emit 0 for all) breaks this test.
func TestRenderPrometheusInstanceWithoutCgroup(t *testing.T) {
	snap := MetricsSnapshot{
		Daemon: DaemonMetrics{InstanceStates: map[string]int{"stopped": 1}, Version: "v1"},
		Instances: []InstanceMetrics{
			{Name: "dead", State: "stopped", Up: 0.0, HasCgroup: false},
		},
	}
	s := string(renderPrometheus(snap))
	if !strings.Contains(s, `minict_instance_up{instance="dead",state="stopped"} 0`) {
		t.Errorf("up/state must be emitted for cgroup-less instance")
	}
	// None of the resource metrics should reference this instance.
	forbidden := []string{
		`minict_instance_cpu_usage_seconds_total{instance="dead"`,
		`minict_instance_memory_bytes{instance="dead"`,
		`minict_instance_io_read_bytes_total{instance="dead"`,
		`minict_instance_pids{instance="dead"`,
	}
	for _, f := range forbidden {
		if strings.Contains(s, f) {
			t.Errorf("cgroup-less instance must NOT emit resource metric: %q", f)
		}
	}
}

// TestRenderPrometheusLabelEscape verifies backslash/quote/LF escaping in
// label values. Mutant M-render-escape (no escaping) breaks this.
func TestRenderPrometheusLabelEscape(t *testing.T) {
	snap := MetricsSnapshot{
		Daemon: DaemonMetrics{InstanceStates: map[string]int{}, Version: "v"},
		Instances: []InstanceMetrics{
			{Name: `a"b\c` + "\n", State: "running", Up: 1, HasCgroup: true,
				MemBytes: 1},
		},
	}
	s := string(renderPrometheus(snap))
	// Escaped form: a\"b\\c\n (backslash-quote, backslash-backslash, backslash-n)
	if !strings.Contains(s, `minict_instance_memory_bytes{instance="a\"b\\c\n"} 1`) {
		t.Errorf("label value not escaped correctly; output:\n%s", s)
	}
}

// TestRenderPrometheusStableOrder verifies samples within a metric are sorted
// by label set so repeated scrapes are deterministic (MD42 counter-not-decrease
// depends on stable output). Mutant M-render-order (no sort) breaks this when
// instances are added out of order.
func TestRenderPrometheusStableOrder(t *testing.T) {
	snap := MetricsSnapshot{
		Daemon: DaemonMetrics{InstanceStates: map[string]int{}, Version: "v"},
		Instances: []InstanceMetrics{
			{Name: "zeta", State: "running", Up: 1, HasCgroup: true, MemBytes: 3},
			{Name: "alpha", State: "running", Up: 1, HasCgroup: true, MemBytes: 1},
			{Name: "mid", State: "running", Up: 1, HasCgroup: true, MemBytes: 2},
		},
	}
	s := string(renderPrometheus(snap))
	// alpha must appear before mid before zeta in the memory_bytes metric.
	idxA := strings.Index(s, `minict_instance_memory_bytes{instance="alpha"`)
	idxM := strings.Index(s, `minict_instance_memory_bytes{instance="mid"`)
	idxZ := strings.Index(s, `minict_instance_memory_bytes{instance="zeta"`)
	if idxA < 0 || idxM < 0 || idxZ < 0 {
		t.Fatalf("missing one of alpha/mid/zeta: a=%d m=%d z=%d", idxA, idxM, idxZ)
	}
	if !(idxA < idxM && idxM < idxZ) {
		t.Errorf("samples not sorted by label set: alpha=%d mid=%d zeta=%d", idxA, idxM, idxZ)
	}
}

// TestCgIOStatSum verifies io.stat parsing sums rbytes/wbytes across devices.
// Mutant M-io-stat-sum (return only first device, or swap rbytes/wbytes)
// breaks this.
func TestCgIOStatSum(t *testing.T) {
	dir := t.TempDir()
	// Two devices, mixed key order, with extra keys to ignore.
	content := "8:0 rbytes=1000 wbytes=2000 rios=10 wios=20 dbytes=0 dios=0\n" +
		"8:16 wbytes=4000 rbytes=3000 rios=5 wios=8 dbytes=100 dios=2\n"
	if err := os.WriteFile(filepath.Join(dir, "io.stat"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	r, w := cgIOStat(dir)
	if r != 4000 {
		t.Errorf("rbytes: want 4000 (1000+3000), got %d", r)
	}
	if w != 6000 {
		t.Errorf("wbytes: want 6000 (2000+4000), got %d", w)
	}
}

// TestCgIOStatMissingFile verifies absent io.stat returns (0,0) not error.
func TestCgIOStatMissingFile(t *testing.T) {
	r, w := cgIOStat(t.TempDir())
	if r != 0 || w != 0 {
		t.Errorf("missing io.stat: want (0,0), got (%d,%d)", r, w)
	}
}

// TestCgIOStatEmptyFile verifies empty io.stat returns (0,0).
func TestCgIOStatEmptyFile(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "io.stat"), []byte(""), 0644)
	r, w := cgIOStat(dir)
	if r != 0 || w != 0 {
		t.Errorf("empty io.stat: want (0,0), got (%d,%d)", r, w)
	}
}

// TestDaemonRSSBytesSmoke just verifies the function does not panic and
// returns a non-negative value (the actual RSS varies). Skip if /proc
// unavailable.
func TestDaemonRSSBytesSmoke(t *testing.T) {
	v := daemonRSSBytes()
	if v < 0 {
		t.Errorf("RSS must be non-negative, got %d", v)
	}
}

// TestRenderPrometheusDaemonInstancesStates verifies minictd_instances emits
// one sample per state, sorted by state label.
func TestRenderPrometheusDaemonInstancesStates(t *testing.T) {
	snap := MetricsSnapshot{
		Daemon: DaemonMetrics{
			InstanceStates: map[string]int{"running": 2, "stopped": 3, "creating": 1},
			Version:        "v",
		},
	}
	s := string(renderPrometheus(snap))
	// States must appear sorted: creating, running, stopped.
	idxC := strings.Index(s, `minictd_instances{state="creating"}`)
	idxR := strings.Index(s, `minictd_instances{state="running"}`)
	idxS := strings.Index(s, `minictd_instances{state="stopped"}`)
	if idxC < 0 || idxR < 0 || idxS < 0 {
		t.Fatalf("missing state samples")
	}
	if !(idxC < idxR && idxR < idxS) {
		t.Errorf("states not sorted: creating=%d running=%d stopped=%d", idxC, idxR, idxS)
	}
	if !strings.Contains(s, `minictd_instances{state="running"} 2`) {
		t.Errorf("running count wrong")
	}
}
