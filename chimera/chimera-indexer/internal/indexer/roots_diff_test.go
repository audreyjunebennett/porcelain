package indexer

import (
	"path/filepath"
	"testing"
)

func TestDiffRoots_detectsAddAndRemove(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := filepath.Join(dir, "a")
	b := filepath.Join(dir, "b")
	prev := []Root{{
		ID: "a", AbsPath: a,
		Scope: ScopeFragment{WorkspaceID: "1", ProjectID: "p", FlavorID: "f"},
	}}
	next := []Root{{
		ID: "b", AbsPath: b,
		Scope: ScopeFragment{WorkspaceID: "1", ProjectID: "p", FlavorID: "f"},
	}}
	added, removed := DiffRoots(prev, next)
	if len(added) != 1 || added[0].AbsPath != b {
		t.Fatalf("added=%v", added)
	}
	if len(removed) != 1 || removed[0].AbsPath != a {
		t.Fatalf("removed=%v", removed)
	}
}

func TestDiffRoots_samePathDifferentWorkspace(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	p := filepath.Join(dir, "shared")
	prev := []Root{{AbsPath: p, Scope: ScopeFragment{WorkspaceID: "1"}}}
	next := []Root{{AbsPath: p, Scope: ScopeFragment{WorkspaceID: "2"}}}
	added, removed := DiffRoots(prev, next)
	if len(added) != 1 || len(removed) != 1 {
		t.Fatalf("expected swap: added=%v removed=%v", added, removed)
	}
}
