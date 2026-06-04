package operatorstore

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/testsupport"
)

func TestImportSupervisedYAMLRootsIfEmpty(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "indexer.supervised.yaml")
	root := filepath.Join(dir, "watch-me")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "defaults:\n  project_id: proj\n  flavor_id: flav\nroots:\n  - path: " + filepath.ToSlash(root) + "\n"
	if err := os.WriteFile(yamlPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	store, err := Open(filepath.Join(dir, "op.sqlite"), testsupport.GatewayOperatorMigrationsDir(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	if err := ImportSupervisedYAMLRootsIfEmpty(context.Background(), store, "", yamlPath, nil); err != nil {
		t.Fatal(err)
	}
	ws, err := store.ListWorkspaces(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(ws) != 1 || len(ws[0].Paths) != 1 {
		t.Fatalf("workspaces=%+v", ws)
	}
	if filepath.Clean(ws[0].Paths[0].Path) != filepath.Clean(root) {
		t.Fatalf("path=%q want %q", ws[0].Paths[0].Path, root)
	}

	// Second call is a no-op.
	if err := ImportSupervisedYAMLRootsIfEmpty(context.Background(), store, "", yamlPath, nil); err != nil {
		t.Fatal(err)
	}
	ws2, _ := store.ListWorkspaces(context.Background(), "")
	if len(ws2) != 1 {
		t.Fatalf("expected no duplicate import, got %d workspaces", len(ws2))
	}
}
