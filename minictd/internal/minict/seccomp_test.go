package minict

import (
	"strings"
	"testing"
)

// Tests for the embedded seccomp allowlist profiles (GĐ2 mục 3a, issue #139,
// D59): loading, validation, and the MINICT_SPEC / env-file renderers both
// the payload child and the setns exec child consume.

func TestLoadSeccompProfileBusybox(t *testing.T) {
	nrs, err := LoadSeccompProfile("busybox")
	if err != nil {
		t.Fatalf("busybox: %v", err)
	}
	if len(nrs) == 0 || len(nrs) > SeccompAllowMax {
		t.Fatalf("busybox: got %d syscalls, want 1..%d", len(nrs), SeccompAllowMax)
	}
	// the filter needs execve(59) to start the payload at all
	has := false
	for _, nr := range nrs {
		if nr == 59 {
			has = true
		}
	}
	if !has {
		t.Fatal("busybox profile missing execve(59): payload could never exec")
	}
	// sorted + unique (the C allow_add dedupes too, but the Go render must
	// not ship duplicates — duplicate lines die fail-closed in LoadSeccomp)
	for i := 1; i < len(nrs); i++ {
		if nrs[i] <= nrs[i-1] {
			t.Fatalf("busybox not strictly sorted at %d: %d then %d", i, nrs[i-1], nrs[i])
		}
	}
}

func TestLoadSeccompProfileAllEmbedded(t *testing.T) {
	for _, name := range SeccompProfileNames() {
		nrs, err := LoadSeccompProfile(name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		for _, nr := range nrs {
			if nr < 0 || nr > 511 {
				t.Fatalf("%s: syscall %d out of range", name, nr)
			}
		}
	}
}

func TestLoadSeccompProfileUnknownFails(t *testing.T) {
	if _, err := LoadSeccompProfile("no-such-profile"); err == nil {
		t.Fatal("unknown profile must be an error (fail-closed), got nil")
	}
}

func TestLoadSeccompProfileBadNameFails(t *testing.T) {
	for _, bad := range []string{"", "BusyBox", "../busybox", "a/b"} {
		if _, err := LoadSeccompProfile(bad); err == nil {
			t.Fatalf("profile name %q must be an error", bad)
		}
	}
}

func TestSeccompAllowLinesChunking(t *testing.T) {
	nrs := make([]int, 0, 95)
	for i := 0; i < 95; i++ {
		nrs = append(nrs, 100+i)
	}
	lines := seccompAllowLines(nrs)
	if len(lines) != 3 {
		t.Fatalf("got %d chunks for 95 numbers, want 3 (40/40/15)", len(lines))
	}
	if !strings.HasPrefix(lines[0], "seccomp_allow 100,") {
		t.Fatalf("first chunk wrong: %q", lines[0][:40])
	}
	last := strings.TrimPrefix(lines[2], "seccomp_allow ")
	if got := strings.Split(last, ","); len(got) != 15 {
		t.Fatalf("last chunk has %d numbers, want 15", len(got))
	}
	for _, l := range lines {
		if len(l) > 1024 {
			t.Fatalf("chunk exceeds SPEC_MAX_LINE 1024: %d", len(l))
		}
		if strings.HasSuffix(l, ",") || strings.Contains(l, ",,") {
			t.Fatalf("malformed chunk: %q", l)
		}
	}
}

func TestSeccompEnvLinesFormat(t *testing.T) {
	lines := seccompEnvLines([]int{41, 42, 59})
	if len(lines) != 1 {
		t.Fatalf("got %d lines, want 1", len(lines))
	}
	if lines[0] != "MINICT_SECCOMP_ALLOW=41,42,59" {
		t.Fatalf("env line wrong: %q", lines[0])
	}
}

func TestSeccompProfileNamesShipped(t *testing.T) {
	want := map[string]bool{"busybox": false, "alpine": false,
		"alpine-nginx": false, "debian": false}
	for _, n := range SeccompProfileNames() {
		if _, ok := want[n]; ok {
			want[n] = true
		}
	}
	for n, found := range want {
		if !found {
			t.Fatalf("embedded profile %q missing (go:embed profiles/*.allow)", n)
		}
	}
}
