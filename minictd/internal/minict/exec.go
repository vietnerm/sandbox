package minict

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/vietnerm/sandbox/minictd/contract"
)

// ErrNotRunning is returned when exec targets an instance whose payload is
// not alive (mapped to HTTP 409 by the API layer; docker errors exec on
// stopped containers with "is not running").
var ErrNotRunning = errors.New("instance not running")

// execStart spawns one exec against a RUNNING instance and returns pipes +
// a wait closure. Two modes:
//
//	setns (preferred): minict-exec joins the instance's namespaces via
//	                   /proc/<pidns-1-child>/ns/* — user FIRST, then mount,
//	                   pid, uts, ipc, net (ancestor-userns caps; the
//	                   RootlessKit/podman exec model). cmd runs INSIDE the
//	                   live instance: same netns (preview ports reachable),
//	                   same pidns view.
//	newns (fallback):   the PoC posture (D5) — a fresh namespace set on the
//	                   same rootfs. The exec is isolated from the running
//	                   payload; env reaches the payload through a
//	                   MINICT_SPEC file with env lines only; WorkDir is
//	                   applied with a tiny POSIX shell wrapper.
type execProc struct {
	mode   string
	stdout io.ReadCloser
	stderr io.ReadCloser
	wait   func() (contract.ExecV2Result, error)
}

func randHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// sanitizeEnv validates KEY=VALUE entries for the exec channels (both the
// minict-exec env-file and the MINICT_SPEC env lines are line-oriented).
func sanitizeEnv(entries []string) ([]string, error) {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		k, _, ok := strings.Cut(e, "=")
		if !ok || k == "" {
			return nil, fmt.Errorf("exec env entry %q is not KEY=VALUE", e)
		}
		if !envKeyRe.MatchString(k) {
			return nil, fmt.Errorf("exec env key %q must match [A-Za-z_][A-Za-z0-9_]*", k)
		}
		if strings.ContainsAny(e, "\n\r") || len(e) > 1024 {
			return nil, fmt.Errorf("exec env entry %q exceeds line limits", k)
		}
		out = append(out, e)
	}
	return out, nil
}

// firstChild returns the lowest direct child pid of root (the pidns-1 child
// for a healthy instance). exec joins namespaces through it so the pid
// namespace join lands INSIDE the instance, not the daemon's.
func firstChild(root int) int {
	if root <= 0 {
		return 0
	}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return 0
	}
	best := 0
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
		if err != nil || ppid != root {
			continue
		}
		if best == 0 || pid < best {
			best = pid
		}
	}
	return best
}

// execStart is the shared spawn path for Exec/ExecV2/ExecV2Stream.
func (s *Store) execStart(ctx context.Context, name string, req contract.ExecV2Request) (*execProc, error) {
	if len(req.Cmd) == 0 {
		return nil, fmt.Errorf("exec: empty command")
	}
	st, err := s.loadState(name)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, contract.ErrNotFound
		}
		return nil, err
	}
	st = s.alive(st)
	if st.State != StateRunning {
		return nil, fmt.Errorf("%w: instance %s (state %s)", ErrNotRunning, name, st.State)
	}

	mode := ExecNewns
	target := st.PID
	if s.MinictExecBin != "" {
		if _, err := os.Stat(s.MinictExecBin); err == nil {
			if kid := firstChild(st.PID); kid > 0 {
				mode = ExecSetns
				target = kid
			}
		}
	}

	env, err := sanitizeEnv(req.Env)
	if err != nil {
		return nil, err
	}

	var (
		cmd     *exec.Cmd
		cleanup = func() {}
	)
	execDir := filepath.Join(s.dir(name), "exec-env")
	_ = os.MkdirAll(execDir, 0700)

	if mode == ExecSetns {
		id := randHex(8)
		envf := filepath.Join(execDir, id+".env")
		// S5 (issue #121, D54): the payload path applies spec.Ulimits
		// via MINICT_SPEC `ulimit` lines BEFORE capdrop; exec'd
		// processes joined AFTER capdrop and missed that step. The
		// daemon appends MINICT_RLIMIT_<NAME>=soft[:hard] directives
		// to the env-file — minict-exec pulls them into a separate
		// array (NOT exported to the cmd env, R2 parity) and applies
		// them in the child before capdrop (D23 parity). PidsLimit
		// is NOT mapped here: under cgroups v2 the exec'd process
		// lives inside the instance's cgroup (pids.max already
		// bounds it); without cgroups the per-uid nproc approximation
		// carries the S2 shared-limit hazard and is left to the
		// payload path only.
		rlimitLines, err := s.execRlimitLines(name)
		if err != nil {
			return nil, err
		}
		seccompLines, err := s.execSeccompLines(name)
		if err != nil {
			return nil, err
		}
		fileLines := make([]string, 0, len(env)+len(rlimitLines)+len(seccompLines))
		fileLines = append(fileLines, env...)
		fileLines = append(fileLines, rlimitLines...)
		fileLines = append(fileLines, seccompLines...)
		if err := os.WriteFile(envf, []byte(strings.Join(fileLines, "\n")+"\n"), 0600); err != nil {
			return nil, err
		}
		cleanup = func() { _ = os.Remove(envf) }
		args := []string{
			s.MinictExecBin,
			"--pid", fmt.Sprintf("%d", target),
			"--cwd", cwdOrDefault(req.WorkDir),
			"--env-file", envf,
			"--",
		}
		args = append(args, req.Cmd...)
		cmd = exec.CommandContext(ctx, args[0], args[1:]...)
		cmd.Env = childEnv()
	} else {
		// newns fallback: env via a MINICT_SPEC file with env lines only.
		id := randHex(8)
		specf := filepath.Join(execDir, id+".spec")
		lines := make([]string, 0, len(env))
		for _, e := range env {
			lines = append(lines, "env "+e)
		}
		if err := os.WriteFile(specf, []byte(strings.Join(lines, "\n")+"\n"), 0600); err != nil {
			return nil, err
		}
		cleanup = func() { _ = os.Remove(specf) }

		rootfs := filepath.Join(s.dir(name), "rootfs")
		inner := req.Cmd
		if req.WorkDir != "" && filepath.IsAbs(req.WorkDir) && !strings.Contains(req.WorkDir, "\n") {
			// POSIX-safe chdir wrapper: cd "$1" && shift && exec "$@"
			inner = append([]string{"/bin/sh", "-c", `cd "$1" && shift && exec "$@"`,
				"minict-exec-cwd", req.WorkDir}, req.Cmd...)
		}
		args := append([]string{s.MinictBin, rootfs}, inner...)
		cmd = exec.CommandContext(ctx, args[0], args[1:]...)
		cmd.Env = append(childEnv(), "MINICT_SPEC="+specf)
	}

	// Process group + watchdog posture (shared by both modes):
	//   - own group so the daemon can TERM the whole exec tree at timeout
	//   - ctx cancellation TERMs the group; WaitDelay escalates to SIGKILL
	//     after 2s (mirrors docker's `timeout --kill-after=2`)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
	}
	cmd.WaitDelay = 2 * time.Second

	var timedOut atomic.Bool
	if req.TimeoutS > 0 {
		t := time.AfterFunc(time.Duration(req.TimeoutS)*time.Second, func() {
			timedOut.Store(true)
			if cmd.Process != nil {
				_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
				time.AfterFunc(2*time.Second, func() {
					_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
				})
			}
		})
		ol := cleanup
		cleanup = func() { t.Stop(); ol() }
	}

	// stdin: minict-exec inherits fd 0 untouched; the newns fallback
	// execs the payload with the daemon-provided stdin as well.
	cmd.Stdin = bytes.NewReader(orEmpty(req.Stdin))

	// Pipes MUST be created before Start (os/exec contract).
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cleanup()
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		cleanup()
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		cleanup()
		return nil, fmt.Errorf("start exec (%s mode): %w", mode, err)
	}

	started := time.Now()
	wait := func() (contract.ExecV2Result, error) {
		defer cleanup()
		waitErr := cmd.Wait()
		res := contract.ExecV2Result{DurationMs: time.Since(started).Milliseconds()}
		if timedOut.Load() {
			res.TimedOut = true
			res.ExitCode = contract.ExecTimeoutExit
			return res, nil
		}
		if waitErr == nil {
			return res, nil
		}
		var ee *exec.ExitError
		if errors.As(waitErr, &ee) {
			res.ExitCode = ee.ExitCode()
			return res, nil
		}
		return res, waitErr
	}
	return &execProc{mode: mode, stdout: stdout, stderr: stderr, wait: wait}, nil
}

func cwdOrDefault(w string) string {
	if w == "" {
		return "/"
	}
	return w
}

// execRlimitLines builds the MINICT_RLIMIT_<NAME>=soft[:hard] directives for
// the setns exec path (S5, issue #121, D54). Only nofile/nproc are emitted —
// parity with the payload's MINICT_SPEC parser (store.go buildSpecMinict).
// spec.Ulimits was validated at Run time; unknown names are silently skipped
// here (they were already recorded as a Note on Run). A missing spec.json
// (instance created before spec.json was mandatory, or corrupted) yields no
// rlimit lines — exec then behaves exactly as before (host default rlimit),
// no regression on upgrade.
func (s *Store) execRlimitLines(name string) ([]string, error) {
	spec, err := s.loadSpec(name)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []string
	for _, u := range spec.Ulimits {
		resName, val, ok := strings.Cut(u, "=")
		if !ok {
			continue
		}
		switch resName {
		case "nofile", "nproc":
			out = append(out, "MINICT_RLIMIT_"+strings.ToUpper(resName)+"="+val)
		}
	}
	return out, nil
}

// execSeccompLines builds the MINICT_SECCOMP_ALLOW= directives for the
// setns exec path (D59, issue #139) — parity with the payload's
// MINICT_SPEC seccomp_allow lines (store.go renderSpec). The profile rides
// spec.json, so template:<name> carries it automatically (issue #139).
// A missing spec.json yields no lines — exec then behaves exactly as
// before (host blocklist only). A profile that was valid at Run time but
// fails to load here means binaries drifted — fail closed.
func (s *Store) execSeccompLines(name string) ([]string, error) {
	spec, err := s.loadSpec(name)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if spec.SeccompProfile == "" {
		return nil, nil
	}
	nrs, err := LoadSeccompProfile(spec.SeccompProfile)
	if err != nil {
		return nil, err
	}
	return seccompEnvLines(nrs), nil
}

func orEmpty(in []byte) []byte {
	if in == nil {
		return nil
	}
	return in
}

// PrepareTerminal creates the exec env-file for a PTY terminal session
// (GĐ3 mục 2, issue #187) and returns the target pid + env-file path.
// The caller (server terminalManager) starts `minict-exec --pty --env-file
// <path> --pid <pid> -- /bin/sh` with the returned values. Same env-file
// as ExecV2 setns mode (rlimits + seccomp + spec env).
func (s *Store) PrepareTerminal(name string) (pid int, envFile string, err error) {
	st, err := s.loadState(name)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, "", contract.ErrNotFound
		}
		return 0, "", err
	}
	st = s.alive(st)
	if st.State != StateRunning {
		return 0, "", fmt.Errorf("%w: instance %s (state %s)", ErrNotRunning, name, st.State)
	}

	target := st.PID
	if s.MinictExecBin != "" {
		if _, err := os.Stat(s.MinictExecBin); err == nil {
			if kid := firstChild(st.PID); kid > 0 {
				target = kid
			}
		}
	}

	spec, err := s.loadSpec(name)
	if err != nil {
		return 0, "", fmt.Errorf("terminal %s: missing spec.json: %w", name, err)
	}
	env, err := sanitizeEnv(spec.Env)
	if err != nil {
		return 0, "", err
	}

	execDir := filepath.Join(s.dir(name), "exec-env")
	_ = os.MkdirAll(execDir, 0o700)
	id := randHex(8)
	envf := filepath.Join(execDir, id+".env")

	rlimitLines, err := s.execRlimitLines(name)
	if err != nil {
		return 0, "", err
	}
	seccompLines, err := s.execSeccompLines(name)
	if err != nil {
		return 0, "", err
	}
	fileLines := make([]string, 0, len(env)+len(rlimitLines)+len(seccompLines))
	fileLines = append(fileLines, env...)
	fileLines = append(fileLines, rlimitLines...)
	fileLines = append(fileLines, seccompLines...)
	if err := os.WriteFile(envf, []byte(strings.Join(fileLines, "\n")+"\n"), 0o600); err != nil {
		return 0, "", err
	}
	return target, envf, nil
}

// Exec is the legacy contract op: non-interactive, buffered, no timeout.
func (s *Store) Exec(ctx context.Context, name string, cmd []string) (contract.ExecResult, error) {
	res, err := s.ExecV2(ctx, name, contract.ExecV2Request{Cmd: cmd})
	if err != nil {
		return contract.ExecResult{}, err
	}
	return contract.ExecResult{Stdout: res.Stdout, Stderr: res.Stderr, ExitCode: res.ExitCode}, nil
}

// ExecV2 is the buffered v2 contract op (timeout, env, workdir, stdin,
// bounded output with head+tail preservation).
func (s *Store) ExecV2(ctx context.Context, name string, req contract.ExecV2Request) (contract.ExecV2Result, error) {
	proc, err := s.execStart(ctx, name, req)
	if err != nil {
		return contract.ExecV2Result{}, err
	}

	bufOut, bufErr := &bytes.Buffer{}, &bytes.Buffer{}
	var truncOut, truncErr bool
	var wOut, wErr io.Writer = bufOut, bufErr
	if req.MaxOutputBytes > 0 {
		wOut = newHeadTailWriter(bufOut, req.MaxOutputBytes, &truncOut)
		wErr = newHeadTailWriter(bufErr, req.MaxOutputBytes, &truncErr)
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); _, _ = io.Copy(wOut, proc.stdout) }()
	go func() { defer wg.Done(); _, _ = io.Copy(wErr, proc.stderr) }()

	res, waitErr := proc.wait()
	wg.Wait()
	if req.MaxOutputBytes > 0 {
		wOut.(*headTailWriter).flushTail()
		wErr.(*headTailWriter).flushTail()
	}
	res.Stdout = bufOut.String()
	res.Stderr = bufErr.String()
	res.Truncated = truncOut || truncErr
	return res, waitErr
}

// ExecV2Stream is the streaming contract op: raw pipes out, exit status
// collected by wait(). MaxOutputBytes is ignored — streaming IS the
// unbounded-output escape hatch (docker parity).
func (s *Store) ExecV2Stream(ctx context.Context, name string, req contract.ExecV2Request) (
	stdout, stderr io.ReadCloser, wait func() (contract.ExecV2Result, error), err error,
) {
	proc, err := s.execStart(ctx, name, req)
	if err != nil {
		return nil, nil, nil, err
	}
	return proc.stdout, proc.stderr, proc.wait, nil
}

// --- headTailWriter: verbatim semantics from sandboxd internal/docker ----

// headTailWriter keeps at most `max` bytes: the FIRST max bytes of the
// stream (the head) and, once the limit is exceeded, the trailing max/4
// bytes (the tail, flushed after a marker when the process exits). RAM
// per stream is bounded by max + max/4 regardless of output size.
type headTailWriter struct {
	dst       *bytes.Buffer
	max       int64
	headLen   int64
	written   int64
	truncated *bool
	tail      []byte
}

func newHeadTailWriter(dst *bytes.Buffer, max int64, truncated *bool) *headTailWriter {
	return &headTailWriter{dst: dst, max: max, truncated: truncated}
}

func (w *headTailWriter) Write(p []byte) (int, error) {
	n := len(p)
	w.written += int64(n)
	if room := w.max - w.headLen; room > 0 {
		take := int64(len(p))
		if take > room {
			take = room
		}
		w.dst.Write(p[:take])
		w.headLen += take
		p = p[take:]
	}
	if len(p) > 0 {
		*w.truncated = true
		keep := w.max / 4
		if keep <= 0 {
			keep = 1
		}
		w.tail = append(w.tail, p...)
		if int64(len(w.tail)) > keep {
			w.tail = w.tail[int64(len(w.tail))-keep:]
		}
	}
	return n, nil
}

// flushTail appends the kept tail after a marker. Call exactly once after
// the process exits.
func (w *headTailWriter) flushTail() {
	if !*w.truncated {
		return
	}
	w.dst.WriteString("\n-- output truncated --\n")
	w.dst.Write(w.tail)
	w.tail = nil
}
