// Package firecracker — vsock.go (GĐ4 mục 1b.d, issue #239).
//
// Guest agent vsock protocol: the microVM guest runs a minimal agent as PID 1
// that communicates with the host daemon over AF_VSOCK. The protocol is
// length-prefixed JSON (4-byte big-endian length + JSON body) — simple to
// parse in C (agent) and Go (daemon), no external dependency.
//
// The agent handles these operations:
//   - exec: run a command, return stdout/stderr/exit_code
//   - pty: open a PTY, stream I/O (for terminal)
//   - file_read: read a file by path
//   - file_write: write a file by path
//   - ping: health check (returns version)
//
// On the host side, the daemon connects to the guest agent via vsock
// (CID = guest, port = AGENT_PORT). In tests, vsock is simulated with
// a unix socket (no KVM needed).

package firecracker

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
)

// AgentPort is the vsock port the guest agent listens on.
const AgentPort = 52

// Op is the operation type in the protocol.
type Op string

const (
	OpExec      Op = "exec"
	OpPty       Op = "pty"
	OpFileRead  Op = "file_read"
	OpFileWrite Op = "file_write"
	OpPing      Op = "ping"
)

// Request is the host→guest message.
type Request struct {
	Op       Op       `json:"op"`
	Cmd      []string `json:"cmd,omitempty"`       // exec
	Path     string   `json:"path,omitempty"`      // file_read/file_write
	Data     []byte   `json:"data,omitempty"`      // file_write
	Env      []string `json:"env,omitempty"`       // exec
	Cwd      string   `json:"cwd,omitempty"`       // exec
	Rows     int      `json:"rows,omitempty"`      // pty
	Cols     int      `json:"cols,omitempty"`      // pty
	StreamID string   `json:"stream_id,omitempty"` // pty (for input/resize)
}

// Response is the guest→host message.
type Response struct {
	Op       Op     `json:"op"`
	StreamID string `json:"stream_id,omitempty"`
	Stdout   []byte `json:"stdout,omitempty"`
	Stderr   []byte `json:"stderr,omitempty"`
	ExitCode int    `json:"exit_code,omitempty"`
	Data     []byte `json:"data,omitempty"`
	Error    string `json:"error,omitempty"`
	Version  string `json:"version,omitempty"` // ping
}

// EncodeMessage writes a length-prefixed JSON message to w.
func EncodeMessage(w io.Writer, msg any) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("vsock encode: marshal: %w", err)
	}
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], uint32(len(body)))
	if _, err := w.Write(hdr[:]); err != nil {
		return fmt.Errorf("vsock encode: write header: %w", err)
	}
	if _, err := w.Write(body); err != nil {
		return fmt.Errorf("vsock encode: write body: %w", err)
	}
	return nil
}

// DecodeMessage reads a length-prefixed JSON message from r into msg.
func DecodeMessage(r io.Reader, msg any) error {
	var hdr [4]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return fmt.Errorf("vsock decode: read header: %w", err)
	}
	length := binary.BigEndian.Uint32(hdr[:])
	if length > 16<<20 { // 16 MiB max
		return fmt.Errorf("vsock decode: message too large: %d", length)
	}
	body := make([]byte, length)
	if _, err := io.ReadFull(r, body); err != nil {
		return fmt.Errorf("vsock decode: read body: %w", err)
	}
	if err := json.Unmarshal(body, msg); err != nil {
		return fmt.Errorf("vsock decode: unmarshal: %w", err)
	}
	return nil
}

// EncodeRequest is a typed wrapper for EncodeMessage.
func EncodeRequest(w io.Writer, req *Request) error {
	return EncodeMessage(w, req)
}

// DecodeRequest reads a Request from r.
func DecodeRequest(r io.Reader) (*Request, error) {
	var req Request
	if err := DecodeMessage(r, &req); err != nil {
		return nil, err
	}
	return &req, nil
}

// EncodeResponse is a typed wrapper for EncodeMessage.
func EncodeResponse(w io.Writer, resp *Response) error {
	return EncodeMessage(w, resp)
}

// DecodeResponse reads a Response from r.
func DecodeResponse(r io.Reader) (*Response, error) {
	var resp Response
	if err := DecodeMessage(r, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
