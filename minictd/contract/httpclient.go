package contract

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
)

// HTTPClient implements Runtime against a running minictd. It is the
// control-plane-side replacement for docker.Client when a sandbox is
// scheduled onto a minict node:
//
//	var rt Runtime = contract.NewHTTPClient("http://127.0.0.1:7601", token)
type HTTPClient struct {
	Base  string // e.g. http://127.0.0.1:7601
	Token string
	HC    *http.Client
}

// NewHTTPClient builds a client. The http.Client has NO overall timeout:
// logs/execstream are long-lived streams; per-op timeouts live in ctx.
func NewHTTPClient(base, token string) *HTTPClient {
	return &HTTPClient{Base: base, Token: token, HC: &http.Client{}}
}

// NewUnixHTTPClient builds a client that dials a unix socket (S4, issue
// #116): the base URL is fixed to "http://unix" (host is ignored — the
// DialContext below routes every request to socketPath). Token semantics
// unchanged: the daemon requires it on every transport (R3).
func NewUnixHTTPClient(socketPath, token string) *HTTPClient {
	tr := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", socketPath)
		},
	}
	return &HTTPClient{Base: "http://unix", Token: token, HC: &http.Client{Transport: tr}}
}

func (c *HTTPClient) newReq(ctx context.Context, method, path string, in any) (*http.Request, error) {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.Base+path, body)
	if err != nil {
		return nil, err
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	return req, nil
}

// call performs a request and decodes a JSON reply. Non-2xx becomes an
// error; 404 maps to ErrNotFound (docker.ErrNotFound parity).
func (c *HTTPClient) call(ctx context.Context, method, path string, in, out any) error {
	req, err := c.newReq(ctx, method, path, in)
	if err != nil {
		return err
	}
	resp, err := c.HC.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNoContent {
		return nil
	}
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(buf.Bytes(), &e)
		if resp.StatusCode == http.StatusNotFound {
			return ErrNotFound
		}
		return fmt.Errorf("minictd %s %s: %d: %s", method, path, resp.StatusCode, e.Error)
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(buf.Bytes(), out)
}

// Run starts a sandbox. Returns the instance name (== handle under minictd).
func (c *HTTPClient) Run(ctx context.Context, spec RunSpec) (string, error) {
	var out struct {
		ID string `json:"id"`
	}
	if err := c.call(ctx, "POST", "/v1/run", &spec, &out); err != nil {
		return "", err
	}
	return out.ID, nil
}

func (c *HTTPClient) Inspect(ctx context.Context, name string) (*ContainerJSON, error) {
	var cj ContainerJSON
	if err := c.call(ctx, "GET", "/v1/sandboxes/"+url.PathEscape(name), nil, &cj); err != nil {
		return nil, err
	}
	return &cj, nil
}

func (c *HTTPClient) Remove(ctx context.Context, name string) error {
	return c.call(ctx, "POST", "/v1/sandboxes/"+url.PathEscape(name)+"/remove", nil, nil)
}

func (c *HTTPClient) Start(ctx context.Context, name string) error {
	return c.call(ctx, "POST", "/v1/sandboxes/"+url.PathEscape(name)+"/start", nil, nil)
}

func (c *HTTPClient) Stop(ctx context.Context, name string, timeoutSec int) error {
	if timeoutSec < 0 {
		timeoutSec = 0
	}
	return c.call(ctx, "POST", "/v1/sandboxes/"+url.PathEscape(name)+"/stop",
		map[string]int{"timeout_sec": timeoutSec}, nil)
}

func (c *HTTPClient) Exec(ctx context.Context, name string, cmd []string) (ExecResult, error) {
	var out ExecResult
	if err := c.call(ctx, "POST", "/v1/sandboxes/"+url.PathEscape(name)+"/exec",
		map[string][]string{"cmd": cmd}, &out); err != nil {
		return ExecResult{}, err
	}
	return out, nil
}

func (c *HTTPClient) ExecV2(ctx context.Context, name string, req ExecV2Request) (ExecV2Result, error) {
	var out ExecV2Result
	if err := c.call(ctx, "POST", "/v1/sandboxes/"+url.PathEscape(name)+"/execv2", &req, &out); err != nil {
		return ExecV2Result{}, err
	}
	return out, nil
}

// ExecV2Stream re-materializes the NDJSON wire format into two pipes plus
// a wait closure — the exact signature docker.Client.ExecV2Stream has.
func (c *HTTPClient) ExecV2Stream(ctx context.Context, name string, req ExecV2Request) (
	stdout, stderr io.ReadCloser, wait func() (ExecV2Result, error), err error,
) {
	hreq, err := c.newReq(ctx, "POST", "/v1/sandboxes/"+url.PathEscape(name)+"/execstream", &req)
	if err != nil {
		return nil, nil, nil, err
	}
	resp, err := c.HC.Do(hreq)
	if err != nil {
		return nil, nil, nil, err
	}
	if resp.StatusCode >= 300 {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&e)
		resp.Body.Close()
		if resp.StatusCode == http.StatusNotFound {
			return nil, nil, nil, ErrNotFound
		}
		return nil, nil, nil, fmt.Errorf("minictd execstream: %d: %s", resp.StatusCode, e.Error)
	}

	outR, outW := io.Pipe()
	errR, errW := io.Pipe()
	resCh := make(chan ExecV2Result, 1)
	errCh := make(chan error, 1)

	go func() {
		sc := bufio.NewScanner(resp.Body)
		sc.Buffer(make([]byte, 0, 256*1024), 8*1024*1024)
		for sc.Scan() {
			line := sc.Bytes()
			if len(line) == 0 {
				continue
			}
			var f StreamFrame
			if err := json.Unmarshal(line, &f); err != nil {
				continue
			}
			switch f.T {
			case "stdout":
				outW.Write(f.Data)
			case "stderr":
				errW.Write(f.Data)
			case "exit":
				resCh <- ExecV2Result{
					ExitCode:   f.ExitCode,
					TimedOut:   f.TimedOut,
					Truncated:  f.Truncated,
					DurationMs: f.DurationMs,
				}
				return
			}
		}
		if err := sc.Err(); err != nil {
			errCh <- err
		} else {
			errCh <- io.EOF
		}
	}()

	wait = func() (ExecV2Result, error) {
		defer resp.Body.Close()
		outW.Close()
		errW.Close()
		select {
		case res := <-resCh:
			return res, nil
		case err := <-errCh:
			return ExecV2Result{}, err
		}
	}
	return outR, errR, wait, nil
}

func (c *HTTPClient) Logs(ctx context.Context, name string, tail int, follow bool) (io.ReadCloser, error) {
	q := url.Values{}
	if tail > 0 {
		q.Set("tail", strconv.Itoa(tail))
	}
	if follow {
		q.Set("follow", "1")
	}
	path := "/v1/sandboxes/" + url.PathEscape(name) + "/logs"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	req, err := c.newReq(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.HC.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&e)
		resp.Body.Close()
		if resp.StatusCode == http.StatusNotFound {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("minictd logs: %d: %s", resp.StatusCode, e.Error)
	}
	return resp.Body, nil
}

func (c *HTTPClient) ListByNamePrefix(ctx context.Context, prefix string) ([]string, error) {
	var out struct {
		Names []string `json:"names"`
	}
	if err := c.call(ctx, "GET", "/v1/sandboxes?prefix="+url.QueryEscape(prefix), nil, &out); err != nil {
		return nil, err
	}
	return out.Names, nil
}

func (c *HTTPClient) Stats(ctx context.Context, name string) (Stats, error) {
	var out Stats
	if err := c.call(ctx, "GET", "/v1/sandboxes/"+url.PathEscape(name)+"/stats", nil, &out); err != nil {
		return Stats{}, err
	}
	return out, nil
}

func (c *HTTPClient) ServerVersion(ctx context.Context) (string, error) {
	var out struct {
		Version string `json:"version"`
	}
	if err := c.call(ctx, "GET", "/v1/version", nil, &out); err != nil {
		return "", err
	}
	return out.Version, nil
}

func (c *HTTPClient) Info(ctx context.Context) (string, error) {
	return c.ServerVersion(ctx)
}

func (c *HTTPClient) NetworkSubnets(ctx context.Context, network string) (subnets, gateways []string, err error) {
	var out struct {
		Subnets  []string `json:"subnets"`
		Gateways []string `json:"gateways"`
	}
	if err := c.call(ctx, "GET", "/v1/subnets?network="+url.QueryEscape(network), nil, &out); err != nil {
		return nil, nil, err
	}
	return out.Subnets, out.Gateways, nil
}

// Wake probes the preview ports of a running instance over host loopback
// (G3, issue #97 — non-contract extra, not part of the Runtime interface:
// only minict nodes serve it). BridgeIP() is always "" for minictd, so the
// wake probe dials 127.0.0.1:<port> (MERGE-GUIDE Step 5). 404 when the
// instance does not exist, 409 when it is stopped (same mapping as
// exec/stop). Returns per-port TCP-ready status.
func (c *HTTPClient) Wake(ctx context.Context, name string) (map[int]bool, error) {
	var out map[string]bool
	if err := c.call(ctx, "POST", "/v1/sandboxes/"+url.PathEscape(name)+"/wake", nil, &out); err != nil {
		return nil, err
	}
	ports := make(map[int]bool, len(out))
	for k, v := range out {
		p, perr := strconv.Atoi(k)
		if perr != nil {
			return nil, fmt.Errorf("wake %s: bad port key %q", name, k)
		}
		ports[p] = v
	}
	return ports, nil
}

// Compile-time proof that HTTPClient satisfies the runtime contract.
var _ Runtime = (*HTTPClient)(nil)
