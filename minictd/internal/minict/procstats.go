package minict

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// procStat parses /proc/<pid>/stat fields 1-3 and 22 the pid-reuse-safe
// way: state (field 3) and starttime (field 22, ticks since boot). comm
// (field 2) can contain spaces and parens, so we cut around the LAST ')'.
func procStat(pid int) (state byte, startTicks uint64, ok bool) {
	b, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return 0, 0, false
	}
	r := string(b)
	i := strings.LastIndex(r, ")")
	if i < 0 || i+2 >= len(r) {
		return 0, 0, false
	}
	rest := strings.Fields(r[i+2:]) // starts at field 3 (state)
	if len(rest) < 20 {
		return 0, 0, false
	}
	state = rest[0][0]
	startTicks, err = strconv.ParseUint(rest[19], 10, 64) // field 22 overall
	if err != nil {
		return 0, 0, false
	}
	return state, startTicks, true
}

// procCPU returns utime+stime of a pid in clock ticks (fields 14+15).
func procCPU(pid int) (uint64, bool) {
	b, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return 0, false
	}
	r := string(b)
	i := strings.LastIndex(r, ")")
	if i < 0 {
		return 0, false
	}
	f := strings.Fields(r[i+2:]) // fields 3..; utime is index 11, stime 12
	if len(f) < 13 {
		return 0, false
	}
	u, err1 := strconv.ParseUint(f[11], 10, 64)
	s, err2 := strconv.ParseUint(f[12], 10, 64)
	if err1 != nil || err2 != nil {
		return 0, false
	}
	return u + s, true
}

// descendants walks /proc and returns the pid + all its descendants (host
// view). All minictd processes belong to the daemon's own user, so the
// reads succeed; anything unreadable is skipped silently.
func descendants(root int) []int {
	children := make(map[int][]int)
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return []int{root}
	}
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		b, err := os.ReadFile("/proc/" + e.Name() + "/stat")
		if err != nil {
			continue
		}
		r := string(b)
		i := strings.LastIndex(r, ")")
		if i < 0 || i+2 >= len(r) {
			continue
		}
		f := strings.Fields(r[i+2:])
		if len(f) < 2 {
			continue
		}
		ppid, err := strconv.Atoi(f[1])
		if err != nil {
			continue
		}
		children[ppid] = append(children[ppid], pid)
	}
	out := []int{}
	stack := []int{root}
	seen := map[int]bool{root: true}
	for len(stack) > 0 {
		p := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		out = append(out, p)
		for _, c := range children[p] {
			if !seen[c] {
				seen[c] = true
				stack = append(stack, c)
			}
		}
	}
	return out
}

// procRSS reads VmRSS (kB) from /proc/<pid>/status.
func procRSS(pid int) (int64, bool) {
	b, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
	if err != nil {
		return 0, false
	}
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, "VmRSS:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				kb, err := strconv.ParseInt(fields[1], 10, 64)
				if err == nil {
					return kb * 1024, true
				}
			}
		}
	}
	return 0, false
}

// memInfo returns MemTotal and MemAvailable (bytes) from /proc/meminfo.
func memInfo() (total, avail int64) {
	b, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, 0
	}
	get := func(key string) int64 {
		for _, line := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(line, key+":") {
				fields := strings.Fields(line)
				if len(fields) >= 2 {
					kb, err := strconv.ParseInt(fields[1], 10, 64)
					if err == nil {
						return kb * 1024
					}
				}
			}
		}
		return 0
	}
	return get("MemTotal"), get("MemAvailable")
}

// clockHertz is USER_HZ on stock Linux kernels (the only place /proc stat
// tick fields are scaled). Hardcoded with intent; getconf CLK_TCK agrees
// everywhere we have measured.
const clockHertz = 100

// cpuSample takes a short-window CPU% reading over the shim's whole
// process tree, normalized to one core (docker stats semantics). The
// 250ms window keeps the Stats endpoint latency bounded.
func cpuSample(root int, window time.Duration) float64 {
	t1 := uint64(0)
	for _, p := range descendants(root) {
		if t, ok := procCPU(p); ok {
			t1 += t
		}
	}
	time.Sleep(window)
	t2 := uint64(0)
	for _, p := range descendants(root) {
		if t, ok := procCPU(p); ok {
			t2 += t
		}
	}
	dt := float64(t2 - t1)
	secs := window.Seconds()
	if secs <= 0 {
		return 0
	}
	percent := dt / (clockHertz * secs) * 100.0
	if percent < 0 {
		return 0
	}
	return percent
}

// rssTree sums VmRSS over the shim's process tree. NOT a cgroup charge:
// shared pages are double-counted across the tree. Honest approximation,
// documented in contract.Stats.
func rssTree(root int) int64 {
	var sum int64
	for _, p := range descendants(root) {
		if rss, ok := procRSS(p); ok {
			sum += rss
		}
	}
	return sum
}
