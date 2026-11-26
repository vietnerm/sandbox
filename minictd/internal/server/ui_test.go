package server

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vietnerm/sandbox/minictd/internal/minict"
)

const inline = `self.__next_f.push([1,"x"])`

func uiServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	root := t.TempDir()
	ui := filepath.Join(root, "ui")
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.MkdirAll(filepath.Join(ui, "_next", "static"), 0o755))
	must(os.WriteFile(filepath.Join(ui, "index.html"),
		[]byte(`<html><script src="/a.js"></script><script>`+inline+`</script></html>`), 0o644))
	must(os.WriteFile(filepath.Join(ui, "404.html"), []byte(`<html>404</html>`), 0o644))
	must(os.WriteFile(filepath.Join(ui, "_next", "static", "app.js"), []byte(`console.log(1)`), 0o644))
	must(os.WriteFile(filepath.Join(ui, ".env"), []byte(`SECRET=1`), 0o644))
	secret := filepath.Join(root, "secret.txt")
	must(os.WriteFile(secret, []byte(`host secret`), 0o644))
	must(os.Symlink(secret, filepath.Join(ui, "leak.txt")))

	state := filepath.Join(root, "state")
	must(os.MkdirAll(filepath.Join(state, "_templates", "web", "rootfs"), 0o755))
	must(os.WriteFile(filepath.Join(state, "_templates", "web", "spec.json"), []byte(`{}`), 0o644))
	must(os.WriteFile(filepath.Join(state, "_templates", "web", "rootfs", "f"), []byte(`12345`), 0o644))
	must(os.MkdirAll(filepath.Join(state, "_templates", "half"), 0o755)) // no spec.json
	must(os.MkdirAll(filepath.Join(state, "_backups"), 0o755))
	must(os.WriteFile(filepath.Join(state, "_backups", "b1.tar.gz"), []byte(`gz`), 0o644))
	must(os.WriteFile(filepath.Join(state, "_backups", "notes.txt"), []byte(`x`), 0o644))

	s := New(&minict.Store{Root: state}, "tok", slog.New(slog.NewTextHandler(io.Discard, nil)))
	must(s.SetUIDir(ui))
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	return ts, ui
}

func do(t *testing.T, method, url, token string) (*http.Response, string) {
	t.Helper()
	req, _ := http.NewRequest(method, url, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(res.Body)
	res.Body.Close()
	return res, string(b)
}

func TestUIServesIndexWithInlineScriptHash(t *testing.T) {
	ts, _ := uiServer(t)
	res, body := do(t, "GET", ts.URL+"/", "")
	if res.StatusCode != 200 || !strings.Contains(body, inline) {
		t.Fatalf("index: %d %q", res.StatusCode, body)
	}
	sum := sha256.Sum256([]byte(inline))
	want := "'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"
	csp := res.Header.Get("Content-Security-Policy")
	if !strings.Contains(csp, want) || strings.Contains(csp, "unsafe-inline' ; script") ||
		!strings.Contains(csp, "frame-ancestors 'none'") || !strings.Contains(csp, "connect-src 'self'") {
		t.Fatalf("csp = %q", csp)
	}
	if strings.Contains(strings.Split(strings.Split(csp, "script-src")[1], ";")[0], "unsafe-inline") {
		t.Fatalf("script-src must not allow unsafe-inline: %q", csp)
	}
	if res.Header.Get("X-Frame-Options") != "DENY" || res.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("security headers missing: %v", res.Header)
	}
}

func TestUISPAFallbackAndAssets(t *testing.T) {
	ts, _ := uiServer(t)
	if res, body := do(t, "GET", ts.URL+"/sandboxes/s-demo", ""); res.StatusCode != 200 || !strings.Contains(body, inline) {
		t.Fatalf("spa fallback: %d", res.StatusCode)
	}
	res, body := do(t, "GET", ts.URL+"/_next/static/app.js", "")
	if res.StatusCode != 200 || body != "console.log(1)" || !strings.Contains(res.Header.Get("Cache-Control"), "immutable") {
		t.Fatalf("asset: %d %q %v", res.StatusCode, body, res.Header)
	}
	if res, _ := do(t, "GET", ts.URL+"/missing.js", ""); res.StatusCode != 404 {
		t.Fatalf("missing asset with extension must 404, got %d", res.StatusCode)
	}
}

func TestUIRefusesTraversalSymlinkDotfilesAndWrites(t *testing.T) {
	ts, _ := uiServer(t)
	for _, p := range []string{"/.env", "/leak.txt", "/..%2fsecret.txt", "/%2e%2e/secret.txt", "/_next/../../secret.txt"} {
		res, body := do(t, "GET", ts.URL+p, "")
		if strings.Contains(body, "host secret") || strings.Contains(body, "SECRET=1") {
			t.Fatalf("%s leaked: %d %q", p, res.StatusCode, body)
		}
	}
	if res, _ := do(t, "POST", ts.URL+"/", ""); res.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("POST / = %d, want 405", res.StatusCode)
	}
}

func TestUIDoesNotBypassAPIAuth(t *testing.T) {
	ts, _ := uiServer(t)
	for _, p := range []string{"/v1/sandboxes", "/v1/templates", "/v1/nope", "/metrics", "/healthz"} {
		if res, _ := do(t, "GET", ts.URL+p, ""); res.StatusCode != http.StatusUnauthorized {
			t.Fatalf("%s without token = %d, want 401", p, res.StatusCode)
		}
	}
}

func TestListTemplatesAndBackups(t *testing.T) {
	ts, _ := uiServer(t)
	res, body := do(t, "GET", ts.URL+"/v1/templates", "tok")
	var tr struct {
		Templates []minict.Artifact `json:"templates"`
	}
	if res.StatusCode != 200 || json.Unmarshal([]byte(body), &tr) != nil {
		t.Fatalf("templates: %d %q", res.StatusCode, body)
	}
	if len(tr.Templates) != 1 || tr.Templates[0].Name != "web" || tr.Templates[0].Image != "template:web" || tr.Templates[0].SizeBytes != 5 {
		t.Fatalf("templates = %+v", tr.Templates)
	}
	res, body = do(t, "GET", ts.URL+"/v1/backups", "tok")
	var br struct {
		Backups []minict.Artifact `json:"backups"`
	}
	if res.StatusCode != 200 || json.Unmarshal([]byte(body), &br) != nil {
		t.Fatalf("backups: %d %q", res.StatusCode, body)
	}
	if len(br.Backups) != 1 || br.Backups[0].Image != "backup:b1" {
		t.Fatalf("backups = %+v", br.Backups)
	}
}

func TestSetUIDirRequiresIndex(t *testing.T) {
	s := New(&minict.Store{Root: t.TempDir()}, "tok", slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := s.SetUIDir(t.TempDir()); err == nil {
		t.Fatal("SetUIDir on a dir without index.html must fail")
	}
}

func TestNoUIKeepsLegacyRouting(t *testing.T) {
	s := New(&minict.Store{Root: t.TempDir()}, "tok", slog.New(slog.NewTextHandler(io.Discard, nil)))
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	if res, _ := do(t, "GET", ts.URL+"/", ""); res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("without --ui-dir, / must stay behind auth, got %d", res.StatusCode)
	}
}

// The access-log middleware must not hide http.Flusher from streaming
// handlers (logs?follow=1 / execstream).
func TestRequestLogKeepsFlusher(t *testing.T) {
	s := New(&minict.Store{Root: t.TempDir()}, "", slog.New(slog.NewTextHandler(io.Discard, nil)))
	var ok bool
	h := s.requestLog(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, ok = w.(http.Flusher)
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/v1/sandboxes/x/logs", nil))
	if !ok {
		t.Fatal("statusRecorder hides http.Flusher: streaming responses would buffer")
	}
}
