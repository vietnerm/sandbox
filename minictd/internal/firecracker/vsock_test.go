// Package firecracker — vsock_test.go (GĐ4 mục 1b.d, issue #239).
//
// Tests for the vsock protocol encode/decode + request/response round-trip.
// Uses unix socket pairs to simulate vsock (no KVM needed).

package firecracker

import (
	"bytes"
	"encoding/binary"
	"net"
	"testing"
)

func TestEncodeDecodeRoundTrip(t *testing.T) {
	req := &Request{
		Op:  OpExec,
		Cmd: []string{"/bin/sh", "-c", "echo hello"},
		Env: []string{"PATH=/bin"},
		Cwd: "/tmp",
	}
	var buf bytes.Buffer
	if err := EncodeRequest(&buf, req); err != nil {
		t.Fatalf("EncodeRequest: %v", err)
	}
	got, err := DecodeRequest(&buf)
	if err != nil {
		t.Fatalf("DecodeRequest: %v", err)
	}
	if got.Op != OpExec || got.Cmd[1] != "-c" || got.Cwd != "/tmp" {
		t.Errorf("round-trip mismatch: %+v", got)
	}
}

func TestEncodeDecodeResponse(t *testing.T) {
	resp := &Response{
		Op:       OpExec,
		Stdout:   []byte("hello\n"),
		ExitCode: 0,
	}
	var buf bytes.Buffer
	if err := EncodeResponse(&buf, resp); err != nil {
		t.Fatalf("EncodeResponse: %v", err)
	}
	got, err := DecodeResponse(&buf)
	if err != nil {
		t.Fatalf("DecodeResponse: %v", err)
	}
	if string(got.Stdout) != "hello\n" || got.ExitCode != 0 {
		t.Errorf("response mismatch: %+v", got)
	}
}

func TestProtocolOverUnixSocket(t *testing.T) {
	// Simulate vsock via a unix socket pair.
	ln, err := net.Listen("unix", "")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	// Server (guest agent simulation)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		req, err := DecodeRequest(conn)
		if err != nil {
			return
		}
		resp := &Response{Op: req.Op}
		switch req.Op {
		case OpPing:
			resp.Version = "1.0.0"
		case OpExec:
			resp.Stdout = []byte("hello from guest\n")
			resp.ExitCode = 0
		default:
			resp.Error = "unsupported op"
		}
		EncodeResponse(conn, resp)
	}()

	// Client (daemon simulation)
	conn, err := net.Dial("unix", ln.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	// Test ping
	if err := EncodeRequest(conn, &Request{Op: OpPing}); err != nil {
		t.Fatalf("EncodeRequest ping: %v", err)
	}
	resp, err := DecodeResponse(conn)
	if err != nil {
		t.Fatalf("DecodeResponse ping: %v", err)
	}
	if resp.Version != "1.0.0" {
		t.Errorf("ping version: got %q, want 1.0.0", resp.Version)
	}
}

func TestDecodeMessageTooLarge(t *testing.T) {
	// Craft a message with a huge length header
	var buf bytes.Buffer
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], 17<<20) // 17 MiB > 16 MiB limit
	buf.Write(hdr[:])
	_, err := DecodeRequest(&buf)
	if err == nil {
		t.Fatal("expected error for oversized message")
	}
}

func TestFileReadWriteRoundTrip(t *testing.T) {
	req := &Request{
		Op:   OpFileWrite,
		Path: "/etc/hostname",
		Data: []byte("test-vm\n"),
	}
	var buf bytes.Buffer
	EncodeRequest(&buf, req)
	got, _ := DecodeRequest(&buf)
	if got.Op != OpFileWrite || got.Path != "/etc/hostname" || string(got.Data) != "test-vm\n" {
		t.Errorf("file write round-trip: %+v", got)
	}
}

func TestPtyRequestRoundTrip(t *testing.T) {
	req := &Request{
		Op:       OpPty,
		Rows:     24,
		Cols:     80,
		StreamID: "abc123",
	}
	var buf bytes.Buffer
	EncodeRequest(&buf, req)
	got, _ := DecodeRequest(&buf)
	if got.Rows != 24 || got.Cols != 80 || got.StreamID != "abc123" {
		t.Errorf("pty round-trip: %+v", got)
	}
}
