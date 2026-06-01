package operatorstore

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/testsupport"
	"github.com/lynn/porcelain/chimera/internal/config"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "operator.sqlite"), testsupport.GatewayOperatorMigrationsDir(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestVirtualModel_CRUDAndCascade(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	vm, err := s.CreateVirtualModel(ctx, CreateVirtualModelInput{
		Name: "Test", Version: "1.0.0", Visibility: VisibilityPublic, Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if vm.ModelID != "Test-1.0.0" {
		t.Fatalf("model_id=%q", vm.ModelID)
	}

	chain := []string{"groq/a", "groq/b"}
	if err := s.SetVirtualModelFallback(ctx, "", vm.ID, chain); err != nil {
		t.Fatal(err)
	}
	pol := "ambiguous_default_model: groq/a\nrules: []\n"
	if err := s.SetVirtualModelRoutingPolicy(ctx, "", vm.ID, true, pol); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetVirtualModelByModelID(ctx, "Test-1.0.0")
	if err != nil || got == nil {
		t.Fatalf("get: err=%v got=%+v", err, got)
	}
	if len(got.FallbackChain) != 2 || !got.RoutingPolicyEnabled {
		t.Fatalf("routing=%+v", got)
	}

	dup, err := s.CreateVirtualModel(ctx, CreateVirtualModelInput{
		ModelID: "Test-1.0.0", Name: "X", Version: "1.0.0",
	})
	if err == nil {
		t.Fatalf("expected unique model_id violation, got %+v", dup)
	}

	if err := s.DeleteVirtualModel(ctx, "", vm.ID); err != nil {
		t.Fatal(err)
	}
	after, err := s.GetVirtualModelByModelID(ctx, "Test-1.0.0")
	if err != nil || after != nil {
		t.Fatalf("after delete: %+v err=%v", after, err)
	}
}

func TestBootstrapVirtualModels_seedsRuleCatalogOnly(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	res := &config.Resolved{Semver: "0.2.0"}
	if err := BootstrapVirtualModels(ctx, s, res, nil); err != nil {
		t.Fatal(err)
	}
	vm, err := s.GetVirtualModelByModelID(ctx, "Chimera-0.2.0")
	if err != nil || vm != nil {
		t.Fatalf("expected no bootstrap vm, got %+v err=%v", vm, err)
	}
	rules, err := s.ListRoutingRuleDefinitions(ctx)
	if err != nil || len(rules) < 2 {
		t.Fatalf("routing rule catalog: count=%d err=%v", len(rules), err)
	}
	// idempotent
	if err := BootstrapVirtualModels(ctx, s, res, nil); err != nil {
		t.Fatal(err)
	}
	all, err := s.ListVirtualModels(ctx, "", "")
	if err != nil || len(all) != 0 {
		t.Fatalf("virtual model count=%d err=%v", len(all), err)
	}
}

func TestEnsureGeminiVirtualModel(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	if err := EnsureGeminiVirtualModel(ctx, s, []string{"gemini/a", "gemini/b"}, nil); err != nil {
		t.Fatal(err)
	}
	vm, err := s.GetVirtualModelByModelID(ctx, "Gemini-0.1.0")
	if err != nil || vm == nil {
		t.Fatalf("gemini vm: %+v err=%v", vm, err)
	}
	for _, m := range vm.FallbackChain {
		if len(m) < 7 || m[:7] != "gemini/" {
			t.Fatalf("non-gemini in chain: %q", m)
		}
	}
}
