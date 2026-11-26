package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/vietnerm/sandbox/minictd/contract"
)

func TestParseStackValid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "stack.json")
	os.WriteFile(path, []byte(`{
		"name": "test",
		"services": {
			"web": {"image": "base", "cmd": ["/bin/sh", "-c", "sleep 1"], "depends_on": ["db"], "ready": {"type": "tcp", "port": 8080, "timeout_s": 5}},
			"db": {"image": "base", "cmd": ["/bin/sh", "-c", "sleep 1"]}
		}
	}`), 0644)

	sf, err := parseStack(path)
	if err != nil {
		t.Fatalf("parseStack: %v", err)
	}
	if sf.Name != "test" {
		t.Errorf("name: got %q, want test", sf.Name)
	}
	if len(sf.Services) != 2 {
		t.Errorf("services: got %d, want 2", len(sf.Services))
	}
	if sf.Services["web"].Ready == nil || sf.Services["web"].Ready.Type != "tcp" {
		t.Errorf("web ready not parsed")
	}
}

func TestParseStackUnknownField(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	os.WriteFile(path, []byte(`{"name":"x","services":{"a":{"image":"base","bogus_field":true}}}`), 0644)

	_, err := parseStack(path)
	if err == nil {
		t.Fatal("expected error for unknown field, got nil")
	}
}

func TestParseStackMissingName(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "noname.json")
	os.WriteFile(path, []byte(`{"services":{"a":{"image":"base"}}}`), 0644)

	_, err := parseStack(path)
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestParseStackBadDependsOn(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "baddep.json")
	os.WriteFile(path, []byte(`{"name":"x","services":{"a":{"image":"base","depends_on":["nonexistent"]}}}`), 0644)

	_, err := parseStack(path)
	if err == nil {
		t.Fatal("expected error for depends_on unknown service")
	}
}

func TestTopoSort(t *testing.T) {
	services := map[string]ServiceDef{
		"web":   {DependsOn: []string{"db", "cache"}},
		"db":    {},
		"cache": {DependsOn: []string{"db"}},
	}
	order, err := topoSort(services)
	if err != nil {
		t.Fatalf("topoSort: %v", err)
	}
	// db must come before cache and web; cache before web
	pos := map[string]int{}
	for i, n := range order {
		pos[n] = i
	}
	if pos["db"] >= pos["cache"] {
		t.Errorf("db must come before cache: order=%v", order)
	}
	if pos["db"] >= pos["web"] {
		t.Errorf("db must come before web: order=%v", order)
	}
	if pos["cache"] >= pos["web"] {
		t.Errorf("cache must come before web: order=%v", order)
	}
}

func TestTopoSortCircular(t *testing.T) {
	services := map[string]ServiceDef{
		"a": {DependsOn: []string{"b"}},
		"b": {DependsOn: []string{"a"}},
	}
	_, err := topoSort(services)
	if err == nil {
		t.Fatal("expected circular dependency error")
	}
}

// ---------- M2 residuals d–i (#203, D76) ----------

func TestValidatePastaPortsStack(t *testing.T) {
	good := [][]string{
		{"8080:80"},
		{"tcp/5353:53", "udp/1053:53"},
		{strconv.Itoa(stackPastaHostMin) + ":80", "tcp/" + strconv.Itoa(stackPastaHostMax) + ":65535"},
	}
	bad := map[string][]string{
		"garbage":       {"tcp/abc:def"},
		"no colon":      {"8080"},
		"reserved host": {"80:80"},    // below the 1024 rootless floor
		"host too high": {"65536:80"}, // above 65535
		"ns zero":       {"8080:0"},   // ns must be 1..65535
		"dup host bind": {"8080:80", "tcp/8080:90"},
		"range dash":    {"8080-8090:80"},
		"bad proto":     {"sctp/8080:80"},
		"cap exceeded":  {fmt.Sprintf("%d:80", stackPastaHostMin), fmt.Sprintf("%d:81", stackPastaHostMin+1), fmt.Sprintf("%d:82", stackPastaHostMin+2), fmt.Sprintf("%d:83", stackPastaHostMin+3), fmt.Sprintf("%d:84", stackPastaHostMin+4), fmt.Sprintf("%d:85", stackPastaHostMin+5), fmt.Sprintf("%d:86", stackPastaHostMin+6), fmt.Sprintf("%d:87", stackPastaHostMin+7), fmt.Sprintf("%d:88", stackPastaHostMin+8), fmt.Sprintf("%d:89", stackPastaHostMin+9), fmt.Sprintf("%d:90", stackPastaHostMin+10), fmt.Sprintf("%d:91", stackPastaHostMin+11), fmt.Sprintf("%d:92", stackPastaHostMin+12), fmt.Sprintf("%d:93", stackPastaHostMin+13), fmt.Sprintf("%d:94", stackPastaHostMin+14), fmt.Sprintf("%d:95", stackPastaHostMin+15), fmt.Sprintf("%d:96", stackPastaHostMin+16)},
	}
	for _, entries := range good {
		if err := validatePastaPortsStack(entries); err != nil {
			t.Errorf("want OK for %v, got %v", entries, err)
		}
	}
	for why, entries := range bad {
		err := validatePastaPortsStack(entries)
		if err == nil {
			t.Errorf("want error for %s: %v", why, entries)
		}
	}
}

func TestParseStackPastaPortsBad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "stack.json")
	os.WriteFile(path, []byte(`{"name":"x","services":{"web":{"image":"base","pasta_ports":["80:80"]}}}`), 0644)
	_, err := parseStack(path)
	if err == nil {
		t.Fatal("reserved host port 80 must fail at parse, before any instance starts")
	}
	if !strings.Contains(err.Error(), "pasta_ports") {
		t.Errorf("error should mention pasta_ports: %v", err)
	}
}

func TestParseStackReadyUnknownType(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "stack.json")
	os.WriteFile(path, []byte(`{"name":"x","services":{"a":{"image":"base","ready":{"type":"grpc","port":8080}}}}`), 0644)
	_, err := parseStack(path)
	if err == nil {
		t.Fatal("unknown ready type must fail at parse time, not at run time (#203 e)")
	}
	if !strings.Contains(err.Error(), "http|tcp|exec|none") {
		t.Errorf("error should list valid types: %v", err)
	}
}

func TestParseStackReadyBadPort(t *testing.T) {
	for _, spec := range []string{
		`{"type":"tcp","port":0}`,
		`{"type":"tcp","port":70000}`,
		`{"type":"http","port":-1}`,
		`{"type":"http","port":65536}`,
	} {
		dir := t.TempDir()
		path := filepath.Join(dir, "stack.json")
		os.WriteFile(path, []byte(`{"name":"x","services":{"a":{"image":"base","ready":`+spec+`}}}`), 0644)
		if _, err := parseStack(path); err == nil {
			t.Errorf("ready %s must fail port validation", spec)
		}
	}
}

func TestParseStackExecReady(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "good.json")
	os.WriteFile(good, []byte(`{"name":"x","services":{"a":{"image":"base","ready":{"type":"exec","cmd":["/bin/true"]}}}}`), 0644)
	sf, err := parseStack(good)
	if err != nil {
		t.Fatalf("exec ready with cmd must parse: %v", err)
	}
	if len(sf.Services["a"].Ready.Cmd) != 1 {
		t.Fatal("exec cmd not parsed")
	}
	bad := filepath.Join(dir, "bad.json")
	os.WriteFile(bad, []byte(`{"name":"x","services":{"a":{"image":"base","ready":{"type":"exec"}}}}`), 0644)
	if _, err := parseStack(bad); err == nil {
		t.Fatal("exec ready without cmd must fail at parse")
	}
}

func TestWaitForReadyHTTPHappyAndTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()
	u := srv.URL[len("http://"):]
	port, _ := strconv.Atoi(u[strings.LastIndex(u, ":")+1:])
	rdy := &ReadyDef{Type: "http", Port: port, TimeoutS: 10}
	if err := waitForReady(context.Background(), &stubRuntime{}, "inst", rdy); err != nil {
		t.Fatalf("happy path: %v", err)
	}
	// Closed port → connection refused every attempt → timeout error
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	closed := l.Addr().(*net.TCPAddr).Port
	l.Close()
	rdyBad := &ReadyDef{Type: "http", Port: closed, TimeoutS: 2}
	err := waitForReady(context.Background(), &stubRuntime{}, "inst", rdyBad)
	if err == nil || !strings.Contains(err.Error(), "timeout after 2s") {
		t.Errorf("want timeout after 2s, got %v", err)
	}
}

func TestWaitForReadyExec(t *testing.T) {
	ok := &stubRuntime{execRes: contract.ExecV2Result{ExitCode: 0}}
	if err := waitForReady(context.Background(), ok, "inst", &ReadyDef{Type: "exec", Cmd: []string{"/bin/true"}, TimeoutS: 5}); err != nil {
		t.Fatalf("exec exit 0 must be ready: %v", err)
	}
	// exit 1 forever → poll until deadline, then timeout
	bad := &stubRuntime{execRes: contract.ExecV2Result{ExitCode: 1}}
	err := waitForReady(context.Background(), bad, "inst", &ReadyDef{Type: "exec", Cmd: []string{"/bin/false"}, TimeoutS: 2})
	if err == nil || !strings.Contains(err.Error(), "timeout after 2s") {
		t.Errorf("want timeout after 2s, got %v", err)
	}
}

// stubRuntime implements contract.Runtime for stack policy tests (#203 g).
type stubRuntime struct {
	runErr       error
	execRes      contract.ExecV2Result
	execErr      error
	stopFail     error           // non-NotFound stop error, any name
	removeFail   error           // non-NotFound remove error, any name
	stopNotFound map[string]bool // names whose Stop/Remove return ErrNotFound
	ran          []string
	stopped      []string
	removed      []string
}

func (s *stubRuntime) Run(ctx context.Context, spec contract.RunSpec) (string, error) {
	if s.runErr != nil {
		return "", s.runErr
	}
	s.ran = append(s.ran, spec.Name)
	return "id-" + spec.Name, nil
}
func (s *stubRuntime) Inspect(ctx context.Context, name string) (*contract.ContainerJSON, error) {
	return &contract.ContainerJSON{}, nil
}
func (s *stubRuntime) Remove(ctx context.Context, name string) error {
	if s.stopNotFound[name] {
		return contract.ErrNotFound
	}
	if s.removeFail != nil {
		return s.removeFail
	}
	s.removed = append(s.removed, name)
	return nil
}
func (s *stubRuntime) Start(ctx context.Context, name string) error { return nil }
func (s *stubRuntime) Stop(ctx context.Context, name string, timeoutSec int) error {
	if s.stopNotFound[name] {
		return contract.ErrNotFound
	}
	if s.stopFail != nil {
		return s.stopFail
	}
	s.stopped = append(s.stopped, name)
	return nil
}
func (s *stubRuntime) Exec(ctx context.Context, name string, cmd []string) (contract.ExecResult, error) {
	return contract.ExecResult{}, nil
}
func (s *stubRuntime) ExecV2(ctx context.Context, name string, req contract.ExecV2Request) (contract.ExecV2Result, error) {
	return s.execRes, s.execErr
}
func (s *stubRuntime) ExecV2Stream(ctx context.Context, name string, req contract.ExecV2Request) (io.ReadCloser, io.ReadCloser, func() (contract.ExecV2Result, error), error) {
	return nil, nil, nil, nil
}
func (s *stubRuntime) Logs(ctx context.Context, name string, tail int, follow bool) (io.ReadCloser, error) {
	return nil, nil
}
func (s *stubRuntime) ListByNamePrefix(ctx context.Context, prefix string) ([]string, error) {
	return nil, nil
}
func (s *stubRuntime) Stats(ctx context.Context, name string) (contract.Stats, error) {
	return contract.Stats{}, nil
}
func (s *stubRuntime) ServerVersion(ctx context.Context) (string, error) { return "0", nil }
func (s *stubRuntime) Info(ctx context.Context) (string, error)          { return "", nil }
func (s *stubRuntime) NetworkSubnets(ctx context.Context, network string) (subnets, gateways []string, err error) {
	return nil, nil, nil
}

func stackTestFile(t *testing.T) *StackFile {
	t.Helper()
	return &StackFile{Name: "st", Services: map[string]ServiceDef{
		"db":  {Image: "base"},
		"web": {Image: "base", DependsOn: []string{"db"}},
	}}
}

func TestStackDownMissingSkipped(t *testing.T) {
	rt := &stubRuntime{stopNotFound: map[string]bool{"st-web": true, "st-db": true}}
	if err := stackDown(context.Background(), rt, stackTestFile(t)); err != nil {
		t.Fatalf("all-missing stack down must be a no-op success (#203 g): %v", err)
	}
	if len(rt.removed) != 0 || len(rt.stopped) != 0 {
		t.Errorf("nothing should be stopped/removed, got %v/%v", rt.stopped, rt.removed)
	}
}

func TestStackDownHappy(t *testing.T) {
	rt := &stubRuntime{}
	if err := stackDown(context.Background(), rt, stackTestFile(t)); err != nil {
		t.Fatalf("happy down: %v", err)
	}
	if len(rt.removed) != 2 || len(rt.stopped) != 2 {
		t.Errorf("want both services stopped+removed, got %v/%v", rt.stopped, rt.removed)
	}
}

func TestStackDownRemoveErrorReported(t *testing.T) {
	rt := &stubRuntime{removeFail: fmt.Errorf("boom-500")}
	err := stackDown(context.Background(), rt, stackTestFile(t))
	if err == nil || !strings.Contains(err.Error(), "boom-500") {
		t.Fatalf("real remove error must surface, not be swallowed: %v", err)
	}
	if len(rt.removed) != 0 {
		t.Errorf("no instance was actually removed, removed=%v", rt.removed)
	}
}

func TestStackDownStopErrorNoRemove(t *testing.T) {
	rt := &stubRuntime{stopFail: fmt.Errorf("stop-boom")}
	err := stackDown(context.Background(), rt, stackTestFile(t))
	if err == nil || !strings.Contains(err.Error(), "stop-boom") {
		t.Fatalf("real stop error must surface: %v", err)
	}
	if len(rt.removed) != 0 {
		t.Errorf("must NOT remove an instance whose Stop failed: removed=%v", rt.removed)
	}
}
