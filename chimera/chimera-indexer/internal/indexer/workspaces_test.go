package indexer

import (
	"path/filepath"
	"testing"
)

func TestWorkspacesRootsFingerprint_ChangesWhenPathAddedToExistingWorkspace(t *testing.T) {
	respOne := &WorkspacesAPIResponse{
		Workspaces: []WorkspaceAPIEntry{{
			WorkspaceID: 1,
			ProjectID:   "task-orchestrator",
			FlavorID:    "base",
			Paths: []WorkspacePathAPI{{
				PathID: 10,
				Path:   `C:\repo\alpha`,
			}},
		}},
	}
	respTwo := &WorkspacesAPIResponse{
		Workspaces: []WorkspaceAPIEntry{{
			WorkspaceID: 1,
			ProjectID:   "task-orchestrator",
			FlavorID:    "base",
			Paths: []WorkspacePathAPI{
				{PathID: 10, Path: `C:\repo\alpha`},
				{PathID: 11, Path: `C:\repo\beta`},
			},
		}},
	}

	fpOne := WorkspacesRootsFingerprint(respOne)
	fpTwo := WorkspacesRootsFingerprint(respTwo)
	if fpOne == "" || fpTwo == "" {
		t.Fatalf("fingerprints must be non-empty: one=%q two=%q", fpOne, fpTwo)
	}
	if fpOne == fpTwo {
		t.Fatalf("expected different fingerprints when path added to same workspace: %q", fpOne)
	}
}

func TestRootsSnapshotFingerprint_MatchesResponseForSameRoots(t *testing.T) {
	rootDir := `C:\repo\alpha`
	resp := &WorkspacesAPIResponse{
		Workspaces: []WorkspaceAPIEntry{{
			WorkspaceID: 7,
			ProjectID:   "p1",
			FlavorID:    "f1",
			Paths: []WorkspacePathAPI{{
				PathID: 2,
				Path:   rootDir,
			}},
		}},
	}
	roots := []Root{{
		AbsPath: filepath.Clean(rootDir),
		Scope: ScopeFragment{
			ProjectID:   "p1",
			FlavorID:    "f1",
			WorkspaceID: "7",
		},
	}}

	respFP := WorkspacesRootsFingerprint(resp)
	rootsFP := RootsSnapshotFingerprint(roots)
	if respFP != rootsFP {
		t.Fatalf("response=%q roots=%q", respFP, rootsFP)
	}
}

func TestWorkspacesRootsFingerprint_IgnoresEmptyPaths(t *testing.T) {
	resp := &WorkspacesAPIResponse{
		Workspaces: []WorkspaceAPIEntry{
			{WorkspaceID: 1, Paths: nil},
			{
				WorkspaceID: 2,
				ProjectID:   "p",
				Paths:       []WorkspacePathAPI{{PathID: 1, Path: `C:\only`}},
			},
		},
	}
	fp := WorkspacesRootsFingerprint(resp)
	if fp == "" {
		t.Fatal("expected non-empty fingerprint")
	}
	if got := WorkspaceIDsFromResponse(resp); got != "2" {
		t.Fatalf("workspace ids=%q", got)
	}
}
