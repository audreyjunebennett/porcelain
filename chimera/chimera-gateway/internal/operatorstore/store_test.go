package operatorstore

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"
)

func testMigrationsDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "migrations", "chimera-gateway", "operator"))
}

func TestStore_CreateListWorkspace(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "operator.sqlite")
	s, err := Open(dbPath, testMigrationsDir(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	ctx := context.Background()
	w, err := s.CreateWorkspace(ctx, "", "myproj", "f1", []string{"/tmp/a", "/tmp/b"})
	if err != nil {
		t.Fatal(err)
	}
	if w.ID < 1 || len(w.Paths) != 2 {
		t.Fatalf("workspace=%+v", w)
	}

	all, err := s.ListWorkspaces(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 || len(all[0].Paths) != 2 {
		t.Fatalf("list=%+v", all)
	}

	if err := s.DeletePath(ctx, "", w.Paths[0].ID); err != nil {
		t.Fatal(err)
	}
	all2, err := s.ListWorkspaces(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(all2[0].Paths) != 1 {
		t.Fatalf("after delete path: %+v", all2)
	}

	if err := s.DeleteWorkspace(ctx, "", w.ID); err != nil {
		t.Fatal(err)
	}
	all3, err := s.ListWorkspaces(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(all3) != 0 {
		t.Fatalf("want empty, got %+v", all3)
	}
}
