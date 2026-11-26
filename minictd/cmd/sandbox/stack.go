// Package main — stack.go (Track M M2, issue #199, D75).
//
// Multi-instance stack management: `sandbox stack up|down|ps` reads a
// stack.json file, starts services in depends_on order with readiness
// checks, and cleans up on failure/down. Client-side only — calls the
// minictd REST API, daemon unchanged.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"sort"
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
	Image     string    `json:"image"`
	Cmd       []string  `json:"cmd,omitempty"`
	Env       []string  `json:"env,omitempty"`
	CPUs      string    `json:"cpus,omitempty"`
	Memory    string    `json:"memory,omitempty"`
	PidsLimit int       `json:"pids_limit,omitempty"`
	Mounts    []string  `json:"mounts,omitempty"`
	Labels    []string  `json:"labels,omitempty"`
	DependsOn []string  `json:"depends_on,omitempty"`
	Ready     *ReadyDef `json:"ready,omitempty"`
}

// ReadyDef is a readiness check for a service.
type ReadyDef struct {
	Type     string `json:"type"` // "http", "tcp", "exec", "none"
	Port     int    `json:"port,omitempty"`
	Path     string `json:"path,omitempty"`
	TimeoutS int    `json:"timeout_s,omitempty"`
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
func stackUp(ctx context.Context, rt *contract.HTTPClient, sf *StackFile) error {
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
			Name:    prefix + name,
			Image:   svc.Image,
			Cmd:     svc.Cmd,
			Env:     svc.Env,
			CPUs:    svc.CPUs,
			Memory:  svc.Memory,
			Volumes: svc.Mounts,
			Labels:  svc.Labels,
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

// waitForReady polls the service until it's ready or timeout.
func waitForReady(ctx context.Context, rt *contract.HTTPClient, name string, rdy *ReadyDef) error {
	timeoutS := rdy.TimeoutS
	if timeoutS <= 0 {
		timeoutS = 30
	}
	deadline := time.Now().Add(time.Duration(timeoutS) * time.Second)
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
			resp, err := http.Get(url)
			if err == nil {
				resp.Body.Close()
				if resp.StatusCode >= 200 && resp.StatusCode < 300 {
					return nil
				}
			}
		default:
			return fmt.Errorf("unsupported ready type: %s", rdy.Type)
		}
		time.Sleep(time.Second)
	}
	return fmt.Errorf("timeout after %ds", timeoutS)
}

// stackDown stops + removes all services (reverse order).
func stackDown(ctx context.Context, rt *contract.HTTPClient, sf *StackFile) error {
	order, err := topoSort(sf.Services)
	if err != nil {
		return err
	}
	prefix := sf.Name + "-"
	// Reverse order for shutdown
	for i := len(order) - 1; i >= 0; i-- {
		name := prefix + order[i]
		_ = rt.Stop(ctx, name, 10)
		_ = rt.Remove(ctx, name)
		fmt.Fprintf(os.Stderr, "stack down: %s removed\n", order[i])
	}
	return nil
}

// stackPs lists the status of all services.
func stackPs(ctx context.Context, rt *contract.HTTPClient, sf *StackFile) error {
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

// stackCleanup stops + removes a list of started service names.
func stackCleanup(ctx context.Context, rt *contract.HTTPClient, names []string, prefix string) {
	for i := len(names) - 1; i >= 0; i-- {
		name := prefix + names[i]
		_ = rt.Stop(ctx, name, 5)
		_ = rt.Remove(ctx, name)
	}
}
