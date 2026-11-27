// Package server exposes a minict.Store over the minictd HTTP API. The API
// is the transport mirror of the contract.Runtime interface — one endpoint
// per op — so contract.HTTPClient satisfies the runtime interface by
// calling these endpoints. Auth: static bearer token when configured.
package server

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/vietnerm/sandbox/minictd/contract"
	"github.com/vietnerm/sandbox/minictd/internal/minict"
)

// ErrNotRunning mirrors minict.ErrNotRunning for status mapping.
var ErrNotRunning = minict.ErrNotRunning

// maxBodyBytes caps every request body (exec stdin included).
const maxBodyBytes = 16 << 20

// Server wires the store into the HTTP mux.
type Server struct {
	store *minict.Store
	token string
	log   *slog.Logger
	uiDir string           // GĐ3 static console root (SetUIDir); "" = UI off
	term  *terminalManager // GĐ3 mục 2 PTY sessions
	// #218 (D84): extra allowed Origin hosts for the WS PTY endpoint (for
	// proxied deployments where the listener Host is 127.0.0.1:port but the
	// browser Origin carries the public domain). Empty → strict same-host.
	wsOriginAllow []string
}

// SetWSOriginAllow registers extra Origin host[:port] values accepted by the
// WS handshake (beyond the strict Origin==Host check). Called by main.go
// for the --ws-origin flag.
func (s *Server) SetWSOriginAllow(allow []string) { s.wsOriginAllow = allow }

func New(store *minict.Store, token string, log *slog.Logger) *Server {
	return &Server{store: store, token: token, log: log, term: newTerminalManager(store.MinictExecBin, log)}
}

// Handler builds the route table (Go 1.22+ method+path patterns).
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /v1/run", s.handleRun)
	mux.HandleFunc("GET /v1/sandboxes", s.handleList)
	mux.HandleFunc("GET /v1/sandboxes/{name}", s.handleInspect)
	mux.HandleFunc("GET /v1/sandboxes/{name}/state", s.handleState)
	mux.HandleFunc("GET /v1/sandboxes/{name}/logs", s.handleLogs)
	mux.HandleFunc("GET /v1/sandboxes/{name}/stats", s.handleStats)
	mux.HandleFunc("POST /v1/sandboxes/{name}/save-template", s.handleSaveTemplate)
	mux.HandleFunc("POST /v1/sandboxes/{name}/snapshot", s.handleSnapshot)
	mux.HandleFunc("POST /v1/sandboxes/{name}/remove", s.handleRemove)
	mux.HandleFunc("POST /v1/sandboxes/{name}/start", s.handleStart)
	mux.HandleFunc("POST /v1/sandboxes/{name}/stop", s.handleStop)
	mux.HandleFunc("POST /v1/sandboxes/{name}/wake", s.handleWake)
	mux.HandleFunc("POST /v1/sandboxes/{name}/exec", s.handleExec)
	mux.HandleFunc("POST /v1/sandboxes/{name}/execv2", s.handleExecV2)
	mux.HandleFunc("POST /v1/sandboxes/{name}/execstream", s.handleExecStream)
	mux.HandleFunc("GET /v1/version", s.handleVersion)
	mux.HandleFunc("GET /v1/info", s.handleInfo)
	mux.HandleFunc("GET /v1/subnets", s.handleSubnets)
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("GET /metrics", s.handleMetrics) // GĐ2 mục 7c (issue #168): same token auth as /v1/*
	// GĐ3 mục 2 (issue #187): PTY terminal sessions
	mux.HandleFunc("POST /v1/sandboxes/{name}/terminal", s.handleTerminalStart)
	mux.HandleFunc("GET /v1/sandboxes/{name}/terminal/ws", s.handleTerminalWS) // #218/D84: must precede the {sid} pattern (literal wins)
	mux.HandleFunc("GET /v1/sandboxes/{name}/terminal/{sid}/stream", s.handleTerminalStream)
	mux.HandleFunc("POST /v1/sandboxes/{name}/terminal/{sid}/input", s.handleTerminalInput)
	mux.HandleFunc("POST /v1/sandboxes/{name}/terminal/{sid}/resize", s.handleTerminalResize)
	// GĐ3 mục 1: read-only artifact lists for the console (#128/#130 residual)
	mux.HandleFunc("GET /v1/templates", s.handleListTemplates)
	mux.HandleFunc("GET /v1/backups", s.handleListBackups)
	mux.HandleFunc("POST /v1/templates/{name}/remove", s.handleRemoveTemplate) // GĐ3 mục 1
	mux.HandleFunc("POST /v1/backups/{name}/remove", s.handleRemoveBackup)

	api := s.auth(s.requestLog(mux))
	if s.uiDir == "" {
		return api
	}
	return s.withUI(api)
}

// isWSHandshakePath reports whether r targets the WebSocket PTY endpoint
// (#218/D84). Browsers cannot attach an Authorization header to a WebSocket
// handshake, so the endpoint authenticates with the daemon token delivered
// as the FIRST text frame (constant-time compare, 5 s window, close 4401 on
// failure) — the standard header check would break every browser client and
// the frame check cannot be skipped by skipping the header check. The
// Origin CSWSH gate + mask/size frame rules run in ws.go regardless.
func isWSHandshakePath(p string) bool {
	return strings.HasSuffix(p, "/terminal/ws")
}

func (s *Server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.token != "" && !isWSHandshakePath(r.URL.Path) {
			// constant-time compare: a byte-wise != leaks the matching
			// prefix length through timing (R3, 2026-11-18)
			got := []byte(r.Header.Get("Authorization"))
			want := []byte("Bearer " + s.token)
			if subtle.ConstantTimeCompare(got, want) != 1 {
				writeErr(w, http.StatusUnauthorized, "unauthorized")
				return
			}
		}
		if r.Body != nil {
			// bounded bodies: RunSpec/exec requests are small; exec stdin
			// is the only large field (R5)
			r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
		}
		next.ServeHTTP(w, r)
	})
}

// statusRecorder wraps the ResponseWriter to capture the status code
// for the access log (GĐ2 mục 7a, issue #168).
type statusRecorder struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (r *statusRecorder) WriteHeader(code int) {
	if !r.wrote {
		r.status = code
		r.wrote = true
	}
	r.ResponseWriter.WriteHeader(code)
}

// Flush forwards to the wrapped writer. Without it the access-log
// middleware hid http.Flusher from streaming handlers (logs?follow=1,
// execstream): their `w.(http.Flusher)` check failed and output sat in
// the 4 KB response buffer until it filled (found by the GĐ3 console's
// live-log panel, 2026-11-26).
// Hijack forwards to the wrapped writer — the WS PTY endpoint (#218/D84)
// upgrades behind this middleware; without the forward, `w.(http.Hijacker)`
// failed and every handshake died with a 500.
func (r *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := r.ResponseWriter.(http.Hijacker); ok {
		return h.Hijack()
	}
	return nil, nil, errors.New("statusRecorder: underlying writer is not a Hijacker")
}

func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		if !r.wrote {
			r.status = http.StatusOK
			r.wrote = true
		}
		f.Flush()
	}
}

// Unwrap lets http.ResponseController reach the underlying writer.
func (r *statusRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

func (r *statusRecorder) Write(b []byte) (int, error) {
	if !r.wrote {
		r.status = http.StatusOK
		r.wrote = true
	}
	return r.ResponseWriter.Write(b)
}

// requestLog is the HTTP access log middleware (GĐ2 mục 7a, issue #168,
// D68). It generates/reads a request_id, captures the response status,
// and emits one structured log line per request: msg="http", request_id,
// method, path, instance (from path value when present), op (derived from
// path), status, result (ok if <400 else error), duration_ms. The token
// is NEVER logged (it lives only in the Authorization header which is not
// read here).
func (s *Server) requestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rid := requestID(r)
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		result := "ok"
		if rec.status >= 400 {
			result = "error"
		}
		inst := nameOf(r) // empty for paths without {name}
		s.log.Info("http",
			"request_id", rid,
			"method", r.Method,
			"path", r.URL.Path,
			"instance", inst,
			"op", opOf(r),
			"status", rec.status,
			"result", result,
			"duration_ms", time.Since(start).Milliseconds())
	})
}

// requestID returns the X-Request-Id header if present, otherwise
// generates a 16-char hex random ID. Used for log correlation (GĐ2 mục 7a).
func requestID(r *http.Request) string {
	if rid := r.Header.Get("X-Request-Id"); rid != "" {
		return rid
	}
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// opOf derives a short op name from the request path for the access log.
// Returns the last meaningful path segment (e.g. "start", "stop",
// "destroy", "metrics", "run").
func opOf(r *http.Request) string {
	p := strings.Trim(r.URL.Path, "/")
	parts := strings.Split(p, "/")
	if len(parts) == 0 {
		return r.URL.Path
	}
	last := parts[len(parts)-1]
	// Map remove -> destroy to match the lifecycle event name.
	if last == "remove" {
		return "destroy"
	}
	return last
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// mapErr maps runtime errors onto HTTP statuses.
func mapErr(w http.ResponseWriter, err error) {
	switch {
	case err == nil:
		return
	case errors.Is(err, minict.ErrInvalidName):
		writeErr(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, contract.ErrNotFound):
		writeErr(w, http.StatusNotFound, contract.ErrNotFound.Error())
	case errors.Is(err, minict.ErrNotRunning):
		writeErr(w, http.StatusConflict, err.Error())
	case errors.Is(err, minict.ErrPortInUse):
		writeErr(w, http.StatusConflict, err.Error())
	case errors.Is(err, minict.ErrInstanceRunning):
		writeErr(w, http.StatusConflict, err.Error())
	case errors.Is(err, context.Canceled):
		writeErr(w, 499, "client closed request")
	default:
		writeErr(w, http.StatusInternalServerError, err.Error())
	}
}

func nameOf(r *http.Request) string { return r.PathValue("name") }

func decodeBody(w http.ResponseWriter, r *http.Request, v any) bool {
	if r.Body == nil {
		return true
	}
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeErr(w, http.StatusBadRequest, "bad json body: "+err.Error())
		return false
	}
	return true
}

// --- op handlers (one per contract.Runtime method) ----------------------

func (s *Server) handleRun(w http.ResponseWriter, r *http.Request) {
	var spec contract.RunSpec
	if !decodeBody(w, r, &spec) {
		return
	}
	id, err := s.store.Run(r.Context(), spec)
	if err != nil {
		mapErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": id})
}

func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	names, err := s.store.ListByNamePrefix(r.Context(), r.URL.Query().Get("prefix"))
	if err != nil {
		mapErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string][]string{"names": names})
}

func (s *Server) handleInspect(w http.ResponseWriter, r *http.Request) {
	cj, err := s.store.Inspect(r.Context(), nameOf(r))
	if err != nil {
		mapErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, cj)
}

// handleState exposes minictd's own operational state (non-contract extra:
// notes, proxy pids, exec mode, rootfs source).
func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
	st, err := s.store.RawState(nameOf(r))
	if err != nil {
		mapErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) handleSnapshot(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Backup string `json:"backup"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	if err := s.store.Snapshot(r.Context(), nameOf(r), req.Backup); err != nil {
		mapErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"backup": req.Backup})
}

func (s *Server) handleListTemplates(w http.ResponseWriter, r *http.Request) {
	ts, err := s.store.ListTemplates()
	if err != nil {
		mapErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"templates": ts})
}

func (s *Server) handleListBackups(w http.ResponseWriter, r *http.Request) {
	bs, err := s.store.ListBackups()
	if err != nil {
		mapErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"backups": bs})
}

// handleRemoveTemplate deletes a saved template (GĐ3 mục 1, issue #179).
// Idempotent: 204 whether the template existed or not.
func (s *Server) handleRemoveTemplate(w http.ResponseWriter, r *http.Request) {
	if err := s.store.RemoveTemplate(nameOf(r)); err != nil {
		mapErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleRemoveBackup deletes a snapshot archive (GĐ3 mục 1, issue #179).
func (s *Server) handleRemoveBackup(w http.ResponseWriter, r *http.Request) {
	if err := s.store.RemoveBackup(nameOf(r)); err != nil {
		mapErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleSaveTemplate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Template string `json:"template"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	if err := s.store.SaveTemplate(r.Context(), nameOf(r), req.Template); err != nil {
		mapErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"template": req.Template})
}

func (s *Server) handleRemove(w http.ResponseWriter, r *http.Request) {
	if err := s.store.Remove(r.Context(), nameOf(r)); err != nil {
		mapErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleStart(w http.ResponseWriter, r *http.Request) {
	if err := s.store.Start(r.Context(), nameOf(r)); err != nil {
		mapErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleStop(w http.ResponseWriter, r *http.Request) {
	timeoutSec := 10
	if r.Body != nil {
		var buf bytes.Buffer
		if _, err := buf.ReadFrom(r.Body); err == nil && buf.Len() > 0 {
			var body struct {
				TimeoutSec int `json:"timeout_sec"`
			}
			if err := json.Unmarshal(buf.Bytes(), &body); err != nil {
				writeErr(w, http.StatusBadRequest, "bad json body: "+err.Error())
				return
			}
			if body.TimeoutSec > 0 {
				timeoutSec = body.TimeoutSec
			}
		}
	}
	if err := s.store.Stop(r.Context(), nameOf(r), timeoutSec); err != nil {
		mapErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleWake probes the preview ports of a running instance over host
// loopback (G3, issue #97 — non-contract extra; BridgeIP() is always ""
// for minictd so the probe dials 127.0.0.1:<port> per MERGE-GUIDE Step 5).
func (s *Server) handleWake(w http.ResponseWriter, r *http.Request) {
	ports, err := s.store.Wake(r.Context(), nameOf(r))
	if err != nil {
		mapErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ports": ports})
}

func (s *Server) handleExec(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Cmd []string `json:"cmd"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	res, err := s.store.Exec(r.Context(), nameOf(r), body.Cmd)
	if err != nil {
		mapErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleExecV2(w http.ResponseWriter, r *http.Request) {
	var req contract.ExecV2Request
	if !decodeBody(w, r, &req) {
		return
	}
	res, err := s.store.ExecV2(r.Context(), nameOf(r), req)
	if err != nil {
		mapErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// handleExecStream multiplexes stdout/stderr into NDJSON frames and ends
// with exactly one exit frame (see contract.StreamFrame).
func (s *Server) handleExecStream(w http.ResponseWriter, r *http.Request) {
	var req contract.ExecV2Request
	if !decodeBody(w, r, &req) {
		return
	}
	stdout, stderr, wait, err := s.store.ExecV2Stream(r.Context(), nameOf(r), req)
	if err != nil {
		mapErr(w, err)
		return
	}

	flusher, canFlush := w.(http.Flusher)
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	if canFlush {
		flusher.Flush()
	}

	enc := json.NewEncoder(w)
	emit := func(f contract.StreamFrame) error {
		if err := enc.Encode(f); err != nil {
			return err
		}
		if canFlush {
			flusher.Flush()
		}
		return nil
	}
	if !muxStreams(r.Context(), stdout, stderr, emit) {
		// client went away or the write failed; the runtime process dies
		// via ctx. Still reap it (wait() also removes the exec env file)
		// so a disconnect leaves no zombie behind.
		go func() { _, _ = wait() }()
		return
	}
	res, werr := wait()
	exit := contract.StreamFrame{T: "exit", ExitCode: res.ExitCode,
		TimedOut: res.TimedOut, Truncated: res.Truncated, DurationMs: res.DurationMs}
	if werr != nil {
		s.log.Warn("execstream wait", "err", werr.Error())
	}
	_ = emit(exit)
}

// muxStreams forwards stdout/stderr as frames until both reach EOF. It
// returns false if ctx ended or emit failed (the caller must not send an
// exit frame then).
//
// R6 (2026-11-18): each frame gets its OWN copy of the bytes — the read
// buffer is reused by the next Read while the frame may still sit in the
// channel — and every send also watches ctx so a vanished client cannot
// strand the pumps.
//
// Issue #50 (2026-11-19): a pump queues its last chunk and then signals
// done. select picks randomly among ready cases, so both done signals can
// win over chunks still buffered in frames; the loop used to exit there
// and drop them (MD13: stdout frame b”). Once both pumps are done nothing
// else is sent, so drain whatever is left before returning.
func muxStreams(ctx context.Context, stdout, stderr io.Reader,
	emit func(contract.StreamFrame) error) bool {
	frames := make(chan contract.StreamFrame, 256)
	done := make(chan struct{})
	pump := func(rc io.Reader, t string) {
		defer func() {
			select {
			case done <- struct{}{}:
			case <-ctx.Done():
			}
		}()
		buf := make([]byte, 32*1024)
		for {
			n, err := rc.Read(buf)
			if n > 0 {
				chunk := append([]byte(nil), buf[:n]...)
				select {
				case frames <- contract.StreamFrame{T: t, Data: chunk}:
				case <-ctx.Done():
					return
				}
			}
			if err != nil {
				return
			}
		}
	}
	go pump(stdout, "stdout")
	go pump(stderr, "stderr")

	for completed := 0; completed < 2; {
		select {
		case <-ctx.Done():
			return false
		case f := <-frames:
			if emit(f) != nil {
				return false
			}
		case <-done:
			completed++
		}
	}
	for {
		select {
		case f := <-frames:
			if emit(f) != nil {
				return false
			}
		default:
			return true
		}
	}
}

func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	tail, _ := strconv.Atoi(r.URL.Query().Get("tail"))
	follow := r.URL.Query().Get("follow") == "1" || r.URL.Query().Get("follow") == "true"
	rc, err := s.store.Logs(r.Context(), nameOf(r), tail, follow)
	if err != nil {
		mapErr(w, err)
		return
	}
	defer rc.Close()
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	flusher, canFlush := w.(http.Flusher)
	buf := make([]byte, 32*1024)
	for {
		n, err := rc.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return
			}
			if canFlush {
				flusher.Flush()
			}
		}
		if err != nil {
			return
		}
	}
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	st, err := s.store.Stats(r.Context(), nameOf(r))
	if err != nil {
		mapErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	v, err := s.store.ServerVersion(r.Context())
	if err != nil {
		mapErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"version": v})
}

func (s *Server) handleInfo(w http.ResponseWriter, r *http.Request) {
	v, err := s.store.Info(r.Context())
	if err != nil {
		mapErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"version": v})
}

func (s *Server) handleSubnets(w http.ResponseWriter, r *http.Request) {
	network := r.URL.Query().Get("network")
	subnets, gateways, err := s.store.NetworkSubnets(r.Context(), network)
	if err != nil {
		mapErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string][]string{"subnets": subnets, "gateways": gateways})
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"ok": "1"})
}

// handleMetrics serves the Prometheus text exposition format for
// minictd observability (GĐ2 mục 7c, issue #168, D68). Same token auth
// as /v1/* (enforced by the auth middleware wrapping the whole mux).
// Scrape-time: reads cgroup files per instance, no goroutine, no RAM
// history. Content-Type per Prometheus spec v0.0.4.
func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	body := s.store.RenderMetrics()
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

// --- GĐ3 mục 2: PTY terminal handlers (issue #187) ---

func (s *Server) handleTerminalStart(w http.ResponseWriter, r *http.Request) {
	name := nameOf(r)
	pid, envf, err := s.store.PrepareTerminal(name)
	if err != nil {
		mapErr(w, err)
		return
	}
	sid, err := s.term.Start(name, pid, envf, "/")
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"session_id": sid})
}

func (s *Server) handleTerminalStream(w http.ResponseWriter, r *http.Request) {
	sid := r.PathValue("sid")
	if err := s.term.Stream(sid, w); err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
	}
}

func (s *Server) handleTerminalInput(w http.ResponseWriter, r *http.Request) {
	sid := r.PathValue("sid")
	body, err := io.ReadAll(io.LimitReader(r.Body, 64<<10))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad body")
		return
	}
	if err := s.term.Input(sid, body); err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleTerminalResize(w http.ResponseWriter, r *http.Request) {
	sid := r.PathValue("sid")
	var req struct {
		Rows int `json:"rows"`
		Cols int `json:"cols"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	if req.Rows <= 0 || req.Cols <= 0 || req.Rows > 1000 || req.Cols > 1000 {
		writeErr(w, http.StatusBadRequest, "rows/cols out of range")
		return
	}
	if err := s.term.Resize(sid, req.Rows, req.Cols); err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleTerminalWS — #218 (D84): bidirectional WebSocket PTY transport.
// The handler only adapts the daemon (prepare → pid/env-file); the whole
// lifecycle (auth frame → ready → pumps → close) lives in terminal_ws.go.
func (s *Server) handleTerminalWS(w http.ResponseWriter, r *http.Request) {
	name := nameOf(r)
	prepare := func() (int, string, error) { return s.store.PrepareTerminal(name) }
	s.term.ServeWS(w, r, s.token, s.wsOriginAllow, prepare)
}
