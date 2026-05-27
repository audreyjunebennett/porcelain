package runtime

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/catalog"
	"github.com/lynn/porcelain/chimera/internal/providerlimits"
)

func TestLimitsGuard_overlaysFreshCatalogContext(t *testing.T) {
	dir := t.TempDir()
	limitsPath := filepath.Join(dir, "provider-model-limits.yaml")
	gatewayPath := filepath.Join(dir, "gateway.yaml")
	if err := os.WriteFile(limitsPath, []byte(`
schema_version: 2
defaults:
  context_safety_factor: 1.0
providers:
  groq:
    models:
      groq/live-only:
        rpm: 30
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(gatewayPath, []byte(`
gateway: { listen_port: 3000 }
paths:
  tokens: "./tokens.yaml"
  routing_policy: "./routing-policy.yaml"
  provider_model_limits: "./provider-model-limits.yaml"
routing:
  fallback_chain: ["groq/live-only"]
metrics:
  enabled: false
`), 0o644); err != nil {
		t.Fatal(err)
	}

	rt, err := NewRuntime(gatewayPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(rt.CloseMetrics)

	rt.SetCatalogSnapshot(catalog.NewTestSnapshotWithModelContext(time.Now().UTC(), map[string]int64{
		"groq/live-only": 1000,
	}))

	g := rt.LimitsGuard()
	if g == nil {
		t.Fatal("expected guard")
	}
	if g.Catalog == nil {
		t.Fatal("expected fresh catalog attached")
	}
	d, err := g.Allow(context.Background(), "groq/live-only", providerlimits.RequestAdmission{
		EstPromptTokens: 700,
		MaxTokens:       200,
	})
	if err != nil || !d.Allowed {
		t.Fatalf("catalog overlay should allow: %+v err=%v", d, err)
	}
}

func TestLimitsGuard_staleCatalogDoesNotOverlay(t *testing.T) {
	dir := t.TempDir()
	limitsPath := filepath.Join(dir, "provider-model-limits.yaml")
	gatewayPath := filepath.Join(dir, "gateway.yaml")
	if err := os.WriteFile(limitsPath, []byte(`
schema_version: 2
defaults:
  context_safety_factor: 1.0
providers:
  groq:
    models:
      groq/stale:
        max_prompt_tokens: 100
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(gatewayPath, []byte(`
gateway: { listen_port: 3000 }
paths:
  tokens: "./tokens.yaml"
  routing_policy: "./routing-policy.yaml"
  provider_model_limits: "./provider-model-limits.yaml"
routing:
  fallback_chain: ["groq/stale"]
metrics:
  enabled: false
`), 0o644); err != nil {
		t.Fatal(err)
	}

	rt, err := NewRuntime(gatewayPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(rt.CloseMetrics)

	staleAt := time.Now().UTC().Add(-10 * time.Minute)
	rt.SetCatalogSnapshot(catalog.NewTestSnapshotWithModelContext(staleAt, map[string]int64{
		"groq/stale": 999999,
	}))

	g := rt.LimitsGuard()
	if g == nil {
		t.Fatal("expected guard")
	}
	if g.Catalog != nil {
		t.Fatal("stale snapshot should not attach catalog")
	}
	d, err := g.Allow(context.Background(), "groq/stale", providerlimits.RequestAdmission{EstPromptTokens: 200})
	if err != nil {
		t.Fatal(err)
	}
	if d.Allowed || d.Reason != providerlimits.ReasonContext {
		t.Fatalf("yaml-only cap should still apply: %+v", d)
	}
}

func TestLimitsGuard_yamlContextBeatsFreshCatalog(t *testing.T) {
	dir := t.TempDir()
	limitsPath := filepath.Join(dir, "provider-model-limits.yaml")
	gatewayPath := filepath.Join(dir, "gateway.yaml")
	if err := os.WriteFile(limitsPath, []byte(`
schema_version: 2
defaults:
  context_safety_factor: 1.0
providers:
  groq:
    models:
      groq/x:
        context_window: 500
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(gatewayPath, []byte(`
gateway: { listen_port: 3000 }
paths:
  tokens: "./tokens.yaml"
  routing_policy: "./routing-policy.yaml"
  provider_model_limits: "./provider-model-limits.yaml"
routing:
  fallback_chain: ["groq/x"]
metrics:
  enabled: false
`), 0o644); err != nil {
		t.Fatal(err)
	}

	rt, err := NewRuntime(gatewayPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(rt.CloseMetrics)

	rt.SetCatalogSnapshot(catalog.NewTestSnapshotWithModelContext(time.Now().UTC(), map[string]int64{
		"groq/x": 131072,
	}))

	g := rt.LimitsGuard()
	d, err := g.Allow(context.Background(), "groq/x", providerlimits.RequestAdmission{EstPromptTokens: 600})
	if err != nil {
		t.Fatal(err)
	}
	if d.Allowed || d.Reason != providerlimits.ReasonContext {
		t.Fatalf("yaml cap should beat catalog: %+v", d)
	}
}
