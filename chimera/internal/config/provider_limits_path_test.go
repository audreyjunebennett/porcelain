package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadGatewayYAML_providerLimits_defaultPath_missingFile_emptySpec(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	gw := filepath.Join(dir, "config", "gateway.yaml")
	if err := os.MkdirAll(filepath.Dir(gw), 0o755); err != nil {
		t.Fatal(err)
	}
	raw := strings.TrimSpace(`
gateway: { listen_port: 3000 }
paths: { tokens: "./t.yaml", routing_policy: "./r.yaml" }
routing: { fallback_chain: ["a/b"] }
`)
	if err := os.WriteFile(gw, []byte(raw+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := LoadGatewayYAML(gw, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "config", "provider-model-limits.yaml")
	if res.ProviderLimitsPath != want {
		t.Fatalf("path: got %q want %q", res.ProviderLimitsPath, want)
	}
	if res.ProviderLimitsSpec == nil || len(res.ProviderLimitsSpec.Providers) != 0 {
		t.Fatalf("expected non-nil empty spec, got %+v", res.ProviderLimitsSpec)
	}
}

func TestLoadGatewayYAML_providerLimits_parsesSiblingFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	gw := filepath.Join(dir, "config", "gateway.yaml")
	if err := os.MkdirAll(filepath.Dir(gw), 0o755); err != nil {
		t.Fatal(err)
	}
	limitsPath := filepath.Join(filepath.Dir(gw), "provider-model-limits.yaml")
	if err := os.WriteFile(limitsPath, []byte(`
schema_version: 1
providers:
  groq:
    usage_day_timezone: UTC
    rpm: 30
`), 0o644); err != nil {
		t.Fatal(err)
	}
	raw := `
gateway: { listen_port: 3000 }
paths: { tokens: "./t.yaml", routing_policy: "./r.yaml" }
routing: { fallback_chain: ["a/b"] }
`
	if err := os.WriteFile(gw, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := LoadGatewayYAML(gw, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.ProviderLimitsSpec == nil || res.ProviderLimitsSpec.Providers["groq"].RPM == nil {
		t.Fatalf("groq rpm missing: %+v", res.ProviderLimitsSpec)
	}
}
