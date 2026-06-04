package cataloglimits

import (
	"database/sql"
	"encoding/json"
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

func TestCatalogModelIDs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "catalog.yaml")
	const src = `
data:
  - id: groq/a
    context_length: 8192
  - id: gemini/b
    context_length: 4096
`
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	ids, err := CatalogModelIDs(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 2 || ids[0] != "gemini/b" || ids[1] != "groq/a" {
		t.Fatalf("ids=%v", ids)
	}
}

func TestMergeEnsureModels_deduplicates(t *testing.T) {
	got := MergeEnsureModels([]string{"groq/a", "groq/b"}, []string{"groq/b", "gemini/x"})
	if len(got) != 3 || got[0] != "gemini/x" {
		t.Fatalf("got=%v", got)
	}
}

func TestLoadEnsureModelsFromOperatorSQLite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "operator.sqlite")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE virtual_models (
		id INTEGER PRIMARY KEY, model_id TEXT NOT NULL, name TEXT NOT NULL, version TEXT NOT NULL,
		description TEXT NOT NULL DEFAULT '', enabled INTEGER NOT NULL DEFAULT 1,
		visibility TEXT NOT NULL DEFAULT 'public', created_by_principal_id TEXT NOT NULL DEFAULT '',
		tenant_id TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL, updated_at TEXT NOT NULL
	); CREATE TABLE virtual_model_fallback (
		virtual_model_id INTEGER PRIMARY KEY, chain_json TEXT NOT NULL, updated_at TEXT NOT NULL
	);`); err != nil {
		t.Fatal(err)
	}
	chain, _ := json.Marshal([]string{"groq/fast", "groq/slow"})
	if _, err := db.Exec(`INSERT INTO virtual_models(id, model_id, name, version, enabled, created_at, updated_at)
		VALUES (1, 'Test-1.0.0', 'Test', '1.0.0', 1, 'now', 'now')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO virtual_model_fallback(virtual_model_id, chain_json, updated_at)
		VALUES (1, ?, 'now')`, string(chain)); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	ids, err := LoadEnsureModelsFromOperatorSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 2 || ids[0] != "groq/fast" || ids[1] != "groq/slow" {
		t.Fatalf("ids=%v", ids)
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
