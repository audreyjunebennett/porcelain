package indexer

import "testing"

func TestIngestProject_WorkspaceAlias(t *testing.T) {
	if got := IngestProject(ScopeFragment{WorkspaceID: "ws"}); got != "ws" {
		t.Fatalf("workspace: got %q", got)
	}
	if got := IngestProject(ScopeFragment{ProjectID: "p", WorkspaceID: "ws"}); got != "p" {
		t.Fatalf("project wins: got %q", got)
	}
}

func TestResolved_IngestHeaders_mergeOrder(t *testing.T) {
	root := Root{ID: "r", AbsPath: "/tmp/x", Scope: ScopeFragment{ProjectID: "rootproj"}}
	r := Resolved{
		DefaultScope: ScopeFragment{ProjectID: "def", FlavorID: "deflav"},
		GlobOverrides: []GlobOverride{
			{Pattern: "**/*.md", Scope: ScopeFragment{FlavorID: "mdflav"}},
		},
	}
	p, f := r.IngestHeaders(root, "src/a.go")
	if p != "rootproj" || f != "deflav" {
		t.Fatalf("go file: project=%q flavor=%q", p, f)
	}
	p, f = r.IngestHeaders(root, "docs/hi.md")
	if p != "rootproj" || f != "mdflav" {
		t.Fatalf("md file: project=%q flavor=%q", p, f)
	}
}

func TestResolved_IngestHeaders_defaultsOnly(t *testing.T) {
	root := Root{ID: "r", AbsPath: "/x"}
	r := Resolved{DefaultScope: ScopeFragment{ProjectID: "solo", FlavorID: "f"}}
	p, f := r.IngestHeaders(root, "any.go")
	if p != "solo" || f != "f" {
		t.Fatalf("got %q %q", p, f)
	}
}
