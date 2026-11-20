package minict

// Unit tests for the A3 proactive reconcile + autostart recovery set
// (issue #91, D47). They deliberately do NOT spawn payloads: pidAlive is
// /proc-derived, so a running state pointing at a non-existent pid heals,
// and one pointing at THIS process (startTicks 0 skips the tick check) is
// left alone. The e2e counterparts are MD20/MD21 in tests/test_minictd.sh.

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	return &Store{
		Root:      t.TempDir(),
		MinictBin: "minict", // never spawned by these tests
		Log:       slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
	}
}

func (s *Store) seedState(t *testing.T, name string, st InstanceState) {
	t.Helper()
	if err := validName(name); err != nil {
		t.Fatalf("seed name %q: %v", name, err)
	}
	dir := s.dir(name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if st.Name == "" {
		st.Name = name
	}
	if st.State == "" {
		st.State = StateRunning
	}
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, fState), b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestReconcileAllHealsDeadRunning(t *testing.T) {
	s := newTestStore(t)
	s.seedState(t, "dead", InstanceState{State: StateRunning, PID: 999_999, StartTicks: 42})
	s.seedState(t, "self", InstanceState{State: StateRunning, PID: os.Getpid(), StartTicks: 0}) // alive
	s.seedState(t, "stopped", InstanceState{State: StateStopped, PID: 0})

	healed, err := s.ReconcileAll()
	if err != nil {
		t.Fatalf("ReconcileAll: %v", err)
	}
	want := []string{"dead"}
	if !reflect.DeepEqual(healed, want) {
		t.Fatalf("healed = %v, want %v", healed, want)
	}
	st, err := s.loadState("dead")
	if err != nil {
		t.Fatal(err)
	}
	if st.State != StateStopped || st.PID != 0 || !st.Exited {
		t.Fatalf("dead not healed: %+v", st)
	}
	// self (live pid) and stopped must be untouched
	st, _ = s.loadState("self")
	if st.State != StateRunning {
		t.Fatalf("live instance healed: %+v", st)
	}
}

func TestReconcileAllSkipsBrokenState(t *testing.T) {
	s := newTestStore(t)
	s.seedState(t, "good", InstanceState{State: StateRunning, PID: 999_999})
	broken := s.dir("broken")
	if err := os.MkdirAll(broken, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(broken, fState), []byte("{not-json"), 0o644); err != nil {
		t.Fatal(err)
	}
	healed, err := s.ReconcileAll()
	if err != nil {
		t.Fatalf("ReconcileAll with broken state: %v", err)
	}
	if !reflect.DeepEqual(healed, []string{"good"}) {
		t.Fatalf("healed = %v, want [good]", healed)
	}
}

func TestAutostartNames(t *testing.T) {
	s := newTestStore(t)
	s.seedState(t, "a-live", InstanceState{State: StateRunning, PID: os.Getpid(), StartTicks: 0})
	s.seedState(t, "b-dead", InstanceState{State: StateRunning, PID: 999_999, StartTicks: 42})
	s.seedState(t, "c-stopped", InstanceState{State: StateStopped})
	s.seedState(t, "d-creating", InstanceState{State: StateCreating, PID: 999_998})

	want, err := s.AutostartNames()
	if err != nil {
		t.Fatalf("AutostartNames: %v", err)
	}
	sort.Strings(want)
	if !reflect.DeepEqual(want, []string{"a-live", "b-dead"}) {
		t.Fatalf("AutostartNames = %v, want [a-live b-dead]", want)
	}
	// both must actually be startable from the store's point of view:
	// a-live passes alive(); b-dead heals first. We do not Start() here —
	// that spawns minict — the e2e suite (MD21) covers the real path.
}

func TestAutostartNamesEmptyRoot(t *testing.T) {
	s := newTestStore(t) // Root does not exist yet — ListByNamePrefix returns nil,nil
	got, err := s.AutostartNames()
	if err != nil {
		t.Fatalf("AutostartNames on missing root: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("want empty, got %v", got)
	}
}

// contract import guard removed: reconcile tests need no RunSpec.
