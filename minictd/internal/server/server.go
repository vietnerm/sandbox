// Package server exposes a minict.Store over the minictd HTTP API. The API
// is the transport mirror of the contract.Runtime interface — one endpoint
// per op — so contract.HTTPClient satisfies the runtime interface by
// calling these endpoints. Auth: static bearer token when configured.
package server

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
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
}

func New(store *minict.Store, token string, log *slog.Logger) *Server {
	return &Server{store: store, token: token, log: log}
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
	mux.HandleFunc("POST /v1/sandboxes/{name}/remove", s.handleRemove)
	mux.HandleFunc("POST /v1/sandboxes/{name}/start", s.handleStart)
	mux.HandleFunc("POST /v1/sandboxes/{name}/stop", s.handleStop)
	mux.HandleFunc("POST /v1/sandboxes/{name}/exec", s.handleExec)
	mux.HandleFunc("POST /v1/sandboxes/{name}/execv2", s.handleExecV2)
	mux.HandleFunc("POST /v1/sandboxes/{name}/execstream", s.handleExecStream)
	mux.HandleFunc("GET /v1/version", s.handleVersion)
	mux.HandleFunc("GET /v1/info", s.handleInfo)
	mux.HandleFunc("GET /v1/subnets", s.handleSubnets)
	mux.HandleFunc("GET /healthz", s.handleHealthz)

	return s.auth(s.logRequests(mux))
}

func (s *Server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.token != "" {
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

func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		s.log.Info("http", "method", r.Method, "path", r.URL.Path,
			"dur_ms", time.Since(start).Milliseconds())
	})
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
	frames := make(chan contract.StreamFrame, 256)
	done := make(chan struct{})

	ctx := r.Context()
	// R6 (2026-11-18): each frame gets its OWN copy of the bytes — the
	// read buffer is reused by the next Read while the frame may still sit
	// in the channel (aliasing corrupted interleaved output) — and every
	// send also watches ctx so a vanished client cannot strand the pumps.
	pump := func(rc interface{ Read([]byte) (int, error) }, t string) {
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

	completed := 0
	for completed < 2 {
		select {
		case <-ctx.Done():
			// client went away; the runtime process dies via ctx. Still
			// reap it (wait() also removes the exec env file) so a
			// disconnect leaves no zombie behind.
			go func() { _, _ = wait() }()
			return
		case f := <-frames:
			if err := enc.Encode(f); err != nil {
				return
			}
			if canFlush {
				flusher.Flush()
			}
		case <-done:
			completed++
		}
	}
	res, werr := wait()
	exit := contract.StreamFrame{T: "exit", ExitCode: res.ExitCode,
		TimedOut: res.TimedOut, Truncated: res.Truncated, DurationMs: res.DurationMs}
	if werr != nil {
		s.log.Warn("execstream wait", "err", werr.Error())
	}
	_ = enc.Encode(exit)
	if canFlush {
		flusher.Flush()
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
