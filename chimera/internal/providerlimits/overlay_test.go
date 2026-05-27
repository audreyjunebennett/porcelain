package providerlimits

import (
	"context"
	"testing"
)

type stubContextCatalog map[string]int64

func (m stubContextCatalog) ContextLength(modelID string) (int64, bool) {
	n, ok := m[modelID]
	return n, ok && n > 0
}

func TestOverlayCatalogContext_yamlWinsOverCatalog(t *testing.T) {
	yamlCap := int64(4096)
	catalogCap := int64(131072)
	eff := Effective{
		ModelID:       "groq/x",
		ContextWindow: &yamlCap,
	}
	out := OverlayCatalogContext(eff, stubContextCatalog{"groq/x": catalogCap})
	if out.ContextWindow == nil || *out.ContextWindow != yamlCap {
		t.Fatalf("yaml should win: got %v", out.ContextWindow)
	}
}

func TestOverlayCatalogContext_fillsFromCatalogWhenYAMLUnset(t *testing.T) {
	eff := Effective{ModelID: "groq/x"}
	out := OverlayCatalogContext(eff, stubContextCatalog{"groq/x": 131072})
	if out.ContextWindow == nil || *out.ContextWindow != 131072 {
		t.Fatalf("catalog overlay: got %v", out.ContextWindow)
	}
}

func TestOverlayCatalogContext_nilCatalogNoChange(t *testing.T) {
	eff := Effective{ModelID: "groq/x"}
	out := OverlayCatalogContext(eff, nil)
	if out.ContextWindow != nil {
		t.Fatalf("expected nil context_window, got %v", out.ContextWindow)
	}
}

func TestResolveWithCatalog(t *testing.T) {
	cfg, err := Parse([]byte(`
defaults:
  context_safety_factor: 1.0
providers:
  groq:
    models:
      groq/yaml-only:
        max_prompt_tokens: 8192
      groq/catalog-only:
        rpm: 1
`))
	if err != nil {
		t.Fatal(err)
	}
	cat := stubContextCatalog{
		"groq/yaml-only":    999999,
		"groq/catalog-only": 65536,
	}
	yamlOnly := cfg.ResolveWithCatalog("groq/yaml-only", cat)
	if yamlOnly.MaxPromptTokens == nil || *yamlOnly.MaxPromptTokens != 8192 {
		t.Fatalf("max_prompt_tokens preserved: %v", yamlOnly.MaxPromptTokens)
	}
	if yamlOnly.ContextWindow == nil || *yamlOnly.ContextWindow != 999999 {
		t.Fatalf("catalog fills yaml-only context: %v", yamlOnly.ContextWindow)
	}
	catalogOnly := cfg.ResolveWithCatalog("groq/catalog-only", cat)
	if catalogOnly.ContextWindow == nil || *catalogOnly.ContextWindow != 65536 {
		t.Fatalf("catalog-only context: %v", catalogOnly.ContextWindow)
	}
	if catalogOnly.RPM == nil || *catalogOnly.RPM != 1 {
		t.Fatalf("rpm preserved: %v", catalogOnly.RPM)
	}
}

func TestGuard_usesCatalogContextWhenYAMLUnset(t *testing.T) {
	cfg, err := Parse([]byte(`
defaults:
  context_safety_factor: 1.0
providers:
  groq:
    models:
      groq/live:
        rpm: 100
`))
	if err != nil {
		t.Fatal(err)
	}
	g := &Guard{
		Cfg:     cfg,
		Catalog: stubContextCatalog{"groq/live": 1000},
	}
	d, err := g.Allow(context.Background(), "groq/live", RequestAdmission{EstPromptTokens: 700, MaxTokens: 200})
	if err != nil {
		t.Fatal(err)
	}
	if !d.Allowed {
		t.Fatalf("expected allow under catalog context cap: %+v", d)
	}
	d2, err := g.Allow(context.Background(), "groq/live", RequestAdmission{EstPromptTokens: 900, MaxTokens: 200})
	if err != nil {
		t.Fatal(err)
	}
	if d2.Allowed || d2.Reason != ReasonContext {
		t.Fatalf("expected context deny from catalog overlay: %+v", d2)
	}
}

func TestGuard_yamlContextOverridesCatalog(t *testing.T) {
	cfg, err := Parse([]byte(`
defaults:
  context_safety_factor: 1.0
providers:
  groq:
    models:
      groq/x:
        context_window: 500
`))
	if err != nil {
		t.Fatal(err)
	}
	g := &Guard{
		Cfg:     cfg,
		Catalog: stubContextCatalog{"groq/x": 131072},
	}
	d, err := g.Allow(context.Background(), "groq/x", RequestAdmission{EstPromptTokens: 600})
	if err != nil {
		t.Fatal(err)
	}
	if d.Allowed || d.Reason != ReasonContext {
		t.Fatalf("yaml cap 500 should beat catalog 131072: %+v", d)
	}
}

func TestGuard_staleCatalogNotAttached(t *testing.T) {
	// Guard with nil Catalog simulates stale/missing snapshot — YAML-only.
	cfg, err := Parse([]byte(`
defaults:
  context_safety_factor: 1.0
providers:
  groq:
    models:
      groq/x:
        max_prompt_tokens: 100
`))
	if err != nil {
		t.Fatal(err)
	}
	g := &Guard{Cfg: cfg, Catalog: nil}
	d, err := g.Allow(context.Background(), "groq/x", RequestAdmission{EstPromptTokens: 500})
	if err != nil {
		t.Fatal(err)
	}
	if d.Allowed || d.Reason != ReasonContext {
		t.Fatalf("yaml-only max_prompt_tokens should still enforce: %+v", d)
	}
}
