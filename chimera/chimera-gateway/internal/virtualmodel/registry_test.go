package virtualmodel

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/operatorstore"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/testsupport"
	"github.com/lynn/porcelain/chimera/internal/config"
)

func TestRegistry_ReloadAndResolve(t *testing.T) {
	dir := t.TempDir()
	s, err := operatorstore.Open(filepath.Join(dir, "op.sqlite"), testsupport.GatewayOperatorMigrationsDir(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	res := &config.Resolved{
		Semver: "0.2.0", VirtualModelID: "Chimera-0.2.0",
		FallbackChain: []string{"groq/a", "groq/b"},
	}
	if err := operatorstore.BootstrapVirtualModels(ctx, s, res, nil); err != nil {
		t.Fatal(err)
	}
	reg := NewRegistry()
	if err := reg.Reload(ctx, s); err != nil {
		t.Fatal(err)
	}
	vm, err := reg.Resolve("Chimera-0.2.0", "")
	if err != nil || vm == nil {
		t.Fatalf("resolve: %+v err=%v", vm, err)
	}
	body := map[string]json.RawMessage{
		"model":    json.RawMessage(`"Chimera-0.2.0"`),
		"messages": json.RawMessage(`[{"role":"user","content":"hi"}]`),
	}
	initial, via := PickInitialModel(vm, body, nil)
	if initial != "groq/a" || via == "" {
		t.Fatalf("pick: model=%q via=%q", initial, via)
	}
}
