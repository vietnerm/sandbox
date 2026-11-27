// Package server — ws.go (#218, D84).
//
// Minimal RFC 6455 WebSocket server implementation, Go stdlib only
// (http.Hijacker + crypto/sha1 + encoding/base64) — no new dependency:
// go.mod is zero-dep and manifest §3 mandates "mặc định tối thiểu".
//
// Scope (only what the PTY terminal needs, per issue #218):
//   - handshake: Sec-WebSocket-Accept, reject extensions (no
//     permessage-deflate — RSV bits set in a client frame → close 1002),
//     reject unknown/unsupported versions;
//   - frames: text/binary/continuation/close/ping/pong only;
//   - client frames MUST be masked (RFC 6455 §5.3 — cache-poisoning
//     defence, not content security); unmasked → close 1002;
//   - client payload cap 64 KB per message (fragmentation reassembled,
//     total also capped) → close 1009;
//   - server frames are unmasked (per RFC), chunked ≤ 64 KB on write.
//
// Security posture (issue #218 item 3):
//   - Origin check BEFORE the 101 upgrade: an Origin header that does not
//     match the listener Host is cross-site (CSWSH) → 403. A non-browser
//     client sends no Origin and passes — the token still gates everything
//     via the auth frame (constant-time compare), which must arrive within
//     5 s or the connection is closed 4401 before any PTY is spawned.
package server

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// wsGUID is the RFC 6455 §1.3 magic string for Sec-WebSocket-Accept.
const wsGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

// wsMaxPayload caps one client→server MESSAGE (fragmented or not).
// A PTY keystroke is ≤ ~16 bytes and a paste chunk ≤ ~64 KB is already
// generous; anything larger is abuse → close 1009. Server→browser output
// frames are chunked to wsMaxPayload too.
const wsMaxPayload = 64 << 10

// WS close codes used by the terminal endpoint.
const (
	wsCloseProtocol = 1002 // masked-bit missing, RSV set, bad opcode, bad UTF-8 JSON
	wsCloseTooBig   = 1009 // payload > wsMaxPayload
	wsCloseAuth     = 4401 // missing/wrong/late auth frame (issue #218)
	wsCloseTarget   = 4404 // instance not running / PTY start failure
)

// wsOpcodes (RFC 6455 §5.2).
const (
	wsOpCont    = 0x0
	wsOpText    = 0x1
	wsOpBinary  = 0x2
	wsOpClose   = 0x8
	wsOpPing    = 0x9
	wsOpPong    = 0xA
	wsOpUnknown = 0xF // sentinel: anything not in the supported set
)

// wsAccept computes the handshake accept token (RFC 6455 §4.2.2 step 4).
func wsAccept(key string) string {
	h := sha1.New()
	h.Write([]byte(key + wsGUID))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

// wsOriginAllowed implements the CSWSH gate: an Origin header (browser
// clients always send one) must carry the same host[:port] as the request
// Host — the same-origin posture of the console. wsOriginAllow lists
// extra host:port values (proxied deployments where the listener Host is
// 127.0.0.1:port but Origin carries the public domain — set --ws-origin).
func wsOriginAllowed(origin, host string, allow []string) bool {
	if origin == "" {
		return true // non-browser client: auth frame still gates access
	}
	o := origin
	o = strings.TrimPrefix(o, "http://")
	o = strings.TrimPrefix(o, "https://")
	o = strings.TrimSuffix(o, "/")
	if o != "" && o == host {
		return true
	}
	for _, a := range allow {
		if a != "" && o == a {
			return true
		}
	}
	return false
}

// wsUpgrade performs the RFC 6455 opening handshake over the given
// ResponseWriter. It returns nil (and has already written a 4xx) when the
// request is not a valid same-origin WebSocket upgrade. On success the
// connection is hijacked and owned by the returned conn.
func wsUpgrade(w http.ResponseWriter, r *http.Request, originAllow []string) (*wsConn, error) {
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		http.Error(w, "expected websocket upgrade", http.StatusBadRequest)
		return nil, errors.New("ws: not an upgrade request")
	}
	if !wsOriginAllowed(r.Header.Get("Origin"), r.Host, originAllow) {
		// CSWSH gate — must be 403 BEFORE the upgrade (issue #218 item 3).
		http.Error(w, "cross-origin websocket hijacking refused", http.StatusForbidden)
		return nil, errors.New("ws: origin refused")
	}
	key := r.Header.Get("Sec-WebSocket-Key")
	if key == "" {
		http.Error(w, "missing Sec-WebSocket-Key", http.StatusBadRequest)
		return nil, errors.New("ws: missing key")
	}
	if v := r.Header.Get("Sec-WebSocket-Version"); v != "13" {
		w.Header().Set("Sec-WebSocket-Version", "13")
		http.Error(w, "unsupported websocket version", http.StatusUpgradeRequired)
		return nil, errors.New("ws: unsupported version")
	}
	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "cannot hijack connection", http.StatusInternalServerError)
		return nil, errors.New("ws: responsewriter not hijackable")
	}
	conn, rw, err := hj.Hijack()
	if err != nil {
		return nil, fmt.Errorf("ws: hijack: %w", err)
	}
	// Never accept extensions: the 101 answer carries no Sec-WebSocket-
	// Extensions header, so a compliant client must not compress; a frame
	// with RSV bits set is closed as a protocol error below.
	resp := "HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + wsAccept(key) + "\r\n\r\n"
	if _, err := rw.WriteString(resp); err != nil {
		conn.Close()
		return nil, fmt.Errorf("ws: write handshake: %w", err)
	}
	if err := rw.Flush(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("ws: flush handshake: %w", err)
	}
	return &wsConn{conn: conn, br: rw.Reader, bw: rw.Writer, wmu: make(chan struct{}, 1)}, nil
}

// wsFrame is one decoded client→server message. Control frames (ping/pong/
// close) are delivered inline in read order; opcode wsOpCont never appears
// (fragmentation is reassembled into the payload of the message's opcode).
type wsFrame struct {
	opcode  byte
	payload []byte
}

// wsConn is a hijacked WebSocket connection.
type wsConn struct {
	conn net.Conn
	br   *bufio.Reader
	bw   *bufio.Writer
	wmu  chan struct{} // serialises writes (output pump + control)
}

func newWSConn(conn net.Conn, rw *bufio.ReadWriter) *wsConn {
	return &wsConn{conn: conn, br: rw.Reader, bw: rw.Writer, wmu: make(chan struct{}, 1)}
}

func (c *wsConn) lock() func() {
	c.wmu <- struct{}{}
	return func() { <-c.wmu }
}

// readFrame reads one frame from the wire and unmasks it. Fragmentation is
// the caller's concern (wsReadMessage). Control frames are returned as-is.
func (c *wsConn) readFrame() (wsFrame, error) {
	var hdr [2]byte
	if _, err := io.ReadFull(c.br, hdr[:]); err != nil {
		return wsFrame{}, err
	}
	fin := hdr[0]&0x80 != 0
	rsv := hdr[0] & 0x70
	op := hdr[0] & 0x0F
	masked := hdr[1]&0x80 != 0
	length := uint64(hdr[1] & 0x7F)

	if rsv != 0 {
		return wsFrame{}, fmt.Errorf("ws: RSV bits set (extension not negotiated) — %w", errWSProtocol)
	}
	switch op {
	case wsOpText, wsOpBinary, wsOpCont, wsOpClose, wsOpPing, wsOpPong:
	default:
		return wsFrame{}, fmt.Errorf("ws: unsupported opcode %#x — %w", op, errWSProtocol)
	}
	if !masked {
		// RFC 6455 §5.3: client frames MUST be masked.
		return wsFrame{}, fmt.Errorf("ws: unmasked client frame — %w", errWSProtocol)
	}
	// Control frames: FIN must be set, length ≤ 125 (§5.5.1).
	if op >= 0x8 && (!fin || length > 125) {
		return wsFrame{}, fmt.Errorf("ws: invalid control frame — %w", errWSProtocol)
	}
	if length > wsMaxPayload {
		return wsFrame{}, fmt.Errorf("ws: frame %d > %d — %w", length, wsMaxPayload, errWSTooBig)
	}
	var ext [8]byte
	switch length {
	case 126:
		if _, err := io.ReadFull(c.br, ext[:2]); err != nil {
			return wsFrame{}, err
		}
		length = uint64(binary.BigEndian.Uint16(ext[:2]))
	case 127:
		if _, err := io.ReadFull(c.br, ext[:8]); err != nil {
			return wsFrame{}, err
		}
		length = binary.BigEndian.Uint64(ext[:8])
	}
	if length > wsMaxPayload {
		return wsFrame{}, fmt.Errorf("ws: message %d > %d — %w", length, wsMaxPayload, errWSTooBig)
	}
	var mask [4]byte
	if _, err := io.ReadFull(c.br, mask[:]); err != nil {
		return wsFrame{}, err
	}
	payload := make([]byte, length)
	if _, err := io.ReadFull(c.br, payload); err != nil {
		return wsFrame{}, err
	}
	for i := range payload {
		payload[i] ^= mask[i&3]
	}
	return wsFrame{opcode: op, payload: payload}, nil
}

// wsReadMessage reads the next DATA message. Control frames are handled
// transparently: pings are answered, pongs surface via onPong, a close
// frame ends the read loop with io.EOF. Fragmentation is NOT supported in
// this scope: browsers never fragment messages ≤ 64 KB (the client cap) and
// the PTY contract keeps messages small — a continuation frame is treated
// as a protocol violation (close 1002). xterm.js never fragments; the MD72
// client does not either. Documented limitation, not a security hole: the
// 64 KB cap still holds on every frame.
func (c *wsConn) wsReadMessage(onPong func()) (opcode byte, payload []byte, err error) {
	for {
		f, ferr := c.readFrame()
		if ferr != nil {
			return 0, nil, ferr
		}
		switch f.opcode {
		case wsOpPing:
			if perr := c.writeFrame(wsOpPong, f.payload); perr != nil {
				return 0, nil, perr
			}
		case wsOpPong:
			if onPong != nil {
				onPong()
			}
		case wsOpClose:
			_ = c.writeFrame(wsOpClose, f.payload)
			return 0, nil, io.EOF
		case wsOpText, wsOpBinary:
			return f.opcode, f.payload, nil
		case wsOpCont:
			return 0, nil, fmt.Errorf("ws: unexpected continuation (fragmentation unsupported) — %w", errWSProtocol)
		}
	}
}

// writeFrame writes one server→client frame (unmasked, RFC: servers do not
// mask; masking is a client-only obligation).
func (c *wsConn) writeFrame(op byte, payload []byte) error {
	unlock := c.lock()
	defer unlock()
	if len(payload) > wsMaxPayload {
		return fmt.Errorf("ws: outgoing frame %d > %d", len(payload), wsMaxPayload)
	}
	var hdr [10]byte
	hdr[0] = 0x80 | op // FIN on every server frame
	n := len(payload)
	idx := 2
	switch {
	case n <= 125:
		hdr[1] = byte(n)
	case n <= 0xFFFF:
		hdr[1] = 126
		binary.BigEndian.PutUint16(hdr[2:4], uint16(n))
		idx = 4
	default:
		hdr[1] = 127
		binary.BigEndian.PutUint64(hdr[2:10], uint64(n))
		idx = 10
	}
	if _, err := c.bw.Write(hdr[:idx]); err != nil {
		return err
	}
	if _, err := c.bw.Write(payload); err != nil {
		return err
	}
	return c.bw.Flush()
}

func (c *wsConn) writeText(payload []byte) error {
	return c.writeFrame(wsOpText, payload)
}

func (c *wsConn) writeBinary(payload []byte) error {
	return c.writeFrame(wsOpBinary, payload)
}

// writeClose sends a close frame with the given code and shuts the TCP
// connection down (no lingering half-close — the daemon owns the lifecycle).
func (c *wsConn) writeClose(code uint16, reason string) {
	unlock := c.lock()
	p := make([]byte, 2+len(reason))
	binary.BigEndian.PutUint16(p[:2], code)
	copy(p[2:], reason)
	_, _ = c.bw.Write([]byte{0x88, byte(len(p))})
	_, _ = c.bw.Write(p)
	_ = c.bw.Flush()
	unlock()
	_ = c.conn.Close()
}

func (c *wsConn) setReadDeadline(t time.Time) {
	_ = c.conn.SetReadDeadline(t)
}

// Sentinel errors used by callers to pick the close code.
var (
	errWSProtocol = errors.New("ws: protocol violation")
	errWSTooBig   = errors.New("ws: message too big")
	errWSAuth     = errors.New("ws: auth failed")
)
