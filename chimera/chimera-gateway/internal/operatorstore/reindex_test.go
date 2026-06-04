package operatorstore

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/testsupport"
)

func TestBumpReindexGeneration(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "operator.sqlite"), testsupport.GatewayOperatorMigrationsDir(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	ws, err := s.CreateWorkspace(ctx, "", "p", "f", []string{dir})
	if err != nil {
		t.Fatal(err)
	}
	gen1, err := s.BumpReindexGeneration(ctx, "", ws.ID)
	if err != nil || gen1 != 1 {
		t.Fatalf("first bump: gen=%d err=%v", gen1, err)
	}
	gen2, err := s.BumpReindexGeneration(ctx, "", ws.ID)
	if err != nil || gen2 != 2 {
		t.Fatalf("second bump: gen=%d err=%v", gen2, err)
	}
	list, err := s.ListWorkspaces(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ReindexGeneration != 2 {
		t.Fatalf("list: %+v", list)
	}
}
