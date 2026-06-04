package virtualmodel

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/operatorstore"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/testsupport"
)

func TestRegistry_ReloadAndResolve(t *testing.T) {
	dir := t.TempDir()
	s, err := operatorstore.Open(filepath.Join(dir, "op.sqlite"), testsupport.GatewayOperatorMigrationsDir(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	seed := operatorstore.ChimeraSeed("0.2.0", []string{"groq/a", "groq/b"}, "groq/a")
	if _, err := s.InsertVirtualModelFull(ctx, seed); err != nil {
		t.Fatal(err)
	}
	reg := NewRegistry()
	if err := reg.Reload(ctx, s); err != nil {
		t.Fatal(err)
	}
	resolved, err := reg.Resolve("Chimera-0.2.0", "")
	if err != nil || resolved == nil {
		t.Fatalf("resolve: %+v err=%v", resolved, err)
	}
	body := map[string]json.RawMessage{
		"model":    json.RawMessage(`"Chimera-0.2.0"`),
		"messages": json.RawMessage(`[{"role":"user","content":"hi"}]`),
	}
	initial, via := PickInitialModel(resolved, body, nil)
	if initial != "groq/a" || via == "" {
		t.Fatalf("pick: model=%q via=%q", initial, via)
	}
}
