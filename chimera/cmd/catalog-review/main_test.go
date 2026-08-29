package main

import (
	"path/filepath"
	"testing"
)

func TestRejectActiveOutputCollisions(t *testing.T) {
	dir := t.TempDir()
	activeFree := filepath.Join(dir, "provider-free-tier.yaml")
	activeLimits := filepath.Join(dir, "provider-model-limits.yaml")
	protected := []string{filepath.Join(dir, "catalog.yaml"), activeFree, activeLimits, filepath.Join(dir, "operator.sqlite")}
	if err := rejectOutputCollisions(protected, []string{filepath.Join(dir, "generated.yaml")}); err != nil {
		t.Fatalf("safe generated output rejected: %v", err)
	}
	if err := rejectOutputCollisions(protected, []string{activeFree}); err == nil {
		t.Fatal("expected active free-tier overwrite rejection")
	}
	if err := rejectOutputCollisions(protected, []string{activeLimits}); err == nil {
		t.Fatal("expected active limits overwrite rejection")
	}
	if err := rejectOutputCollisions(protected, []string{protected[3]}); err == nil {
		t.Fatal("expected operator SQLite overwrite rejection")
	}
}
