package server

// GĐ3 mục 1 (manifest §7): the web console is a STATIC export
// (console/out from `next build`) served by minictd itself on the same
// listener as the API — no extra process, no extra port, so the ≤ 30 MB
// RSS budget and the 127.0.0.1 default bind both still hold.
//
// Security posture:
//   - static files are public build output (no secrets) and are served
//     WITHOUT the token; every /v1/*, /metrics and /healthz request still
//     goes through s.auth (the console sends the token as a Bearer header,
//     never a cookie, so there is no ambient credential to CSRF);
//   - GET/HEAD only; path is cleaned, must stay under the UI root after
//     symlink resolution (no traversal, no symlink escape), dotfiles refused;
//   - nothing is cached in RAM: files are read from disk per request;
//   - HTML gets a strict CSP whose script-src lists the sha256 of each
//     inline <script> in that file (Next.js export inlines its RSC payload),
//     so no 'unsafe-inline' for scripts.

import (
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

// SetUIDir enables the static console. dir must contain index.html.
func (s *Server) SetUIDir(dir string) error {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return fmt.Errorf("ui dir: %w", err)
	}
	if fi, err := os.Stat(filepath.Join(real, "index.html")); err != nil || !fi.Mode().IsRegular() {
		return fmt.Errorf("ui dir %s: index.html missing (run `make console` first)", dir)
	}
	s.uiDir = real
	return nil
}

// withUI routes API paths to api and everything else to the static UI.
func (s *Server) withUI(api http.Handler) http.Handler {
	top := http.NewServeMux()
	top.Handle("/v1/", api)
	top.Handle("/metrics", api)
	top.Handle("/healthz", api)
	top.HandleFunc("/", s.handleUI)
	return top
}

var (
	inlineScriptRe = regexp.MustCompile(`(?is)<script\b([^>]*)>(.*?)</script>`)
	srcAttrRe      = regexp.MustCompile(`(?i)\bsrc\s*=`)
)

// cspFor builds the CSP for one HTML document.
func cspFor(html []byte) string {
	var hashes []string
	for _, m := range inlineScriptRe.FindAllSubmatch(html, -1) {
		attrs, body := m[1], m[2]
		if len(body) == 0 || srcAttrRe.Match(attrs) {
			continue
		}
		sum := sha256.Sum256(body)
		hashes = append(hashes, "'sha256-"+base64.StdEncoding.EncodeToString(sum[:])+"'")
	}
	script := "'self'"
	if len(hashes) > 0 {
		script += " " + strings.Join(hashes, " ")
	}
	return "default-src 'self'; script-src " + script +
		"; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self' data:" +
		"; connect-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'"
}

var errOutside = errors.New("outside ui root")

// resolveUI maps a URL path to a regular file under the UI root.
func (s *Server) resolveUI(urlPath string) (string, error) {
	clean := path.Clean("/" + urlPath)
	for _, seg := range strings.Split(clean, "/") {
		if strings.HasPrefix(seg, ".") {
			return "", errOutside // dotfiles / dot-dirs never served
		}
	}
	try := func(rel string) (string, error) {
		full := filepath.Join(s.uiDir, filepath.FromSlash(rel))
		real, err := filepath.EvalSymlinks(full)
		if err != nil {
			return "", err
		}
		if real != s.uiDir && !strings.HasPrefix(real, s.uiDir+string(os.PathSeparator)) {
			return "", errOutside
		}
		fi, err := os.Stat(real)
		if err != nil {
			return "", err
		}
		if !fi.Mode().IsRegular() {
			return "", os.ErrNotExist
		}
		return real, nil
	}
	if clean == "/" {
		return try("index.html")
	}
	if p, err := try(clean); err == nil || errors.Is(err, errOutside) {
		return p, err
	}
	if path.Ext(clean) == "" {
		if p, err := try(clean + ".html"); err == nil || errors.Is(err, errOutside) {
			return p, err
		}
		// SPA fallback: client-side routes (/sandboxes/<name>, …)
		return try("index.html")
	}
	return "", os.ErrNotExist
}

func (s *Server) handleUI(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("X-Frame-Options", "DENY")
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		h.Set("Allow", "GET, HEAD")
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	file, err := s.resolveUI(r.URL.Path)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	body, err := os.ReadFile(file)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	ctype := mime.TypeByExtension(filepath.Ext(file))
	if ctype == "" {
		ctype = http.DetectContentType(body)
	}
	h.Set("Content-Type", ctype)
	if strings.HasPrefix(ctype, "text/html") {
		h.Set("Content-Security-Policy", cspFor(body))
		h.Set("Cache-Control", "no-cache")
	} else if strings.HasPrefix(r.URL.Path, "/_next/static/") {
		h.Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		h.Set("Cache-Control", "no-cache")
	}
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodGet {
		_, _ = w.Write(body)
	}
}
