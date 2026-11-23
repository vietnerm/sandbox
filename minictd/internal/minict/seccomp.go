package minict

import (
	"embed"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// seccomp allowlist profiles (GĐ2 mục 3a, issue #139, D59).
//
// A profile is a text file, one "nr name" pair per line, '#' comments:
// the x86_64 syscall numbers the template's MEASURED workload needs
// (tools/seccomp-profile-gen.sh, ptrace-based, D11 blocklist active during
// measurement). renderSpec embeds the numbers as `seccomp_allow` lines in
// the MINICT_SPEC (payload path) and exec.go mirrors them as
// MINICT_SECCOMP_ALLOW= lines in the setns env-file; the C side builds a
// BPF program in which the D11 blocklist stays a floor and everything not
// listed gets EPERM. Trusted workloads only (manifest §3): an allowlist
// narrows the kernel surface, it does not make minict a security boundary.
//
// go:embed needs module-relative paths, so the files live under
// internal/minict/profiles/ (issue #29 sketched seccomp/profiles/ — the
// concrete location is recorded in journal/2026-11-23/decisions/).
//
//go:embed profiles/*.allow
var seccompProfiles embed.FS

// SeccompAllowMax mirrors SECCOMP_ALLOW_MAX in sandbox/minict.c and
// minictd/cmd/minict-exec/minict-exec.c (MD0 parity covers the C copies;
// the daemon refuses to render more than the C cap accepts, fail-closed).
const SeccompAllowMax = 512

// seccompProfileMaxNr mirrors the C range check (cap 511): anything above
// is not a valid x86_64 syscall number the filter can match.
const seccompProfileMaxNr = 511

// LoadSeccompProfile parses an embedded profile into sorted unique syscall
// numbers. Unknown profile names are an ERROR, not a Note: a typo in a
// security setting must fail the Run (fail-closed), not silently fall back
// to blocklist-only.
func LoadSeccompProfile(name string) ([]int, error) {
	if name == "" {
		return nil, fmt.Errorf("seccomp profile name is empty")
	}
	// name becomes part of the embed lookup and the report lines; keep it
	// a safe file component (same charset the instance name uses).
	if !validSeccompName(name) {
		return nil, fmt.Errorf("seccomp profile %q: name must match [a-z0-9][a-z0-9.-]*", name)
	}
	data, err := seccompProfiles.ReadFile("profiles/" + name + ".allow")
	if err != nil {
		return nil, fmt.Errorf("seccomp profile %q: unknown profile (embedded: run tools/seccomp-profile-gen.sh to add one)", name)
	}
	seen := map[int]bool{}
	var out []int
	for i, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		nr, err := strconv.Atoi(fields[0])
		if err != nil || nr < 0 || nr > seccompProfileMaxNr {
			return nil, fmt.Errorf("seccomp profile %q line %d: bad syscall number %q", name, i+1, fields[0])
		}
		if seen[nr] {
			return nil, fmt.Errorf("seccomp profile %q line %d: duplicate syscall %d", name, i+1, nr)
		}
		seen[nr] = true
		out = append(out, nr)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("seccomp profile %q: no syscalls", name)
	}
	if len(out) > SeccompAllowMax {
		return nil, fmt.Errorf("seccomp profile %q: %d syscalls exceeds cap %d", name, len(out), SeccompAllowMax)
	}
	sort.Ints(out)
	return out, nil
}

// SeccompProfileNames lists the embedded profiles (for CLI completion and
// the run report Note).
func SeccompProfileNames() []string {
	entries, err := seccompProfiles.ReadDir("profiles")
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		n := e.Name()
		if strings.HasSuffix(n, ".allow") {
			names = append(names, strings.TrimSuffix(n, ".allow"))
		}
	}
	sort.Strings(names)
	return names
}

// seccompAllowLines renders the MINICT_SPEC `seccomp_allow` chunks (payload
// path, D59). Chunks stay well under SPEC_MAX_LINE (1024): ~42 numbers per
// line at ~21 chars/number worst case, so 40 keeps a wide margin.
func seccompAllowLines(nrs []int) []string {
	const chunk = 40
	var lines []string
	for start := 0; start < len(nrs); start += chunk {
		end := start + chunk
		if end > len(nrs) {
			end = len(nrs)
		}
		parts := make([]string, 0, end-start)
		for _, nr := range nrs[start:end] {
			parts = append(parts, strconv.Itoa(nr))
		}
		lines = append(lines, "seccomp_allow "+strings.Join(parts, ","))
	}
	return lines
}

// seccompEnvLines renders the exec-path MINICT_SECCOMP_ALLOW= chunks (same
// numbers, env-file format, D59).
func seccompEnvLines(nrs []int) []string {
	const chunk = 40
	var lines []string
	for start := 0; start < len(nrs); start += chunk {
		end := start + chunk
		if end > len(nrs) {
			end = len(nrs)
		}
		parts := make([]string, 0, end-start)
		for _, nr := range nrs[start:end] {
			parts = append(parts, strconv.Itoa(nr))
		}
		lines = append(lines, "MINICT_SECCOMP_ALLOW="+strings.Join(parts, ","))
	}
	return lines
}

// validSeccompName — embedded file component charset (lowercase only; the
// profiles we ship are busybox/alpine/alpine-nginx/debian).
func validSeccompName(name string) bool {
	if name == "" || len(name) > 64 {
		return false
	}
	for i, c := range name {
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9':
		case c == '-' || c == '.' || c == '_':
			if i == 0 {
				return false
			}
		default:
			return false
		}
	}
	return true
}
