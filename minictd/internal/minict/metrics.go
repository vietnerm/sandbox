// Package minict — metrics.go (GĐ2 mục 7, issue #168, D68).
//
// Prometheus text exposition renderer + scrape-time snapshot collector for
// minictd observability. Stdlib only (no prometheus/client_golang — manifest
// ràng buộc). Reads cgroup v2 files tái dùng cgReadInt/cgCPUStatUsage (cgroup.go,
// #4/D48) + thêm cgIOStat (sum rbytes/wbytes qua device). Fail-soft: instance
// không có cgroup → chỉ Up/State, không bịa 0.
package minict

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/vietnerm/sandbox/minictd/contract"
)

// MetricsSnapshot is the scrape-time view rendered to Prometheus text.
type MetricsSnapshot struct {
	Daemon    DaemonMetrics
	Instances []InstanceMetrics
}

// DaemonMetrics holds daemon-level gauges.
type DaemonMetrics struct {
	RSSBytes       int64               // /proc/self/status VmRSS
	InstanceStates map[string]int      // state -> count
	Version        string              // contract.Version
	BuildInfo      map[string]string   // extra labels (go_version, runtime)
}

// InstanceMetrics holds per-instance gauges/counters.
type InstanceMetrics struct {
	Name        string
	State       string
	Up          float64 // 1.0 running, 0.0 otherwise
	CPUUsageSec float64 // cpu.stat usage_usec / 1e6 (counter)
	MemBytes    int64   // memory.current (gauge)
	MemLimit    int64   // memory.max bytes (0 = max/unset)
	IORead      int64   // io.stat rbytes sum (counter)
	IOWrite     int64   // io.stat wbytes sum (counter)
	PIDs        int64   // pids.current (gauge)
	PIDsLimit   int64   // pids.max (0 = max)
	HasCgroup   bool    // cgroup accounting available
}

// CollectMetrics iterates all instances via ListByNamePrefix, reads cgroup
// files per-instance (scrape-time, no goroutine, no RAM history), and
// returns a snapshot. Daemon RSS from /proc/self/status. Used by the
// /metrics endpoint (server.go). Fail-soft per-instance: a missing or
// unreadable cgroup yields HasCgroup=false (Up/State only).
func (s *Store) CollectMetrics() MetricsSnapshot {
	snap := MetricsSnapshot{
		Daemon: DaemonMetrics{
			InstanceStates: map[string]int{},
			Version:        contract.Version,
			BuildInfo: map[string]string{
				"go_version":   runtime.Version(),
				"runtime":      runtime.GOOS + "/" + runtime.GOARCH,
			},
			RSSBytes: daemonRSSBytes(),
		},
	}
	names, err := s.ListByNamePrefix(context.TODO(), "")
	if err != nil {
		return snap
	}
	for _, name := range names {
		st, err := s.loadState(name)
		if err != nil {
			continue
		}
		st = s.alive(st)
		snap.Daemon.InstanceStates[st.State]++
		im := InstanceMetrics{
			Name:  name,
			State: st.State,
		}
		if st.State == StateRunning {
			im.Up = 1.0
		}
		if st.CgroupPath != "" {
			if mem, ok := cgReadInt(filepath.Join(st.CgroupPath, "memory.current")); ok {
				im.MemBytes = mem
				im.HasCgroup = true
			}
			if lim, ok := cgReadInt(filepath.Join(st.CgroupPath, "memory.max")); ok && lim > 0 {
				im.MemLimit = lim
			}
			if p, ok := cgReadInt(filepath.Join(st.CgroupPath, "pids.current")); ok {
				im.PIDs = p
				im.HasCgroup = true
			}
			if lim, ok := cgReadInt(filepath.Join(st.CgroupPath, "pids.max")); ok && lim > 0 {
				im.PIDsLimit = lim
			}
			if usec := cgCPUStatUsage(st.CgroupPath); usec > 0 {
				im.CPUUsageSec = float64(usec) / 1e6
				im.HasCgroup = true
			}
			if r, w := cgIOStat(st.CgroupPath); r > 0 || w > 0 {
				im.IORead = r
				im.IOWrite = w
				im.HasCgroup = true
			}
		}
		snap.Instances = append(snap.Instances, im)
	}
	// Stable order: sort by name so render output is deterministic.
	sort.Slice(snap.Instances, func(i, j int) bool {
		return snap.Instances[i].Name < snap.Instances[j].Name
	})
	return snap
}

// cgIOStat reads io.stat and returns summed (rbytes, wbytes) across all
// devices. io.stat is nested-keyed: each line "<MAJ>:<MIN> rbytes=N wbytes=N
// rios=N wios=N dbytes=N dios=N". Returns (0, 0) when the file is absent or
// no device lines parse.
func cgIOStat(cgPath string) (rbytes, wbytes int64) {
	b, err := os.ReadFile(filepath.Join(cgPath, "io.stat"))
	if err != nil {
		return 0, 0
	}
	for _, line := range strings.Split(string(b), "\n") {
		if line == "" {
			continue
		}
		// first token is "<MAJ>:<MIN>"; remaining are key=value
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		for _, f := range fields[1:] {
			k, v, ok := strings.Cut(f, "=")
			if !ok {
				continue
			}
			n, err := strconv.ParseInt(v, 10, 64)
			if err != nil {
				continue
			}
			switch k {
			case "rbytes":
				rbytes += n
			case "wbytes":
				wbytes += n
			}
		}
	}
	return rbytes, wbytes
}

// daemonRSSBytes reads /proc/self/status VmRSS (kilobytes → bytes).
func daemonRSSBytes() int64 {
	f, err := os.Open("/proc/self/status")
	if err != nil {
		return 0
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "VmRSS:") {
			continue
		}
		// "VmRSS:\t   1234 kB"
		rest := strings.TrimSpace(strings.TrimPrefix(line, "VmRSS:"))
		rest = strings.TrimSuffix(rest, "kB")
		rest = strings.TrimSpace(rest)
		n, err := strconv.ParseInt(rest, 10, 64)
		if err != nil {
			return 0
		}
		return n * 1024
	}
	return 0
}

// promMetric describes one metric family (name, help, type, samples).
type promMetric struct {
	name    string
	help    string
	mtype   string // "counter" | "gauge"
	samples []promSample
}

// promSample is one metric line: labels (sorted) + value.
type promSample struct {
	labels [][2]string // (name, value) pairs
	value  float64
}

// renderPrometheus emits the Prometheus text exposition format (v0.0.4).
// Output is deterministic: metrics in registration order, samples sorted by
// label set within each metric. Content-Type should be
// "text/plain; version=0.0.4; charset=utf-8" (set by the HTTP handler).
//
// Spec: https://github.com/prometheus/docs/blob/main/docs/instrumenting/exposition_formats.md
//   - Lines separated by \n; last line ends with \n.
//   - # HELP and # TYPE before first sample of a metric.
//   - Label values escape \ → \\, " → \", LF → \n.
//   - Sample: name{labels} value [timestamp_ms].
func renderPrometheus(snap MetricsSnapshot) []byte {
	// Build metrics in a stable registration order.
	var ms []promMetric

	// 1. minict_instance_up (gauge)
	up := promMetric{name: "minict_instance_up", help: "1 if instance state is running, 0 otherwise", mtype: "gauge"}
	// 2. minict_instance_cpu_usage_seconds_total (counter)
	cpu := promMetric{name: "minict_instance_cpu_usage_seconds_total", help: "Total CPU time consumed by the instance in seconds (cgroup cpu.stat usage_usec / 1e6)", mtype: "counter"}
	// 3. minict_instance_memory_bytes (gauge)
	mem := promMetric{name: "minict_instance_memory_bytes", help: "Current memory usage in bytes (cgroup memory.current)", mtype: "gauge"}
	// 4. minict_instance_memory_limit_bytes (gauge)
	memlim := promMetric{name: "minict_instance_memory_limit_bytes", help: "Memory hard limit in bytes (cgroup memory.max; 0 = unlimited)", mtype: "gauge"}
	// 5. minict_instance_io_read_bytes_total (counter)
	iord := promMetric{name: "minict_instance_io_read_bytes_total", help: "Total bytes read (cgroup io.stat rbytes summed across devices)", mtype: "counter"}
	// 6. minict_instance_io_write_bytes_total (counter)
	iowr := promMetric{name: "minict_instance_io_write_bytes_total", help: "Total bytes written (cgroup io.stat wbytes summed across devices)", mtype: "counter"}
	// 7. minict_instance_pids (gauge)
	pids := promMetric{name: "minict_instance_pids", help: "Current number of processes (cgroup pids.current)", mtype: "gauge"}
	// 8. minict_instance_pids_limit (gauge)
	pidslim := promMetric{name: "minict_instance_pids_limit", help: "PID hard limit (cgroup pids.max; 0 = unlimited)", mtype: "gauge"}

	for _, im := range snap.Instances {
		lbl := [][2]string{{"instance", im.Name}, {"state", im.State}}
		up.samples = append(up.samples, promSample{labels: lbl, value: im.Up})
		// Only emit resource metrics when cgroup accounting is on (fail-soft:
		// instance without cgroup → only up/state, never fake 0).
		if im.HasCgroup {
			cpu.samples = append(cpu.samples, promSample{labels: [][2]string{{"instance", im.Name}}, value: im.CPUUsageSec})
			mem.samples = append(mem.samples, promSample{labels: [][2]string{{"instance", im.Name}}, value: float64(im.MemBytes)})
			memlim.samples = append(memlim.samples, promSample{labels: [][2]string{{"instance", im.Name}}, value: float64(im.MemLimit)})
			iord.samples = append(iord.samples, promSample{labels: [][2]string{{"instance", im.Name}}, value: float64(im.IORead)})
			iowr.samples = append(iowr.samples, promSample{labels: [][2]string{{"instance", im.Name}}, value: float64(im.IOWrite)})
			pids.samples = append(pids.samples, promSample{labels: [][2]string{{"instance", im.Name}}, value: float64(im.PIDs)})
			pidslim.samples = append(pidslim.samples, promSample{labels: [][2]string{{"instance", im.Name}}, value: float64(im.PIDsLimit)})
		}
	}

	// Daemon metrics.
	dinst := promMetric{name: "minictd_instances", help: "Number of instances managed by minictd, by state", mtype: "gauge"}
	states := make([]string, 0, len(snap.Daemon.InstanceStates))
	for st := range snap.Daemon.InstanceStates {
		states = append(states, st)
	}
	sort.Strings(states)
	for _, st := range states {
		dinst.samples = append(dinst.samples, promSample{
			labels: [][2]string{{"state", st}},
			value:  float64(snap.Daemon.InstanceStates[st]),
		})
	}

	drss := promMetric{name: "minictd_process_resident_memory_bytes", help: "Resident memory of the minictd process in bytes (/proc/self/status VmRSS)", mtype: "gauge"}
	drss.samples = append(drss.samples, promSample{value: float64(snap.Daemon.RSSBytes)})

	dbi := promMetric{name: "minictd_build_info", help: "minictd build information", mtype: "gauge"}
	biLabels := [][2]string{{"version", snap.Daemon.Version}}
	// Add build_info labels in a stable order (go_version, runtime).
	biExtra := make([]string, 0, len(snap.Daemon.BuildInfo))
	for k := range snap.Daemon.BuildInfo {
		biExtra = append(biExtra, k)
	}
	sort.Strings(biExtra)
	for _, k := range biExtra {
		biLabels = append(biLabels, [2]string{k, snap.Daemon.BuildInfo[k]})
	}
	dbi.samples = append(dbi.samples, promSample{labels: biLabels, value: 1})

	ms = append(ms, up, cpu, mem, memlim, iord, iowr, pids, pidslim, dinst, drss, dbi)

	// Render. Each metric group: optional HELP, optional TYPE, then samples.
	// A metric with no samples still emits HELP+TYPE (declares the family so
	// scrapers know it exists even when empty).
	var b strings.Builder
	for _, m := range ms {
		fmt.Fprintf(&b, "# HELP %s %s\n", m.name, promEscStr(m.help))
		fmt.Fprintf(&b, "# TYPE %s %s\n", m.name, m.mtype)
		// Sort samples by label set within each metric for determinism.
		sortPromSamples(m.samples)
		for _, s := range m.samples {
			b.WriteString(m.name)
			if len(s.labels) > 0 {
				b.WriteByte('{')
				for i, lbl := range s.labels {
					if i > 0 {
						b.WriteByte(',')
					}
					b.WriteString(lbl[0])
					b.WriteString(`="`)
					b.WriteString(promEscStr(lbl[1]))
					b.WriteByte('"')
				}
				b.WriteByte('}')
			}
			b.WriteByte(' ')
			// Use 'g' format to avoid trailing zeros and scientific notation
			// for typical small integers; Prometheus accepts both but g is
			// human-readable.
			b.WriteString(strconv.FormatFloat(s.value, 'g', -1, 64))
			b.WriteByte('\n')
		}
	}
	return []byte(b.String())
}

// sortPromSamples sorts by label set lexicographically (deterministic output).
// Samples with no labels come first (empty < any).
func sortPromSamples(ss []promSample) {
	sort.SliceStable(ss, func(i, j int) bool {
		return promSampleLess(ss[i], ss[j])
	})
}

func promSampleLess(a, b promSample) bool {
	la, lb := a.labels, b.labels
	n := len(la)
	if len(lb) < n {
		n = len(lb)
	}
	for i := 0; i < n; i++ {
		if la[i][0] != lb[i][0] {
			return la[i][0] < lb[i][0]
		}
		if la[i][1] != lb[i][1] {
			return la[i][1] < lb[i][1]
		}
	}
	if len(la) != len(lb) {
		return len(la) < len(lb)
	}
	return a.value < b.value
}

// promEscStr escapes a label value or help docstring per Prometheus spec:
// backslash → \\, double-quote → \", line feed → \n.
func promEscStr(s string) string {
	if !strings.ContainsAny(s, "\\\"\n") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 4)
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '\n':
			b.WriteString(`\n`)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// RenderMetrics is the convenience wrapper used by the HTTP handler: collect
// then render. Exposed so tests can call it end-to-end against a Store.
func (s *Store) RenderMetrics() []byte {
	return renderPrometheus(s.CollectMetrics())
}

// MetricsNow returns the current time — exposed for tests that want to
// assert the snapshot collection does not block longer than a budget.
// (Currently unused by the handler which does not attach timestamps to
// samples; Prometheus uses the scrape time.)
func MetricsNow() time.Time { return time.Now() }
