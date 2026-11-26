package main

import (
	"os"
	"path/filepath"
	"testing"
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
