package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadGatewayYAML_metricsPaths(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	gw := filepath.Join(dir, "config", "gateway.yaml")
	if err := os.MkdirAll(filepath.Dir(gw), 0o755); err != nil {
		t.Fatal(err)
	}
	raw := strings.TrimSpace(`
gateway:
  listen_port: 3000
paths:
  api_keys: "./api-keys.yaml"
  routing_policy: "./routing-policy.yaml"
routing:
  fallback_chain: ["groq/x"]
`)
	if err := os.WriteFile(gw, []byte(raw+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := LoadGatewayYAML(gw, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !res.MetricsEnabled {
		t.Fatal("metrics should default enabled")
	}
	wantDB := filepath.Join(dir, "data", "gateway", "metrics.sqlite")
	if res.MetricsSQLitePath != wantDB {
		t.Fatalf("MetricsSQLitePath=%q want %q", res.MetricsSQLitePath, wantDB)
	}
	wantMig := filepath.Join(dir, "migrations", "chimera-gateway", "metrics")
	if res.MetricsMigrationsDir != wantMig {
		t.Fatalf("MetricsMigrationsDir=%q want %q", res.MetricsMigrationsDir, wantMig)
	}
}

func TestLoadGatewayYAML_metricsDisabled(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	gw := filepath.Join(dir, "gateway.yaml")
	raw := `gateway: { listen_port: 3000 }
paths: { tokens: "./t.yaml", routing_policy: "./r.yaml" }
routing: { fallback_chain: ["a/b"] }
metrics:
  enabled: false
`
	if err := os.WriteFile(gw, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := LoadGatewayYAML(gw, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.MetricsEnabled {
		t.Fatal("expected metrics disabled")
	}
}
