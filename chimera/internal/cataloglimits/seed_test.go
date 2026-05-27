package cataloglimits

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lynn/porcelain/chimera/internal/providerlimits"
)

func TestLoadCatalogContextLengths_missingFile(t *testing.T) {
	_, err := LoadCatalogContextLengths(filepath.Join(t.TempDir(), "missing.yaml"))
	if err == nil {
		t.Fatal("expected error for missing catalog")
	}
}

func TestLoadCatalogContextLengths(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "catalog.yaml")
	const src = `
data:
  - id: groq/groq/compound-mini
    context_length: 131072
  - id: groq/groq/compound
    context_length: 1.31072e+05
  - id: ollama/llama3.2:3b
    created: 1
`
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := LoadCatalogContextLengths(path)
	if err != nil {
		t.Fatal(err)
	}
	if m["groq/groq/compound-mini"] != 131072 {
		t.Fatalf("compound-mini: %d", m["groq/groq/compound-mini"])
	}
	if m["groq/groq/compound"] != 131072 {
		t.Fatalf("compound float: %d", m["groq/groq/compound"])
	}
	if _, ok := m["ollama/llama3.2:3b"]; ok {
		t.Fatal("expected missing context_length to be omitted")
	}
}

func TestApplyContextWindows_preservesTPM_andSeedsContext(t *testing.T) {
	cfg, err := providerlimits.Parse([]byte(`
schema_version: 1
providers:
  groq:
    usage_day_timezone: UTC
    models:
      groq/groq/compound-mini:
        rpm: 30
        tpm: 70000
      groq/llama-3.3-70b-versatile:
        rpm: 30
        tpm: 12000
`))
	if err != nil {
		t.Fatal(err)
	}
	catalog := map[string]int64{
		"groq/groq/compound-mini":      131072,
		"groq/llama-3.3-70b-versatile": 131072,
	}
	rep := ApplyContextWindows(cfg, catalog, []string{"ollama/llama3.2:3b"}, ApplyOptions{})
	if len(rep.Updated) != 2 {
		t.Fatalf("updated=%v", rep.Updated)
	}
	if len(rep.Added) != 1 || rep.Added[0] != "ollama/llama3.2:3b(ollama-default)" {
		t.Fatalf("added=%v", rep.Added)
	}
	mini := cfg.Providers["groq"].Models["groq/groq/compound-mini"]
	if mini.TPM == nil || *mini.TPM != 70000 {
		t.Fatalf("tpm preserved: %v", mini.TPM)
	}
	if mini.ContextWindow == nil || *mini.ContextWindow != 131072 {
		t.Fatalf("context_window: %v", mini.ContextWindow)
	}
	if mini.MaxPromptTokens == nil || *mini.MaxPromptTokens != 8192 {
		t.Fatalf("max_prompt_tokens override: %v", mini.MaxPromptTokens)
	}
	ollama := cfg.Providers["ollama"].Models["ollama/llama3.2:3b"]
	if ollama.ContextWindow == nil || *ollama.ContextWindow != 131072 {
		t.Fatalf("ollama default: %+v", ollama)
	}
	if cfg.SchemaVersion != 2 {
		t.Fatalf("schema=%d", cfg.SchemaVersion)
	}
	if cfg.Defaults.ContextSafetyFactor == nil || *cfg.Defaults.ContextSafetyFactor != 0.9 {
		t.Fatalf("defaults safety factor: %v", cfg.Defaults.ContextSafetyFactor)
	}
}

func TestApplyContextWindows_respectsExistingContextUnlessForce(t *testing.T) {
	existing := int64(4096)
	cfg, err := providerlimits.Parse([]byte(`
schema_version: 2
defaults:
  context_safety_factor: 0.9
  max_body_bytes: 3500000
providers:
  groq:
    models:
      groq/x:
        context_window: 4096
`))
	if err != nil {
		t.Fatal(err)
	}
	catalog := map[string]int64{"groq/x": 999999}
	ApplyContextWindows(cfg, catalog, nil, ApplyOptions{})
	if v := cfg.Providers["groq"].Models["groq/x"].ContextWindow; v == nil || *v != existing {
		t.Fatalf("should keep existing context_window, got %v", v)
	}
	ApplyContextWindows(cfg, catalog, nil, ApplyOptions{Force: true})
	if v := cfg.Providers["groq"].Models["groq/x"].ContextWindow; v == nil || *v != 999999 {
		t.Fatalf("force should update, got %v", v)
	}
}

func TestWriteRoundTrip_preservesQuotaFields(t *testing.T) {
	src := `
schema_version: 1
defaults:
  usage_day_timezone: UTC
providers:
  groq:
    usage_day_timezone: UTC
    models:
      groq/fast:
        rpm: 30
        tpm: 6000
`
	cfg, err := providerlimits.Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	ApplyContextWindows(cfg, map[string]int64{"groq/fast": 8192}, nil, ApplyOptions{})
	out, err := providerlimits.Write(cfg)
	if err != nil {
		t.Fatal(err)
	}
	cfg2, err := providerlimits.Parse(out)
	if err != nil {
		t.Fatal(err)
	}
	fast := cfg2.Providers["groq"].Models["groq/fast"]
	if fast.RPM == nil || *fast.RPM != 30 || fast.TPM == nil || *fast.TPM != 6000 {
		t.Fatalf("quota fields lost: %+v", fast)
	}
	if fast.ContextWindow == nil || *fast.ContextWindow != 8192 {
		t.Fatalf("context_window: %+v", fast.ContextWindow)
	}
}
