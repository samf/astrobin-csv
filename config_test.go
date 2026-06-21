package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadFilterMap(t *testing.T) {
	path := writeFile(t, t.TempDir(), "config.yaml", []byte(
		"filters:\n  L: 33995\n  H: 43627\n  O: 43628\n"))

	m, err := loadFilterMap(path)
	if err != nil {
		t.Fatalf("loadFilterMap: %v", err)
	}
	want := map[string]int{"L": 33995, "H": 43627, "O": 43628}
	if len(m) != len(want) {
		t.Fatalf("got %d entries, want %d: %v", len(m), len(want), m)
	}
	for k, v := range want {
		if m[k] != v {
			t.Errorf("m[%q] = %d, want %d", k, m[k], v)
		}
	}
}

func TestLoadFilterMapMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does-not-exist.yaml")
	_, err := loadFilterMap(path)
	if err == nil {
		t.Fatal("expected error for missing config, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want it to mention 'not found'", err)
	}
}

func TestLoadFilterMapEmpty(t *testing.T) {
	// Valid YAML but no filters defined.
	path := writeFile(t, t.TempDir(), "empty.yaml", []byte("other: value\n"))
	_, err := loadFilterMap(path)
	if err == nil {
		t.Fatal("expected error for config with no filters, got nil")
	}
}

func TestLoadFilterMapInvalidYAML(t *testing.T) {
	path := writeFile(t, t.TempDir(), "bad.yaml", []byte("filters: : :\n  - not a map\n"))
	if _, err := loadFilterMap(path); err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
}
