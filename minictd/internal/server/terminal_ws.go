// Package server — terminal_ws.go (#218, D84).
//
// GET /v1/sandboxes/{name}/terminal/ws — the bidirectional WebSocket PTY
// transport. One connection = one PTY session; the RFC 6455 plumbing lives
// in ws.go (stdlib only), the PTY lifecycle is the terminalManager's.
//
// Wire protocol (issue #218 item 2):
//
//	browser → server  BINARY  = raw PTY stdin bytes (no JSON wrapping)
//	browser → server  TEXT    = {"type":"resize","rows":R,"cols":C}
//	                            {"type":"close"}
//	                            {"type":"auth","token":"…"}   ← FIRST frame only
//	server → browser  BINARY  = raw PTY stdout bytes
//	server → browser  TEXT    = {"type":"ready"}
//	                            {"type":"exit","code":N}
//	                            {"type":"error","msg":"…"}
//
// Security (issue #218 item 3 — every line has a test):
//   - the auth frame must be the FIRST text frame within 5 s, constant-time
//     compared; wrong/late → close 4401 BEFORE any PTY is spawned (a failed
//     auth costs zero sessions, zero env-files);
//   - Origin must match the listener Host (or --ws-origin) — checked in
//     ws.go before the 101; non-browser clients pass the Origin gate but
//     still need the auth frame;
//   - unmasked client frames → 1002, payload > 64 KB → 1009 (ws.go);
//   - liveness: server pings every 30 s; > 60 s without ANY inbound frame
//     (a browser auto-pongs at protocol level — MDN) → close;
//   - backpressure: the output queue holds wsOutQueue frames; a client that
//     stops reading for > wsOutStall drops the session (SIGHUP + close) so
//     daemon RSS cannot grow without bound;
//   - close → Stop() (SIGHUP → reap) and the reaper removes the env-file.
package server

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

const (
	wsAuthTimeout = 5 * time.Second
	wsPingEvery   = 30 * time.Second
	wsPongTimeout = 60 * time.Second
	wsOutQueue    = 64 // frames; 64 × ≤64 KB ≤ 4 MB worst case per session
	wsOutStall    = 30 * time.Second
	wsReadBufIn   = 8192
)

// wsAuthFrame is the only accepted shape of the first client text frame.
type wsAuthFrame struct {
	Type  string `json:"type"`
	Token string `json:"token"`
}

// wsControl is the text-frame control vocabulary after auth.
type wsControl struct {
	Type string `json:"type"`
	Rows int    `json:"rows"`
	Cols int    `json:"cols"`
}

// wsSendJSON marshals and sends one text frame (best effort — the connection
// is torn down on the next write error anyway).
func wsSendJSON(c *wsConn, v map[string]any) {
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	_ = c.writeText(b)
}

// ServeWS runs the whole WebSocket PTY session lifecycle on the hijacked
// connection. prepare() is called AFTER a successful auth frame — a bad
// actor never reaches the instance/store layer.
func (tm *terminalManager) ServeWS(w http.ResponseWriter, r *http.Request, token string, originAllow []string, prepare func() (int, string, error)) {
	c, err := wsUpgrade(w, r, originAllow)
	if err != nil {
		return
	}
	// From here the connection is ours: every exit path must close it.
	authed := false
	defer func() {
		if !authed {
			c.writeClose(wsCloseAuth, "auth required")
		}
	}()

	// --- auth frame: FIRST text frame within 5 s, constant-time compare ---
	c.setReadDeadline(time.Now().Add(wsAuthTimeout))
	op, payload, rerr := c.wsReadMessage(nil)
	if rerr != nil {
		// a PROTOCOL violation or oversize frame is a 1002/1009, not an auth
		// failure (tests assert the exact code — mask/size rules are frame-level)
		switch {
		case errors.Is(rerr, errWSTooBig):
			c.writeClose(wsCloseTooBig, "too big")
		case errors.Is(rerr, errWSProtocol):
			c.writeClose(wsCloseProtocol, "protocol error")
		default:
			c.writeClose(wsCloseAuth, "auth timeout or protocol error")
		}
		return
	}
	if op != wsOpText {
		c.writeClose(wsCloseAuth, "auth must be a text frame")
		return
	}
	var af wsAuthFrame
	if jerr := json.Unmarshal(payload, &af); jerr != nil || af.Type != "auth" || af.Token == "" {
		c.writeClose(wsCloseAuth, "bad auth frame")
		return
	}
	if subtle.ConstantTimeCompare([]byte(af.Token), []byte(token)) != 1 {
		if tm.log != nil {
			tm.log.Warn("terminal ws: auth failed", "op", "terminal_ws", "instance", r.PathValue("name"))
		}
		c.writeClose(wsCloseAuth, "auth failed")
		return
	}
	authed = true
	c.setReadDeadline(time.Time{}) // clear the auth deadline — PTY lifetime now

	// --- spawn the PTY (only after auth) ---
	pid, envFile, perr := prepare()
	if perr != nil {
		wsSendJSON(c, map[string]any{"type": "error", "msg": perr.Error()})
		c.writeClose(wsCloseTarget, "instance not available")
		return
	}
	sid, serr := tm.Start(r.PathValue("name"), pid, envFile, "/")
	if serr != nil {
		wsSendJSON(c, map[string]any{"type": "error", "msg": serr.Error()})
		c.writeClose(wsCloseTarget, "pty start failed")
		return
	}
	wsSendJSON(c, map[string]any{"type": "ready"})

	tm.mu.Lock()
	sess := tm.sessions[sid]
	tm.mu.Unlock()
	if sess == nil {
		c.writeClose(wsCloseTarget, "session vanished")
		return
	}

	// --- cleanup: exactly once, whichever side ends first ---
	var once sync.Once
	var writerDone = make(chan struct{})
	stop := func(reason string) {
		once.Do(func() {
			tm.Stop(sid)
			if tm.log != nil {
				tm.log.Info("terminal ws: session closed", "op", "terminal_ws",
					"instance", sess.name, "sid", sid, "reason", reason)
			}
			c.writeClose(1000, "bye")
		})
	}

	// liveness: any inbound frame counts (a browser auto-pongs server pings)
	var lastRecv atomic.Int64
	lastRecv.Store(time.Now().UnixNano())

	// pump A: PTY stdout → output queue (bounded; stalled client = drop).
	out := make(chan []byte, wsOutQueue)
	go func() {
		defer close(out)
		buf := make([]byte, wsReadBufIn)
		for {
			n, rerr := sess.stdout.Read(buf)
			if n > 0 {
				frame := make([]byte, n)
				copy(frame, buf[:n])
				select {
				case out <- frame:
				case <-time.After(wsOutStall):
					stop("output stall (client stopped reading)")
					return
				}
			}
			if rerr != nil {
				return // PTY closed — exit code flows through sess.done
			}
		}
	}()

	// writer goroutine: drains out → binary frames, pings every 30 s, closes
	// after 60 s of silence, and surfaces the PTY exit code once.
	go func() {
		defer close(writerDone)
		ticker := time.NewTicker(wsPingEvery)
		defer ticker.Stop()
		for {
			select {
			case code := <-sess.done:
				wsSendJSON(c, map[string]any{"type": "exit", "code": code})
				stop("pty exited")
				return
			case frame, ok := <-out:
				if !ok {
					// stdout closed but exit code not delivered yet — the
					// reaper will send it; wait instead of spinning.
					select {
					case code := <-sess.done:
						wsSendJSON(c, map[string]any{"type": "exit", "code": code})
					case <-time.After(wsPongTimeout):
					}
					stop("pty closed")
					return
				}
				if werr := c.writeBinary(frame); werr != nil {
					stop("write failed")
					return
				}
			case <-ticker.C:
				if time.Since(time.Unix(0, lastRecv.Load())) > wsPongTimeout {
					stop("pong timeout (60s silence)")
					return
				}
				if perr := c.writeFrame(0x9, []byte("pty")); perr != nil {
					stop("ping write failed")
					return
				}
			}
		}
	}()

	// reader loop: browser → server. Runs on the handler goroutine.
	for {
		op, payload, rerr := c.wsReadMessage(func() {
			lastRecv.Store(time.Now().UnixNano())
		})
		if rerr != nil {
			reason := "read error"
			switch {
			case errors.Is(rerr, io.EOF):
				reason = "client closed"
			case errors.Is(rerr, errWSTooBig):
				c.writeClose(wsCloseTooBig, "too big")
				reason = "oversize frame"
			case errors.Is(rerr, errWSProtocol):
				c.writeClose(wsCloseProtocol, "protocol error")
				reason = "protocol violation"
			}
			stop(reason)
			<-writerDone
			return
		}
		lastRecv.Store(time.Now().UnixNano())
		switch op {
		case wsOpBinary:
			if len(payload) > 0 {
				if err := tm.Input(sid, payload); err != nil {
					stop("stdin write failed")
					<-writerDone
					return
				}
			}
		case wsOpText:
			var ctl wsControl
			if jerr := json.Unmarshal(payload, &ctl); jerr != nil {
				c.writeClose(wsCloseProtocol, "bad control frame")
				stop("bad control frame")
				<-writerDone
				return
			}
			switch ctl.Type {
			case "resize":
				if ctl.Rows > 0 && ctl.Cols > 0 && ctl.Rows <= 1000 && ctl.Cols <= 1000 {
					_ = tm.Resize(sid, ctl.Rows, ctl.Cols)
				}
			case "close":
				stop("client requested close")
				<-writerDone
				return
			default:
				c.writeClose(wsCloseProtocol, "unknown control type")
				stop("unknown control type")
				<-writerDone
				return
			}
		}
	}
}
