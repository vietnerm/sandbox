// Package main — stack.go (Track M M2, issue #199, D75; residuals d–i
// issue #203, D76).
//
// Multi-instance stack management: `sandbox stack up|down|ps` reads a
// stack.json file, starts services in depends_on order with readiness
// checks, and cleans up on failure/down. Client-side only — calls the
// minictd REST API, daemon unchanged.
//
// M2 residuals (D76): services declare `pasta_ports` (mapped 1:1 to
// RunSpec.PastaPorts, pre-validated with the D67 grammar so a bad entry
// fails at parse time, before any instance starts); ready `exec` is
// implemented over execv2 (exit 0 = ready) and ready `type`/`port`/`cmd`
// are validated at parse time; ready `http` uses an http.Client with a
// per-attempt Timeout (http.Get has none and can hang past timeout_s);
// `stack down` classifies errors — missing instance is skipped, real
// errors are reported and make the command exit non-zero, "removed" is
// only printed when the instance was actually removed.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/vietnerm/sandbox/minictd/contract"
)

// StackFile is the top-level stack.json schema (D75).
type StackFile struct {
	Name     string                `json:"name"`
	Services map[string]ServiceDef `json:"services"`
}

// ServiceDef is one service in the stack.
type ServiceDef struct {
	Image      string    `json:"image"`
	Cmd        []string  `json:"cmd,omitempty"`
	Env        []string  `json:"env,omitempty"`
	CPUs       string    `json:"cpus,omitempty"`
	Memory     string    `json:"memory,omitempty"`
	PidsLimit  int       `json:"pids_limit,omitempty"`
	Mounts     []string  `json:"mounts,omitempty"`
	Labels     []string  `json:"labels,omitempty"`
	DependsOn  []string  `json:"depends_on,omitempty"`
	PastaPorts []string  `json:"pasta_ports,omitempty"` // host:ns forwards via pasta (D67 grammar, #203 d)
	Ready      *ReadyDef `json:"ready,omitempty"`
}

// ReadyDef is a readiness check for a service.
//   - type http: GET http://127.0.0.1:<port><path>, 2xx = ready
//     (reaches the service only through a declared pasta forward — the
//     sandbox netns is loopback-only without one, D17)
//   - type tcp:  TCP connect 127.0.0.1:<port> succeeds = ready
//   - type exec: execv2 <cmd> inside the instance, exit 0 = ready (#203 e)
//   - type none: skip
type ReadyDef struct {
	Type     string   `json:"type"`           // "http", "tcp", "exec", "none"
	Port     int      `json:"port,omitempty"` // http/tcp only, 1..65535 (#203 f)
	Path     string   `json:"path,omitempty"` // http only
	Cmd      []string `json:"cmd,omitempty"`  // exec only, non-empty (#203 e)
	TimeoutS int      `json:"timeout_s,omitempty"`
}

// Stack-side bounds of the D67 pasta grammar (mirror of
// minictd/internal/minict pastaPort* constants — the client does not
// import daemon internals; the daemon re-validates fail-closed and stays
// the source of truth).
const (
	stackPastaHostMin = 1024
	stackPastaHostMax = 65535
	stackPastaNsMin   = 1
	stackPastaNsMax   = 65535
	stackPastaCap     = 16
)

// validatePastaPortsStack mirrors parsePastaPorts (D67) fail-closed at
// parse time so a bad entry aborts `stack up` before ANY instance starts
// (a daemon-side rejection would otherwise roll back already-started
// services for a purely static config error).
func validatePastaPortsStack(entries []string) error {
	if len(entries) == 0 {
		return nil
	}
	if len(entries) > stackPastaCap {
		return fmt.Errorf("pasta_ports: %d entries exceeds the cap of %d", len(entries), stackPastaCap)
	}
	seen := map[string]bool{}
	for _, e := range entries {
		raw := strings.TrimSpace(e)
		proto := "tcp"
		rest := raw
		if before, after, ok := strings.Cut(raw, "/"); ok {
			switch strings.ToLower(before) {
			case "tcp", "udp":
				proto = before
			default:
				return fmt.Errorf("pasta_ports entry %q: protocol must be tcp or udp", raw)
			}
			rest = after
		}
		if strings.ContainsAny(rest, " \t-") {
			return fmt.Errorf("pasta_ports entry %q: ranges/spaces are not supported (grammar: [tcp|udp/]<host>:<ns>)", raw)
		}
		hp, np, ok := strings.Cut(rest, ":")
		if !ok || hp == "" || np == "" {
			return fmt.Errorf("pasta_ports entry %q: need [tcp|udp/]<host>:<ns>", raw)
		}
		host, err := strconv.Atoi(hp)
		if err != nil {
			return fmt.Errorf("pasta_ports entry %q: host port %q is not a number", raw, hp)
		}
		ns, err := strconv.Atoi(np)
		if err != nil {
			return fmt.Errorf("pasta_ports entry %q: namespace port %q is not a number", raw, np)
		}
		if host < stackPastaHostMin || host > stackPastaHostMax {
			return fmt.Errorf("pasta_ports entry %q: host port %d outside %d..%d (rootless bind floor)", raw, host, stackPastaHostMin, stackPastaHostMax)
		}
		if ns < stackPastaNsMin || ns > stackPastaNsMax {
			return fmt.Errorf("pasta_ports entry %q: namespace port %d outside %d..%d", raw, ns, stackPastaNsMin, stackPastaNsMax)
		}
		key := proto + "/" + strconv.Itoa(host)
		if seen[key] {
			return fmt.Errorf("pasta_ports entry %q: duplicate host bind %s", raw, key)
		}
		seen[key] = true
	}
	return nil
}

// validateReady checks a service's ready block at parse time (#203 e/f):
// unknown type is an error NOW, not at run time; http/tcp need a port in
// 1..65535; exec needs a non-empty cmd.
func validateReady(name string, rdy *ReadyDef) error {
	if rdy == nil {
		return nil
	}
	switch rdy.Type {
	case "http", "tcp":
		if rdy.Port < 1 || rdy.Port > 65535 {
			return fmt.Errorf("service %q: ready.type %s needs port 1..65535, got %d", name, rdy.Type, rdy.Port)
		}
	case "exec":
		if len(rdy.Cmd) == 0 {
			return fmt.Errorf("service %q: ready.type exec needs a non-empty cmd", name)
		}
	case "none":
		// nothing to validate
	default:
		return fmt.Errorf("service %q: ready.type %q is not one of http|tcp|exec|none", name, rdy.Type)
	}
	return nil
}

// parseStack reads + validates a stack.json file. Unknown fields → error.
func parseStack(path string) (*StackFile, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("stack: open %s: %w", path, err)
	}
	defer f.Close()
	dec := json.NewDecoder(f)
	dec.DisallowUnknownFields()
	var sf StackFile
	if err := dec.Decode(&sf); err != nil {
		return nil, fmt.Errorf("stack: parse %s: %w", path, err)
	}
	if sf.Name == "" {
		return nil, fmt.Errorf("stack: name is required")
	}
	if len(sf.Services) == 0 {
		return nil, fmt.Errorf("stack: at least one service required")
	}
	// Validate depends_on references
	for name, svc := range sf.Services {
		for _, dep := range svc.DependsOn {
			if _, ok := sf.Services[dep]; !ok {
				return nil, fmt.Errorf("stack: service %q depends_on unknown %q", name, dep)
			}
		}
		if err := validatePastaPortsStack(svc.PastaPorts); err != nil {
			return nil, fmt.Errorf("stack: service %q: %w", name, err)
		}
		if err := validateReady(name, svc.Ready); err != nil {
			return nil, err
		}
	}
	return &sf, nil
}

// topoSort returns service names in dependency order (dependencies first).
func topoSort(services map[string]ServiceDef) ([]string, error) {
	var order []string
	visited := map[string]bool{}
	var visit func(name string, path []string) error
	visit = func(name string, path []string) error {
		if visited[name] {
			return nil
		}
		for _, p := range path {
			if p == name {
				return fmt.Errorf("stack: circular dependency: %v", append(path, name))
			}
		}
		svc := services[name]
		for _, dep := range svc.DependsOn {
			if err := visit(dep, append(path, name)); err != nil {
				return err
			}
		}
		visited[name] = true
		order = append(order, name)
		return nil
	}
	names := make([]string, 0, len(services))
	for n := range services {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		if err := visit(n, nil); err != nil {
			return nil, err
		}
	}
	return order, nil
}

// stackUp starts all services in dependency order with readiness checks.
// rt is the contract.Runtime interface (not the concrete HTTP client) so
// the error policy of stackDown is unit-testable against a stub (#203 g).
func stackUp(ctx context.Context, rt contract.Runtime, sf *StackFile) error {
	order, err := topoSort(sf.Services)
	if err != nil {
		return err
	}
	// Prefix instance names with stack name to avoid collisions.
	prefix := sf.Name + "-"
	started := []string{}
	for _, name := range order {
		svc := sf.Services[name]
		spec := contract.RunSpec{
			Name:       prefix + name,
			Image:      svc.Image,
			Cmd:        svc.Cmd,
			Env:        svc.Env,
			CPUs:       svc.CPUs,
			Memory:     svc.Memory,
			Volumes:    svc.Mounts,
			Labels:     svc.Labels,
			PastaPorts: svc.PastaPorts, // #203 d: map 1:1 (daemon re-validates fail-closed)
		}
		if svc.PidsLimit > 0 {
			spec.PidsLimit = svc.PidsLimit
		}
		id, err := rt.Run(ctx, spec)
		if err != nil {
			// Cleanup already-started instances
			fmt.Fprintf(os.Stderr, "stack up: %s failed: %v — cleaning up\n", name, err)
			stackCleanup(ctx, rt, started, prefix)
			return err
		}
		started = append(started, name)
		fmt.Fprintf(os.Stderr, "stack up: %s started (id=%s)\n", name, id)

		// Readiness check
		if svc.Ready != nil && svc.Ready.Type != "none" {
			if err := waitForReady(ctx, rt, prefix+name, svc.Ready); err != nil {
				fmt.Fprintf(os.Stderr, "stack up: %s not ready: %v — cleaning up\n", name, err)
				stackCleanup(ctx, rt, started, prefix)
				return err
			}
			fmt.Fprintf(os.Stderr, "stack up: %s ready\n", name)
		}
	}
	return nil
}

// readyHTTPClient is created per waitForReady call (#203 f): http.Get uses
// the package DefaultClient which has NO timeout and can hang past
// timeout_s; a per-attempt client deadline bounds connect+headers+body
// while the outer poll loop keeps the overall deadline.
func readyHTTPClient() *http.Client {
	return &http.Client{Timeout: 2 * time.Second}
}

// waitForReady polls the service until it's ready or timeout.
func waitForReady(ctx context.Context, rt contract.Runtime, name string, rdy *ReadyDef) error {
	timeoutS := rdy.TimeoutS
	if timeoutS <= 0 {
		timeoutS = 30
	}
	deadline := time.Now().Add(time.Duration(timeoutS) * time.Second)
	client := readyHTTPClient()
	for time.Now().Before(deadline) {
		switch rdy.Type {
		case "tcp":
			conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", rdy.Port), 2*time.Second)
			if err == nil {
				conn.Close()
				return nil
			}
		case "http":
			url := fmt.Sprintf("http://127.0.0.1:%d%s", rdy.Port, rdy.Path)
			if rdy.Path == "" {
				url = fmt.Sprintf("http://127.0.0.1:%d/", rdy.Port)
			}
			resp, err := client.Get(url)
			if err == nil {
				resp.Body.Close()
				if resp.StatusCode >= 200 && resp.StatusCode < 300 {
					return nil
				}
			}
		case "exec":
			// #203 e: exit 0 = ready (compose healthcheck model).
			// Non-zero/transport errors keep polling until the
			// overall deadline — the payload may still be booting.
			res, err := rt.ExecV2(ctx, name, contract.ExecV2Request{
				Cmd:      rdy.Cmd,
				TimeoutS: 5,
			})
			if err == nil && res.ExitCode == 0 {
				return nil
			}
		default:
			// Unreachable: parseStack validates the type
			// (#203 e), kept fail-closed for direct callers.
			return fmt.Errorf("unsupported ready type: %s", rdy.Type)
		}
		time.Sleep(time.Second)
	}
	return fmt.Errorf("timeout after %ds", timeoutS)
}

// stackDown stops + removes all services (reverse order). Error policy
// (#203 g): an instance that does not exist is skipped silently (compose
// down semantics); any OTHER error is collected, reported, and makes
// stackDown return an error (the CLI exits non-zero). "removed" is only
// printed when the instance was actually removed. After a failed Stop
// (not NotFound) the instance is NOT removed — it may still be running.
func stackDown(ctx context.Context, rt contract.Runtime, sf *StackFile) error {
	order, err := topoSort(sf.Services)
	if err != nil {
		return err
	}
	prefix := sf.Name + "-"
	var errs []string
	// Reverse order for shutdown
	for i := len(order) - 1; i >= 0; i-- {
		svc := order[i]
		name := prefix + svc
		stopErr := rt.Stop(ctx, name, 10)
		if stopErr != nil {
			if errors.Is(stopErr, contract.ErrNotFound) {
				continue // missing instance → skip, not an error (#203 g)
			}
			errs = append(errs, fmt.Sprintf("%s: stop: %v", svc, stopErr))
			continue // do not remove an instance that failed to stop
		}
		rmErr := rt.Remove(ctx, name)
		if rmErr != nil {
			if errors.Is(rmErr, contract.ErrNotFound) {
				continue // raced removal → nothing left to do
			}
			errs = append(errs, fmt.Sprintf("%s: remove: %v", svc, rmErr))
			continue
		}
		fmt.Fprintf(os.Stderr, "stack down: %s removed\n", svc) // only on a real removal
	}
	if len(errs) > 0 {
		return fmt.Errorf("stack down: %s", strings.Join(errs, "; "))
	}
	return nil
}

// stackPs lists the status of all services.
func stackPs(ctx context.Context, rt contract.Runtime, sf *StackFile) error {
	order, err := topoSort(sf.Services)
	if err != nil {
		return err
	}
	prefix := sf.Name + "-"
	type row struct {
		Name   string `json:"name"`
		Status string `json:"status"`
	}
	var rows []row
	for _, name := range order {
		r := row{Name: name, Status: "missing"}
		cj, err := rt.Inspect(ctx, prefix+name)
		if err == nil {
			r.Status = cj.State.Status
		}
		rows = append(rows, r)
	}
	return emitJSON(rows)
}

// stackCleanup stops + removes a list of started service names (rollback).
// Best-effort by design — the original Run error is what gets reported —
// but cleanup failures are printed, never swallowed silently (#203 h:
// rollback must be observable).
func stackCleanup(ctx context.Context, rt contract.Runtime, names []string, prefix string) {
	for i := len(names) - 1; i >= 0; i-- {
		name := prefix + names[i]
		if err := rt.Stop(ctx, name, 5); err != nil && !errors.Is(err, contract.ErrNotFound) {
			fmt.Fprintf(os.Stderr, "stack up: rollback %s: stop: %v\n", names[i], err)
		}
		if err := rt.Remove(ctx, name); err != nil && !errors.Is(err, contract.ErrNotFound) {
			fmt.Fprintf(os.Stderr, "stack up: rollback %s: remove: %v\n", names[i], err)
		}
	}
}
