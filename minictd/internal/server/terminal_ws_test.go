// Package server — terminal_ws_test.go (#218, D84).
//
// Unit tests for the RFC 6455 plumbing + the WS PTY endpoint, using a
// minimal in-test WebSocket CLIENT (handshake + masked frames) over
// httptest — no external dependency. Echo round-trips run against a fake
// minict-exec (a shell that signals 'R' on fd 4 and cats stdin→stdout), so
// every byte-path assertion here is real I/O without a container.
package server

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// --- minimal RFC 6455 client (masked frames, text/binary/close only) ------

type wsTestClient struct {
	conn    net.Conn
	br      *bufio.Reader
	respHdr http.Header
	t       *testing.T
}

func wsDial(t *testing.T, url string, origin string) (*wsTestClient, *http.Response) {
	t.Helper()
	u := strings.TrimPrefix(url, "http://")
	host, path := u[:strings.Index(u, "/")], u[strings.Index(u, "/"):]
	conn, err := net.Dial("tcp", host)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	key := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123"))
	req := "GET " + path + " HTTP/1.1\r\nHost: " + host + "\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: " + key + "\r\nSec-WebSocket-Version: 13\r\n"
	if origin != "" {
		req += "Origin: " + origin + "\r\n"
	}
	req += "\r\n"
	if _, err := conn.Write([]byte(req)); err != nil {
		t.Fatalf("write handshake: %v", err)
	}
	br := bufio.NewReader(conn)
	resp, rerr := http.ReadResponse(br, &http.Request{Method: "GET"})
	if rerr != nil {
		t.Fatalf("read response: %v", rerr)
	}
	return &wsTestClient{conn: conn, br: br, respHdr: resp.Header, t: t}, resp
}

// --- tests ---------------------------------------------------------------

func TestWSHandshakeAcceptVector(t *testing.T) {
	// RFC 6455 §1.3: key "dGhlIHNhbXBsZSBub25jZQ==" → "s3pPLMBiTxaQ9kYGzzhZRbK+xOo="
	if got := wsAccept("dGhlIHNhbXBsZSBub25jZQ=="); got != "s3pPLMBiTxaQ9kYGzzhZRbK+xOo=" {
		t.Fatalf("accept vector mismatch: %q", got)
	}
	// a live handshake must echo the same value for our test key
	srv, _, _ := newWSHarness(t, "tok")
	c, resp := wsDial(t, srv.URL+"/v1/sandboxes/x/terminal/ws", "")
	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("handshake → %d, want 101", resp.StatusCode)
	}
	key := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123"))
	want := base64.StdEncoding.EncodeToString(sha1Sum(key + wsGUID))
	if got := c.respHdr.Get("Sec-WebSocket-Accept"); got != want {
		t.Fatalf("live accept mismatch: got %q want %q", got, want)
	}
	c.sendText(`{"type":"close"}`)
	c.close()
}

func sha1Sum(s string) []byte {
	h := sha1.New()
	h.Write([]byte(s))
	return h.Sum(nil)
}

func (w *wsTestClient) sendFrame(fin bool, op byte, payload []byte, mask bool) {
	w.t.Helper()
	var hdr []byte
	b0 := byte(0)
	if fin {
		b0 |= 0x80
	}
	b0 |= op
	hdr = append(hdr, b0)
	n := len(payload)
	mbit := byte(0)
	if mask {
		mbit = 0x80
	}
	switch {
	case n <= 125:
		hdr = append(hdr, mbit|byte(n))
	case n <= 0xFFFF:
		hdr = append(hdr, mbit|126, byte(n>>8), byte(n))
	default:
		hdr = append(hdr, mbit|127)
		var e [8]byte
		binary.BigEndian.PutUint64(e[:], uint64(n))
		hdr = append(hdr, e[:]...)
	}
	if mask {
		mk := []byte{0x11, 0x22, 0x33, 0x44}
		hdr = append(hdr, mk...)
		masked := make([]byte, n)
		for i := range payload {
			masked[i] = payload[i] ^ mk[i&3]
		}
		payload = masked
	}
	if _, err := w.conn.Write(append(hdr, payload...)); err != nil {
		w.t.Fatalf("send frame: %v", err)
	}
}

func (w *wsTestClient) sendText(s string)              { w.sendFrame(true, wsOpText, []byte(s), true) }
func (w *wsTestClient) sendBin(b []byte)               { w.sendFrame(true, wsOpBinary, b, true) }
func (w *wsTestClient) sendUnmasked(op byte, b []byte) { w.sendFrame(true, op, b, false) }

// readFrame reads one server frame (unmasked by RFC).
func (w *wsTestClient) readFrame() (op byte, payload []byte, err error) {
	var hdr [2]byte
	if _, err = io.ReadFull(w.br, hdr[:]); err != nil {
		return 0, nil, err
	}
	op = hdr[0] & 0x0F
	n := int(hdr[1] & 0x7F)
	if hdr[1]&0x80 != 0 {
		return 0, nil, fmt.Errorf("server frame masked (RFC violation)")
	}
	switch n {
	case 126:
		var e [2]byte
		if _, err = io.ReadFull(w.br, e[:]); err != nil {
			return 0, nil, err
		}
		n = int(binary.BigEndian.Uint16(e[:]))
	case 127:
		var e [8]byte
		if _, err = io.ReadFull(w.br, e[:]); err != nil {
			return 0, nil, err
		}
		n = int(binary.BigEndian.Uint64(e[:]))
	}
	p := make([]byte, n)
	if _, err = io.ReadFull(w.br, p); err != nil {
		return 0, nil, err
	}
	return op, p, nil
}

// waitClose reads frames until a close arrives; returns its code.
func (w *wsTestClient) waitClose(timeout time.Duration) uint16 {
	w.t.Helper()
	w.conn.SetReadDeadline(time.Now().Add(timeout))
	for {
		op, p, err := w.readFrame()
		if err != nil {
			w.t.Fatalf("waitClose: %v", err)
		}
		if op == wsOpClose {
			if len(p) >= 2 {
				return binary.BigEndian.Uint16(p[:2])
			}
			return 0
		}
	}
}

// waitText reads frames (skipping binary/ping) until a text frame arrives.
func (w *wsTestClient) waitText(timeout time.Duration) string {
	w.t.Helper()
	w.conn.SetReadDeadline(time.Now().Add(timeout))
	for {
		op, p, err := w.readFrame()
		if err != nil {
			w.t.Fatalf("waitText: %v", err)
		}
		if op == wsOpText {
			return string(p)
		}
	}
}

func (w *wsTestClient) close() { w.conn.Close() }

// --- fake minict-exec: 'R' on fd 4 then cat stdin→stdout -----------------

func fakeExecBin(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "fake-exec")
	script := "#!/bin/bash\nprintf R >&4\nexec cat\n"
	if err := os.WriteFile(p, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func newWSHarness(t *testing.T, token string) (*httptest.Server, *terminalManager, func() int) {
	t.Helper()
	bin := fakeExecBin(t)
	tm := newTerminalManager(bin, nil)
	sessions := 0
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sessions++
		prepare := func() (int, string, error) {
			env := filepath.Join(t.TempDir(), "env")
			_ = os.WriteFile(env, []byte("K=V\n"), 0o600)
			return 0, env, nil
		}
		tm.ServeWS(w, r, token, nil, prepare)
	})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv, tm, func() int { return sessions }
}

func TestWSUpgradeRejectsPlainGet(t *testing.T) {
	srv, _, _ := newWSHarness(t, "tok")
	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("plain GET → %d, want 400", resp.StatusCode)
	}
}

func TestWSOriginRefusedBeforeUpgrade(t *testing.T) {
	srv, _, _ := newWSHarness(t, "tok")
	_, resp := wsDial(t, srv.URL+"/v1/sandboxes/x/terminal/ws", "http://evil.example.com:1")
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("evil origin → %d, want 403", resp.StatusCode)
	}
	if resp.StatusCode == http.StatusSwitchingProtocols {
		t.Fatal("must refuse BEFORE upgrade")
	}
}

func TestWSOriginSameHostAllowed(t *testing.T) {
	srv, _, _ := newWSHarness(t, "tok")
	host := strings.TrimPrefix(srv.URL, "http://")
	c, resp := wsDial(t, srv.URL+"/v1/sandboxes/x/terminal/ws", "http://"+host)
	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("same-host origin → %d, want 101", resp.StatusCode)
	}
	c.sendText(`{"type":"auth","token":"tok"}`)
	if got := c.waitText(3 * time.Second); !strings.Contains(got, `"ready"`) {
		t.Fatalf("expected ready, got %s", got)
	}
	c.sendText(`{"type":"close"}`)
	c.close()
}

func TestWSOriginAllowlist(t *testing.T) {
	bin := fakeExecBin(t)
	tm := newTerminalManager(bin, nil)
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tm.ServeWS(w, r, "tok", []string{"pub.example.com"}, func() (int, string, error) { return 0, "", fmt.Errorf("nope") })
	})
	srv := httptest.NewServer(h)
	defer srv.Close()
	c, resp := wsDial(t, srv.URL+"/v1/sandboxes/x/terminal/ws", "https://pub.example.com")
	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("allowlisted origin → %d, want 101", resp.StatusCode)
	}
	c.sendText(`{"type":"auth","token":"tok"}`)
	// prepare fails → error + close 4404 (auth passed, target unavailable)
	if got := c.waitText(3 * time.Second); !strings.Contains(got, `"error"`) {
		t.Fatalf("expected error frame, got %s", got)
	}
	if code := c.waitClose(3 * time.Second); code != wsCloseTarget {
		t.Fatalf("close code %d, want %d", code, wsCloseTarget)
	}
}

func TestWSAuthWrongToken4401(t *testing.T) {
	srv, _, _ := newWSHarness(t, "tok")
	c, resp := wsDial(t, srv.URL+"/v1/sandboxes/x/terminal/ws", "")
	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("no origin (non-browser) → %d, want 101 (origin gate is not the auth gate)", resp.StatusCode)
	}
	c.sendText(`{"type":"auth","token":"WRONG"}`)
	if code := c.waitClose(3 * time.Second); code != wsCloseAuth {
		t.Fatalf("close code %d, want %d", code, wsCloseAuth)
	}
	c.close()
}

func TestWSAuthMissingFrame4401(t *testing.T) {
	srv, _, _ := newWSHarness(t, "tok")
	c, _ := wsDial(t, srv.URL+"/v1/sandboxes/x/terminal/ws", "")
	// first frame is a control-ish text that is not auth → 4401
	c.sendText(`{"type":"resize","rows":2,"cols":2}`)
	if code := c.waitClose(3 * time.Second); code != wsCloseAuth {
		t.Fatalf("close code %d, want %d", code, wsCloseAuth)
	}
	c.close()
}

func TestWSAuthBinaryFirst4401(t *testing.T) {
	srv, _, _ := newWSHarness(t, "tok")
	c, _ := wsDial(t, srv.URL+"/v1/sandboxes/x/terminal/ws", "")
	c.sendBin([]byte("auth"))
	if code := c.waitClose(3 * time.Second); code != wsCloseAuth {
		t.Fatalf("close code %d, want %d", code, wsCloseAuth)
	}
	c.close()
}

func TestWSUnmaskedFrameClosed1002(t *testing.T) {
	srv, _, _ := newWSHarness(t, "tok")
	c, _ := wsDial(t, srv.URL+"/v1/sandboxes/x/terminal/ws", "")
	c.sendUnmasked(wsOpText, []byte(`{"type":"auth","token":"tok"}`))
	if code := c.waitClose(3 * time.Second); code != wsCloseProtocol {
		t.Fatalf("close code %d, want %d", code, wsCloseProtocol)
	}
	c.close()
}

func TestWSOversizeFrameClosed1009(t *testing.T) {
	srv, _, _ := newWSHarness(t, "tok")
	c, _ := wsDial(t, srv.URL+"/v1/sandboxes/x/terminal/ws", "")
	big := make([]byte, wsMaxPayload+1)
	c.sendFrame(true, wsOpBinary, big, true)
	if code := c.waitClose(3 * time.Second); code != wsCloseTooBig {
		t.Fatalf("close code %d, want %d", code, wsCloseTooBig)
	}
	c.close()
}

func TestWSAuthTimeout4401(t *testing.T) {
	srv, _, _ := newWSHarness(t, "tok")
	c, _ := wsDial(t, srv.URL+"/v1/sandboxes/x/terminal/ws", "")
	// send NOTHING — the 5 s auth window must expire → 4401
	if code := c.waitClose(8 * time.Second); code != wsCloseAuth {
		t.Fatalf("close code %d, want %d", code, wsCloseAuth)
	}
	c.close()
}

func TestWSEchoBinaryRoundTrip(t *testing.T) {
	srv, _, _ := newWSHarness(t, "tok")
	c, _ := wsDial(t, srv.URL+"/v1/sandboxes/x/terminal/ws", "")
	c.sendText(`{"type":"auth","token":"tok"}`)
	if got := c.waitText(3 * time.Second); !strings.Contains(got, `"ready"`) {
		t.Fatalf("expected ready, got %s", got)
	}
	// binary stdin → fake-exec cat → binary stdout, byte-exact (UTF-8 multibyte)
	in := []byte("tiếng Việt 😀 é\n")
	c.sendBin(in)
	c.conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	var got []byte
	for string(got) != string(in) {
		op, p, err := c.readFrame()
		if err != nil {
			t.Fatalf("read: %v (got %q)", err, got)
		}
		if op == wsOpBinary {
			got = append(got, p...)
		}
	}
	c.sendText(`{"type":"close"}`)
	c.close()
}

func TestWSCloseReapsAndCleansEnvFile(t *testing.T) {
	bin := fakeExecBin(t)
	tm := newTerminalManager(bin, nil)
	var envPath string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		prepare := func() (int, string, error) {
			envPath = filepath.Join(t.TempDir(), "env")
			_ = os.WriteFile(envPath, []byte("K=V\n"), 0o600)
			return 0, envPath, nil
		}
		tm.ServeWS(w, r, "tok", nil, prepare)
	})
	srv := httptest.NewServer(h)
	defer srv.Close()
	c, _ := wsDial(t, srv.URL+"/v1/sandboxes/x/terminal/ws", "")
	c.sendText(`{"type":"auth","token":"tok"}`)
	if got := c.waitText(3 * time.Second); !strings.Contains(got, `"ready"`) {
		t.Fatalf("expected ready, got %s", got)
	}
	if n := len(tm.snapshotSessions()); n != 1 {
		t.Fatalf("sessions after ready: %d, want 1", n)
	}
	c.sendText(`{"type":"close"}`)
	if code := c.waitClose(5 * time.Second); code != 1000 {
		t.Fatalf("close code %d, want 1000", code)
	}
	// reap + env-file removal are async — poll briefly
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if len(tm.snapshotSessions()) == 0 {
			if _, statErr := os.Stat(envPath); os.IsNotExist(statErr) {
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("session not reaped or env-file %s still present", envPath)
}
