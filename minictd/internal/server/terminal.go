// Package server — terminal.go (GĐ3 mục 2, issue #187; #206 fix D77).
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
//
// #206 (D77) — no more fake success: Start captures minict-exec stderr into
// a capped buffer (item b, slog-logged, included in the returned error),
// waits for the child's one-byte 'R' readiness signal on a status pipe
// (item c — setns+chdir+openpty+fork all done) for up to 2 s, and only
// then registers the session and returns the sid. A child that dies before
// ready (the owner-reported bug: openpty ENOENT inside the instance — no
// /dev/ptmx, no devpts — fixed host-side in minict-exec) or misses the
// deadline is killed, reaped, and reported as a 5xx with the stderr tail,
// never as a 200.
package server

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/vietnerm/sandbox/minictd/internal/minict"
)

// ptyStderrBuffer keeps the first and last ptyStderrCap bytes of the child's
// stderr (#206 item b): early die() messages (openpty/setns reason) AND the
// final ones, capped so a chatty child cannot balloon daemon memory.
type ptyStderrBuffer struct {
	head []byte
	tail []byte
	over bool
	n    int // bytes consumed (for the io.Writer count)
}

const (
	ptyStderrCap  = 4096
	ptyStderrHalf = 2048
)

func (w *ptyStderrBuffer) Write(p []byte) (int, error) {
	total := len(p)
	if len(w.head) < ptyStderrHalf {
		n := ptyStderrHalf - len(w.head)
		if n > len(p) {
			n = len(p)
		}
		w.head = append(w.head, p[:n]...)
		p = p[n:]
	}
	if len(p) > 0 {
		w.over = true
		w.tail = append(w.tail, p...)
		if len(w.tail) > ptyStderrHalf {
			w.tail = w.tail[len(w.tail)-ptyStderrHalf:]
		}
	}
	w.n += total
	return total, nil
}

func (w *ptyStderrBuffer) String() string {
	if w.over {
		return string(w.head) + "…[truncated]…" + string(w.tail)
	}
	return string(w.head)
}

// ptyReadyTimeout bounds the wait for minict-exec's 'R' status byte (#206
// item c). Long enough for the setns chain + openpty + fork on a cold host;
// short enough that a dead-on-arrival PTY never hangs the API call.
const ptyReadyTimeout = 2 * time.Second

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
	log      *slog.Logger
}

func newTerminalManager(execBin string, log *slog.Logger) *terminalManager {
	return &terminalManager{
		sessions: map[string]*terminalSession{},
		execBin:  execBin,
		log:      log,
	}
}

// waitPtyReady blocks until the child signals readiness on statusR ('R'),
// the child dies (EOF), or the deadline passes. Extracted for unit testing
// (#206 item e): the outcome does not depend on which case fires.
func waitPtyReady(statusR io.Reader, timeout time.Duration) (ready bool, timedOut bool, err error) {
	ch := make(chan error, 1)
	go func() {
		b := make([]byte, 1)
		n, rerr := statusR.Read(b)
		if rerr != nil || n == 0 {
			ch <- fmt.Errorf("minict-exec exited before reporting ready")
			return
		}
		if b[0] != 'R' {
			ch <- fmt.Errorf("minict-exec sent unexpected status byte %q", b[0])
			return
		}
		ch <- nil
	}()
	select {
	case err := <-ch:
		return err == nil, false, err
	case <-time.After(timeout):
		return false, true, fmt.Errorf("timeout after %s waiting for PTY readiness", timeout)
	}
}

// Start launches a PTY session for the given instance (name + shim pid + env-file).
// Returns the session id ONLY after the child reports ready (#206 item c).
func (tm *terminalManager) Start(name string, pid int, envFile, cwd string) (string, error) {
	// Create the control pipe: read end goes to the child as fd 3 (--control-fd 3),
	// write end stays with the daemon for resize.
	controlR, controlW, err := os.Pipe()
	if err != nil {
		return "", fmt.Errorf("terminal: control pipe: %w", err)
	}
	// Status pipe (#206 item c): write end goes to the child as fd 4
	// (--status-fd 4); the daemon reads one 'R' byte when setup is done.
	statusR, statusW, err := os.Pipe()
	if err != nil {
		controlR.Close()
		controlW.Close()
		return "", fmt.Errorf("terminal: status pipe: %w", err)
	}

	cmd := exec.Command(tm.execBin,
		"--pid", strconv.Itoa(pid),
		"--cwd", cwd,
		"--env-file", envFile,
		"--pty",
		"--control-fd", "3",
		"--status-fd", "4",
		"--", "/bin/sh",
	)
	cmd.ExtraFiles = []*os.File{controlR, statusW} // fd 3, fd 4 in child

	stdin, err := cmd.StdinPipe()
	if err != nil {
		controlR.Close()
		controlW.Close()
		statusR.Close()
		statusW.Close()
		return "", fmt.Errorf("terminal: stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		controlR.Close()
		controlW.Close()
		statusR.Close()
		statusW.Close()
		return "", fmt.Errorf("terminal: stdout pipe: %w", err)
	}
	// #206 item b: capture stderr (was nil — the setns/openpty failure never
	// reached the daemon log). Same posture as the execv2 path: the daemon's
	// own environment is NOT inherited (R2).
	var stderrBuf ptyStderrBuffer
	cmd.Stderr = &stderrBuf
	cmd.Env = minict.ExecChildEnv()

	if err := cmd.Start(); err != nil {
		stdin.Close()
		controlR.Close()
		controlW.Close()
		statusR.Close()
		statusW.Close()
		return "", fmt.Errorf("terminal: start minict-exec: %w", err)
	}
	controlR.Close() // parent does not read the control pipe

	// Readiness handshake (#206 item c): 'R' → register; EOF/garbage → the
	// child died during setup (e.g. openpty/setns) → 5xx with the stderr
	// tail; deadline → kill + 5xx. Never a 200 for a dead PTY.
	_, timedOut, rerr := waitPtyReady(statusR, ptyReadyTimeout)
	statusW.Close() // parent write end no longer needed in any outcome
	if rerr != nil || timedOut {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		waitErr := cmd.Wait()
		statusR.Close()
		stdin.Close()
		tail := stderrBuf.String()
		if tm.log != nil {
			tm.log.Error("terminal: pty start failed",
				"instance", name, "timed_out", timedOut,
				"wait_err", fmt.Sprint(waitErr), "stderr_tail", tail)
		}
		return "", fmt.Errorf("terminal: pty failed to start (instance %s): %v — stderr: %s",
			name, rerr, tail)
	}
	statusR.Close() // handshake done — parent read end closed

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
