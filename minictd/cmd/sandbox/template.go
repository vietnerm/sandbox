// Package main — template.go (Track M M3, issue #215, D81).
//
// Static template registry, client-side only — the daemon is unchanged and
// no dependency is added (stdlib only; Track M ground rule).
//
//	sandbox template push <name> --version V --rootfs DIR --out REGDIR
//	sandbox template pull <root>/<name>[:<ver>] --out DIR [--insecure]
//
// `push` turns a local rootfs directory into registry files: an entry in
// index.json (name, version, sha256, size_bytes, created — OCI-descriptor
// shape) plus a gzip tar blob `<name>-<ver>.tar.gz` (gzip, not the roadmap's
// tar.zst: Go stdlib has no zstd and Track M forbids new dependencies — the
// same documented deviation snapshot.go made, D81).
//
// `pull` fetches the index, then the blob, verifies sha256+size BEFORE any
// extraction, and unpacks with the SAME fail-closed extractor the daemon uses
// for tar:/oci: images (minict.SecureUnpack — #104/D50: reject "../" and
// absolute member names, openat(O_NOFOLLOW) walk, O_EXCL duplicates, special
// bits stripped; D70: absolute symlink targets are refused, not sanitized).
// Importing one implementation instead of copying avoids the copy-drift
// anti-pattern that D25 had to add a drift test for.
//
// Threat model (D81): HTTPS is required; plain HTTP only with --insecure AND
// a loopback host (localhost / 127.0.0.0/8 / ::1) for a local static server.
// sha256 proves blob == what the registry publishes; a mismatched blob fails
// before a single file is written ("không để lại file"). If the whole
// registry (index + blob) is attacker-controlled, the extractor is the last
// line of defense — malicious members are refused and the run leaves nothing
// behind. Because of the D70 posture, registry templates may only contain
// RELATIVE symlinks; push refuses absolute targets so a template that could
// never be pulled never lands in an index.
package main

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/vietnerm/sandbox/minictd/internal/minict"
)

// refRe mirrors the daemon's nameRe (store.go): alphanumeric first char, then
// [a-zA-Z0-9_.-], max 64. Name and version are embedded in a FILENAME
// (<name>-<ver>.tar.gz) — the charset has no "/" and cannot start with "." or
// "-", so a bad name/version can never reach outside the registry directory.
var refRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,63}$`)

func validateRefToken(s, what string) error {
	if !refRe.MatchString(s) {
		return fmt.Errorf("registry %s %q: want [a-zA-Z0-9][a-zA-Z0-9_.-]{0,63}", what, s)
	}
	return nil
}

// registryIndex is the index.json schema (v1, D81). Strict on read: unknown
// fields fail (NO MAGIC, same posture as stack.json DisallowUnknownFields).
type registryIndex struct {
	Version   int             `json:"version"`
	Templates []registryEntry `json:"templates"`
}

// registryEntry describes one blob in the registry.
type registryEntry struct {
	Name      string `json:"name"`
	Version   string `json:"version"`
	SHA256    string `json:"sha256"`
	SizeBytes int64  `json:"size_bytes"`
	Created   string `json:"created"` // RFC3339 UTC
}

// blobName is the on-disk/wire name of a template blob.
func blobName(name, version string) string { return name + "-" + version + ".tar.gz" }

// ── push ────────────────────────────────────────────────────────────────── */

// templatePush packs --rootfs DIR into REGDIR/<name>-<ver>.tar.gz and upserts
// the index.json entry (atomically: temp file + rename for both).
func templatePush(name, version, rootfs, regDir string) error {
	if err := validateRefToken(name, "name"); err != nil {
		return err
	}
	if err := validateRefToken(version, "version"); err != nil {
		return err
	}
	fi, err := os.Stat(rootfs)
	if err != nil {
		return fmt.Errorf("--rootfs: %w", err)
	}
	if !fi.IsDir() {
		return fmt.Errorf("--rootfs %q is not a directory", rootfs)
	}
	if err := os.MkdirAll(regDir, 0o755); err != nil {
		return fmt.Errorf("--out: %w", err)
	}

	blob, err := os.CreateTemp(regDir, ".sandbox-tpl-pack-*")
	if err != nil {
		return err
	}
	tmpPath := blob.Name()
	remove := true
	defer func() {
		if remove {
			os.Remove(tmpPath)
		}
	}()

	hash := sha256.New()
	gw := gzip.NewWriter(io.MultiWriter(blob, hash))
	tw := tar.NewWriter(gw)
	packErr := packTree(rootfs, tw)
	if packErr == nil {
		packErr = tw.Close()
	}
	if packErr == nil {
		packErr = gw.Close()
	}
	if packErr == nil {
		packErr = blob.Close()
	} else {
		blob.Close()
	}
	if packErr != nil {
		return packErr
	}
	// Persist the blob (rename is atomic on the same directory).
	final := filepath.Join(regDir, blobName(name, version))
	if err := os.Rename(tmpPath, final); err != nil {
		return err
	}
	remove = false

	size, err := os.Stat(final)
	if err != nil {
		return err
	}
	sum := hex.EncodeToString(hash.Sum(nil))
	if err := upsertIndex(regDir, registryEntry{
		Name:      name,
		Version:   version,
		SHA256:    sum,
		SizeBytes: size.Size(),
		Created:   time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		return err
	}
	fmt.Printf("pushed %s:%s sha256:%s… (%d bytes) -> %s\n", name, version, sum[:12], size.Size(), final)
	return nil
}

// packTree writes a deterministic gzip tar of rootfs into tw. Symlinks are
// stored as-is (relative targets only — an absolute target would be refused
// by SecureUnpack at pull time per D70, so it must never be published);
// special bits are stripped; devices/fifos/sockets are refused.
func packTree(rootfs string, tw *tar.Writer) error {
	base := rootfs
	return filepath.WalkDir(rootfs, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == base {
			return nil // no root marker entry
		}
		rel, err := filepath.Rel(base, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		info, err := d.Info() // Lstat semantics — symlinks not followed
		if err != nil {
			return err
		}
		mode := info.Mode()
		switch {
		case mode.IsDir():
			return tw.WriteHeader(&tar.Header{
				Name: rel + "/", Mode: 0o755, Typeflag: tar.TypeDir,
				ModTime: info.ModTime().UTC(),
			})
		case mode.IsRegular():
			hdr, err := tar.FileInfoHeader(info, "")
			if err != nil {
				return err
			}
			hdr.Name = rel
			hdr.Mode = int64(mode.Perm()) // strip setuid/setgid/sticky
			if err := tw.WriteHeader(hdr); err != nil {
				return err
			}
			f, err := os.Open(p)
			if err != nil {
				return err
			}
			defer f.Close()
			_, err = io.Copy(tw, f)
			return err
		case mode&os.ModeSymlink != 0:
			target, err := os.Readlink(p)
			if err != nil {
				return err
			}
			if strings.HasPrefix(target, "/") {
				return fmt.Errorf("template %s: absolute symlink %s -> %s (D70: registry templates use relative symlinks only)", rootfs, rel, target)
			}
			return tw.WriteHeader(&tar.Header{
				Name: rel, Linkname: target, Typeflag: tar.TypeSymlink,
				Mode: 0o777, ModTime: info.ModTime().UTC(),
			})
		default:
			return fmt.Errorf("template %s: %s is a %v — not packable (rootless posture, PROMPT.MD §0)", rootfs, rel, mode&os.ModeType)
		}
	})
}

// upsertIndex atomically rewrites index.json with e (replace same
// name+version, else append; entries sorted by name,version for stable diffs).
func upsertIndex(regDir string, e registryEntry) error {
	if err := validateRefToken(e.Name, "name"); err != nil {
		return err
	}
	if err := validateRefToken(e.Version, "version"); err != nil {
		return err
	}
	if len(e.SHA256) != 64 {
		return fmt.Errorf("internal: sha256 %q is not 64 hex chars", e.SHA256)
	}
	idx := registryIndex{Version: 1}
	if b, err := os.ReadFile(filepath.Join(regDir, "index.json")); err == nil {
		dec := json.NewDecoder(strings.NewReader(string(b)))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&idx); err != nil {
			return fmt.Errorf("index.json corrupt: %w", err)
		}
	}
	replaced := false
	for i, old := range idx.Templates {
		if old.Name == e.Name && old.Version == e.Version {
			idx.Templates[i] = e
			replaced = true
			break
		}
	}
	if !replaced {
		idx.Templates = append(idx.Templates, e)
	}
	sort.Slice(idx.Templates, func(i, j int) bool {
		if idx.Templates[i].Name != idx.Templates[j].Name {
			return idx.Templates[i].Name < idx.Templates[j].Name
		}
		return idx.Templates[i].Version < idx.Templates[j].Version
	})
	b, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	tmp, err := os.CreateTemp(regDir, ".sandbox-tpl-index-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}
	if err := os.Chmod(tmpPath, 0o644); err != nil {
		os.Remove(tmpPath)
		return err
	}
	return os.Rename(tmpPath, filepath.Join(regDir, "index.json"))
}

// ── pull ────────────────────────────────────────────────────────────────── */

// parsePullRef splits <root>/<name>[:<ver>] into the registry root URL, the
// template name and an optional version. The scheme is required (the
// transport gate decides https vs gated http).
func parsePullRef(arg string) (root, name, version string, err error) {
	schemeEnd := -1
	for _, s := range []string{"https://", "http://"} {
		if strings.HasPrefix(arg, s) {
			schemeEnd = len(s)
			break
		}
	}
	if schemeEnd < 0 {
		return "", "", "", fmt.Errorf("registry root must be an https:// URL (got %q)", arg)
	}
	rest := arg[schemeEnd:]
	slash := strings.LastIndex(rest, "/")
	if slash < 0 {
		return "", "", "", fmt.Errorf("no template name in %q (want <root>/<name>[:<ver>])", arg)
	}
	root = arg[:schemeEnd+slash]
	tail := rest[slash+1:]
	if tail == "" {
		return "", "", "", fmt.Errorf("empty template name in %q", arg)
	}
	if c := strings.LastIndex(tail, ":"); c >= 0 {
		name, version = tail[:c], tail[c+1:]
		if err := validateRefToken(version, "version"); err != nil {
			return "", "", "", err
		}
	} else {
		name = tail
	}
	if err := validateRefToken(name, "name"); err != nil {
		return "", "", "", err
	}
	// Root hygiene: a URL whose path carries "." or ".." segments would be
	// silently normalized by servers — refuse it here so what we fetch is
	// exactly what was typed (NO MAGIC).
	u, perr := url.Parse(root)
	if perr != nil {
		return "", "", "", fmt.Errorf("registry root %q: %w", root, perr)
	}
	for _, seg := range strings.Split(u.Path, "/") {
		// Whole "." segment, and ANY ".." occurrence (whole segment or
		// suffix like "name:.." — servers silently normalize both).
		if seg == "." || strings.Contains(seg, "..") {
			return "", "", "", fmt.Errorf("registry root %q: refusing traversal-looking path segment %q", root, seg)
		}
	}
	return root, name, version, nil
}

// isLoopbackHost reports whether the URL host (no port) is loopback — the
// only place plain HTTP is ever acceptable (D81).
func isLoopbackHost(hostport string) bool {
	host := hostport
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// gateTransport enforces: https always; http only with --insecure AND a
// loopback host; everything else refused before any dial.
func gateTransport(rawURL string, insecure bool) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return err
	}
	switch u.Scheme {
	case "https":
		return nil
	case "http":
		if !insecure {
			return fmt.Errorf("%s: plain http refused — use https, or --insecure for a local server", rawURL)
		}
		if !isLoopbackHost(u.Host) {
			return fmt.Errorf("%s: --insecure only applies to loopback hosts (localhost/127.0.0.1/::1)", rawURL)
		}
		return nil
	default:
		return fmt.Errorf("unsupported scheme %q in %q", u.Scheme, rawURL)
	}
}

// pullHTTPClient builds the registry client: explicit Timeout everywhere
// (M2f/D76 lesson — a client without one can hang past every budget), and
// redirects never downgrade https -> http.
func pullHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 60 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if req.URL.Scheme != "https" && req.URL.Scheme != "http" {
				return fmt.Errorf("redirect to unsupported scheme %q", req.URL.Scheme)
			}
			if via[0].URL.Scheme == "https" && req.URL.Scheme != "https" {
				return errors.New("https downgraded to http by redirect — refused")
			}
			return nil
		},
	}
}

// fetchGet does one GET with fail-loud status handling.
func fetchGet(c *http.Client, url string) (*http.Response, error) {
	resp, err := c.Get(url)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("%s: HTTP %d", url, resp.StatusCode)
	}
	return resp, nil
}

// loadIndex fetches and strictly parses index.json; a missing template is an
// error listing what IS available (fail loud, no silent emptiness).
func loadIndex(c *http.Client, root string) (*registryIndex, error) {
	resp, err := fetchGet(c, root+"/index.json")
	if err != nil {
		return nil, fmt.Errorf("index: %w", err)
	}
	defer resp.Body.Close()
	dec := json.NewDecoder(io.LimitReader(resp.Body, 4<<20))
	dec.DisallowUnknownFields()
	idx := &registryIndex{}
	if err := dec.Decode(idx); err != nil {
		return nil, fmt.Errorf("index.json corrupt: %w", err)
	}
	if idx.Version != 1 {
		return nil, fmt.Errorf("index.json: unsupported schema version %d", idx.Version)
	}
	return idx, nil
}

// pickEntry resolves name+optional version against the index; empty version
// picks the latest by created (tie-break: highest version string).
func pickEntry(idx *registryIndex, name, version string) (*registryEntry, error) {
	var cands []registryEntry
	for _, e := range idx.Templates {
		if e.Name == name {
			cands = append(cands, e)
		}
	}
	if len(cands) == 0 {
		return nil, fmt.Errorf("template %q not in index (available: %s)", name, indexSummary(idx))
	}
	if version == "" {
		best := cands[0]
		for _, e := range cands[1:] {
			if e.Created > best.Created || (e.Created == best.Created && e.Version > best.Version) {
				best = e
			}
		}
		return &best, nil
	}
	for i := range cands {
		if cands[i].Version == version {
			return &cands[i], nil
		}
	}
	vers := make([]string, 0, len(cands))
	for _, e := range cands {
		vers = append(vers, e.Version)
	}
	sort.Strings(vers)
	return nil, fmt.Errorf("template %s: version %q not in index (available: %s)", name, version, strings.Join(vers, ", "))
}

func indexSummary(idx *registryIndex) string {
	if len(idx.Templates) == 0 {
		return "(empty index)"
	}
	type nv struct{ n, v string }
	pairs := make([]nv, 0, len(idx.Templates))
	for _, e := range idx.Templates {
		pairs = append(pairs, nv{e.Name, e.Version})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].n != pairs[j].n {
			return pairs[i].n < pairs[j].n
		}
		return pairs[i].v < pairs[j].v
	})
	out := make([]string, 0, len(pairs))
	for _, p := range pairs {
		out = append(out, p.n+":"+p.v)
	}
	return strings.Join(out, ", ")
}

// templatePull fetches <root>/<name>[:<ver>] and extracts it into outDir.
// Fail-closed end to end: the blob lands in a temp file, is hashed and
// checked against the index BEFORE extraction; extraction happens into a
// temp directory next to the destination and is renamed in only on success —
// every failure path removes its temporaries ("không để lại file").
func templatePull(ref, outDir string, insecure bool) error {
	root, name, version, err := parsePullRef(ref)
	if err != nil {
		return err
	}
	if err := gateTransport(root, insecure); err != nil {
		return err
	}
	if outDir == "" {
		return fmt.Errorf("--out is required")
	}
	if _, err := os.Stat(outDir); err == nil {
		return fmt.Errorf("--out %q already exists (refusing to overwrite; remove it first)", outDir)
	} else if !os.IsNotExist(err) {
		return err
	}
	c := pullHTTPClient()

	idx, err := loadIndex(c, root)
	if err != nil {
		return err
	}
	entry, err := pickEntry(idx, name, version)
	if err != nil {
		return err
	}
	if entry.SizeBytes <= 0 {
		return fmt.Errorf("index entry %s:%s: bad size_bytes %d", name, entry.Version, entry.SizeBytes)
	}

	// 1) blob → temp file (same parent as --out so the final rename stays
	//    on one filesystem), hashing + size counting while streaming.
	parent := filepath.Dir(outDir)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	blobTmp, err := os.CreateTemp(parent, ".sandbox-tpl-blob-*")
	if err != nil {
		return err
	}
	blobPath := blobTmp.Name()
	cleanup := func() {
		blobTmp.Close()
		os.Remove(blobPath)
	}
	resp, err := fetchGet(c, root+"/"+blobName(name, entry.Version))
	if err != nil {
		cleanup()
		return err
	}
	hash := sha256.New()
	n, err := io.Copy(io.MultiWriter(blobTmp, hash), io.LimitReader(resp.Body, entry.SizeBytes+1))
	resp.Body.Close()
	if err != nil {
		cleanup()
		return fmt.Errorf("blob download: %w", err)
	}
	if err := blobTmp.Close(); err != nil {
		cleanup()
		return err
	}
	if n != entry.SizeBytes {
		cleanup()
		return fmt.Errorf("blob size %d != index size_bytes %d (registry inconsistent)", n, entry.SizeBytes)
	}

	// 2) verify sha256 BEFORE extraction — a mismatched blob must leave no
	//    file behind and never reach the extractor.
	sum := hex.EncodeToString(hash.Sum(nil))
	if sum != strings.ToLower(entry.SHA256) {
		cleanup()
		return fmt.Errorf("sha256 mismatch for %s:%s: got %s… want %s… — nothing extracted", name, entry.Version, sum[:12], entry.SHA256[:12])
	}

	// 3) extract into a temp directory; on success rename to --out, on any
	//    failure (including a hostile tar the index vouches for — registry
	//    compromise) remove everything.
	extractDir, err := os.MkdirTemp(parent, ".sandbox-tpl-extract-*")
	if err != nil {
		cleanup()
		return err
	}
	blobFile, err := os.Open(blobPath)
	if err != nil {
		cleanup()
		os.RemoveAll(extractDir)
		return err
	}
	gz, err := gzip.NewReader(blobFile)
	if err != nil {
		blobFile.Close()
		cleanup()
		os.RemoveAll(extractDir)
		return fmt.Errorf("blob is not gzip: %w", err)
	}
	if err := minict.SecureUnpack(gz, extractDir); err != nil {
		gz.Close()
		blobFile.Close()
		cleanup()
		os.RemoveAll(extractDir)
		return fmt.Errorf("template %s:%s refused (nothing extracted): %w", name, entry.Version, err)
	}
	gz.Close()
	blobFile.Close()
	cleanup()

	if err := os.Rename(extractDir, outDir); err != nil {
		os.RemoveAll(extractDir)
		return err
	}
	fmt.Printf("pulled %s:%s sha256:%s… -> %s\n", name, entry.Version, sum[:12], outDir)
	return nil
}

// ── CLI wiring ──────────────────────────────────────────────────────────── */

// templateCmd dispatches `sandbox template <sub>`.
func templateCmd(sub string, args []string) error {
	switch sub {
	case "push":
		var name, version, rootfs, out string
		if len(args) < 1 || strings.HasPrefix(args[0], "--") {
			return errUsage("template push <name> --version V --rootfs DIR --out REGDIR")
		}
		name = args[0]
		args = args[1:]
		for len(args) > 0 {
			if len(args) < 2 {
				return errUsage("template push: flag %q needs a value", args[0])
			}
			switch args[0] {
			case "--version":
				version = args[1]
			case "--rootfs":
				rootfs = args[1]
			case "--out":
				out = args[1]
			default:
				return errUsage("template push: unknown flag %q (supported: --version --rootfs --out)", args[0])
			}
			args = args[2:]
		}
		if version == "" || rootfs == "" || out == "" {
			return errUsage("template push needs --version, --rootfs and --out (no magic defaults)")
		}
		return templatePush(name, version, rootfs, out)

	case "pull":
		var ref, out string
		insecure := false
		if len(args) < 1 || strings.HasPrefix(args[0], "--") {
			return errUsage("template pull <root>/<name>[:<ver>] --out DIR [--insecure]")
		}
		ref = args[0]
		args = args[1:]
		for len(args) > 0 {
			switch {
			case args[0] == "--out" && len(args) >= 2:
				out = args[1]
				args = args[2:]
			case args[0] == "--insecure":
				insecure = true
				args = args[1:]
			default:
				return errUsage("template pull: bad flag %q (supported: --out DIR --insecure)", args[0])
			}
		}
		if out == "" {
			return errUsage("template pull needs --out DIR (no magic defaults)")
		}
		return templatePull(ref, out, insecure)

	default:
		return errUsage("template %q — want push|pull", sub)
	}
}
