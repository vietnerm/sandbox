package server

// Unit tests for the #206 D77 PTY readiness handshake + stderr cap:
// waitPtyReady must succeed on the 'R' byte, fail on child death (EOF)
// and fail on deadline — never block forever; ptyStderrBuffer must cap.

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"
)

func TestWaitPtyReadyOK(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if _, err := w.Write([]byte{'R'}); err != nil {
		t.Fatal(err)
	}
	w.Close()
	ready, timedOut, rerr := waitPtyReady(r, time.Second)
	if !ready || timedOut || rerr != nil {
		t.Fatalf("want ready, got ready=%v timeout=%v err=%v", ready, timedOut, rerr)
	}
}

func TestWaitPtyReadyDeadChild(t *testing.T) {
	// Write end closed immediately → EOF → "exited before ready"
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	w.Close()
	defer r.Close()
	ready, timedOut, rerr := waitPtyReady(r, time.Second)
	if ready || timedOut || rerr == nil {
		t.Fatalf("want failure (no timeout), got ready=%v timeout=%v err=%v", ready, timedOut, rerr)
	}
	if !strings.Contains(rerr.Error(), "exited before reporting ready") {
		t.Fatalf("wrong error: %v", rerr)
	}
}

func TestWaitPtyReadyTimeout(t *testing.T) {
	// No byte ever → deadline fires (100 ms — the deadline IS the contract)
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	defer r.Close()
	start := time.Now()
	ready, timedOut, rerr := waitPtyReady(r, 100*time.Millisecond)
	if ready || !timedOut || rerr == nil {
		t.Fatalf("want timeout, got ready=%v timeout=%v err=%v", ready, timedOut, rerr)
	}
	if el := time.Since(start); el > 2*time.Second {
		t.Fatalf("timeout waited too long: %v", el)
	}
}

func TestPtyStderrBufferCap(t *testing.T) {
	var b ptyStderrBuffer
	head := "openpty-fail-marker"
	b.Write([]byte(head))
	mid := bytes.Repeat([]byte("x"), 10_000)
	b.Write(mid)
	tail := "setns-fail-marker"
	b.Write([]byte(tail))

	s := b.String()
	if !strings.HasPrefix(s, head) {
		t.Errorf("head lost: %s", s[:50])
	}
	if !strings.Contains(s, "truncated") {
		t.Errorf("truncation not reported")
	}
	if !strings.HasSuffix(s, tail) {
		t.Errorf("tail lost: ...%s", s[len(s)-50:])
	}
	if len(s) > ptyStderrCap+64 {
		t.Errorf("buffer not capped: %d bytes", len(s))
	}
}

func TestPtyStderrBufferSmall(t *testing.T) {
	var b ptyStderrBuffer
	b.Write([]byte("short marker"))
	if b.String() != "short marker" || b.over {
		t.Fatalf("small write must be kept verbatim: %q over=%v", b.String(), b.over)
	}
}
