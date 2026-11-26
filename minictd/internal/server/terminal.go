// Package server — terminal.go (GĐ3 mục 2, issue #187).
//
// PTY terminal sessions over SSE + POST, stdlib only (no WebSocket).
// The daemon starts `minict-exec --pty` as a subprocess per session,
// keeps the stdin/stdout/control pipes, and exposes:
//
//	POST /v1/sandboxes/{name}/terminal          → start session, returns sid
//	GET  /v1/sandboxes/{name}/terminal/{sid}/stream  → SSE stdout (master→browser)
//	POST /v1/sandboxes/{name}/terminal/{sid}/input   → stdin (browser→master)
//	POST /v1/sandboxes/{name}/terminal/{sid}/resize  → TIOCSWINSZ (rows+cols)
//
// Cleanup: when the SSE client disconnects (stream read/flush error), the
// daemon sends SIGHUP to minict-exec → child receives SIGHUP via PTY master
// → default action terminates → reap. SIGKILL after 5s if still alive.
package server

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"syscall"
	"time"
)

// terminalSession is one live PTY session.
type terminalSession struct {
	id        string
	name      string
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	stdout    io.Reader
	controlW  io.WriteCloser // write end of the control pipe (fd 3 in child)
	startedAt time.Time
	mu        sync.Mutex // guards stdin + controlW writes
}

// terminalManager owns all live PTY sessions.
type terminalManager struct {
	mu       sync.Mutex
	sessions map[string]*terminalSession
	execBin  string // path to minict-exec
}

func newTerminalManager(execBin string) *terminalManager {
	return &terminalManager{
		sessions: map[string]*terminalSession{},
		execBin:  execBin,
	}
}

// Start launches a PTY session for the given instance (name + shim pid + env-file).
// Returns the session id.
func (tm *terminalManager) Start(name string, pid int, envFile, cwd string) (string, error) {
	// Create the control pipe: read end goes to the child as fd 3 (--control-fd 3),
	// write end stays with the daemon for resize.
	controlR, controlW, err := os.Pipe()
	if err != nil {
		return "", fmt.Errorf("terminal: control pipe: %w", err)
	}

	cmd := exec.Command(tm.execBin,
		"--pid", strconv.Itoa(pid),
		"--cwd", cwd,
		"--env-file", envFile,
		"--pty",
		"--control-fd", "3",
		"--", "/bin/sh",
	)
	cmd.ExtraFiles = []*os.File{controlR} // fd 3 in child

	stdin, err := cmd.StdinPipe()
	if err != nil {
		controlR.Close()
		controlW.Close()
		return "", fmt.Errorf("terminal: stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		controlR.Close()
		controlW.Close()
		return "", fmt.Errorf("terminal: stdout pipe: %w", err)
	}
	cmd.Stderr = nil // discard stderr

	if err := cmd.Start(); err != nil {
		stdin.Close()
		controlR.Close()
		controlW.Close()
		return "", fmt.Errorf("terminal: start minict-exec: %w", err)
	}
	controlR.Close() // parent does not read the control pipe

	b := make([]byte, 8)
	_, _ = rand.Read(b)
	sid := hex.EncodeToString(b)

	sess := &terminalSession{
		id:        sid,
		name:      name,
		cmd:       cmd,
		stdin:     stdin,
		stdout:    stdout,
		controlW:  controlW,
		startedAt: time.Now(),
	}

	tm.mu.Lock()
	tm.sessions[sid] = sess
	tm.mu.Unlock()

	// Reap in the background when the process exits.
	go func() {
		_ = cmd.Wait()
		tm.mu.Lock()
		delete(tm.sessions, sid)
		tm.mu.Unlock()
		_ = controlW.Close()
	}()

	return sid, nil
}

// Stream pumps stdout from the PTY to the HTTP response as SSE frames.
// Blocks until the PTY closes or the client disconnects (write error).
func (tm *terminalManager) Stream(sid string, w http.ResponseWriter) error {
	tm.mu.Lock()
	sess, ok := tm.sessions[sid]
	tm.mu.Unlock()
	if !ok {
		return fmt.Errorf("terminal: session %s not found", sid)
	}

	flusher, canFlush := w.(http.Flusher)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	if canFlush {
		flusher.Flush()
	}

	buf := make([]byte, 4096)
	for {
		n, err := sess.stdout.Read(buf)
		if n > 0 {
			// SSE data frame: JSON-encoded string for binary safety.
			frame, _ := json.Marshal(string(buf[:n]))
			if _, werr := fmt.Fprintf(w, "data: %s\n\n", frame); werr != nil {
				return nil // client disconnected
			}
			if canFlush {
				flusher.Flush()
			}
		}
		if err != nil {
			return nil // PTY closed
		}
	}
}

// Input writes data to the PTY stdin (browser → master).
func (tm *terminalManager) Input(sid string, data []byte) error {
	tm.mu.Lock()
	sess, ok := tm.sessions[sid]
	tm.mu.Unlock()
	if !ok {
		return fmt.Errorf("terminal: session %s not found", sid)
	}
	sess.mu.Lock()
	defer sess.mu.Unlock()
	_, err := sess.stdin.Write(data)
	return err
}

// Resize sends a TIOCSWINSZ to the PTY via the control pipe.
func (tm *terminalManager) Resize(sid string, rows, cols int) error {
	tm.mu.Lock()
	sess, ok := tm.sessions[sid]
	tm.mu.Unlock()
	if !ok {
		return fmt.Errorf("terminal: session %s not found", sid)
	}
	buf := make([]byte, 4)
	binary.LittleEndian.PutUint16(buf[0:2], uint16(rows))
	binary.LittleEndian.PutUint16(buf[2:4], uint16(cols))
	sess.mu.Lock()
	defer sess.mu.Unlock()
	_, err := sess.controlW.Write(buf)
	return err
}

// Stop kills the session (SIGHUP → SIGKILL after 5s).
func (tm *terminalManager) Stop(sid string) {
	tm.mu.Lock()
	sess, ok := tm.sessions[sid]
	tm.mu.Unlock()
	if !ok {
		return
	}
	if sess.cmd.Process != nil {
		_ = sess.cmd.Process.Signal(syscall.SIGHUP)
		go func() {
			time.Sleep(5 * time.Second)
			if sess.cmd.Process != nil {
				_ = sess.cmd.Process.Kill()
			}
		}()
	}
}
