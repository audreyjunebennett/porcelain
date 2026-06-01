package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadGatewayYAML_IndexerSupervised(t *testing.T) {
	dir := t.TempDir()
	gw := filepath.Join(dir, "gateway.yaml")
	raw := []byte(`gateway: { listen_port: 3000 }
paths: { tokens: "./t.yaml" }
vectorstore:
  url: "http://127.0.0.1:6333"
rag:
  enabled: true
  embedding:
    model: "text-embedding-3-small"
    dim: 1536
indexer:
  supervised:
    enabled: true
    config_path: "./idx/custom.yaml"
    start_when_rag_disabled: true
`)
	if err := os.WriteFile(gw, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := LoadGatewayYAML(gw, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !res.IndexerSupervisedEnabled {
		t.Fatal("expected supervised enabled")
	}
	if !res.IndexerSupervisedLogJSON {
		t.Fatal("expected log_json")
	}
	if !res.IndexerSupervisedStartWhenRAGDisabled {
		t.Fatal("expected start_when_rag_disabled")
	}
	want := filepath.Join(dir, "idx", "custom.yaml")
	if res.IndexerSupervisedConfigPath != want {
		t.Fatalf("config path: got %q want %q", res.IndexerSupervisedConfigPath, want)
	}
}

func TestLoadGatewayYAML_IndexerSupervised_logJSONDefaultTrue(t *testing.T) {
	dir := t.TempDir()
	gw := filepath.Join(dir, "gateway.yaml")
	raw := []byte(`gateway: { listen_port: 3000 }
paths: { tokens: "./t.yaml" }
indexer:
  supervised:
    enabled: true
`)
	if err := os.WriteFile(gw, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := LoadGatewayYAML(gw, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !res.IndexerSupervisedLogJSON {
		t.Fatal("expected log_json default true when omitted")
	}
}

func TestLoadGatewayYAML_IndexerSupervised_logJSONOptOut(t *testing.T) {
	dir := t.TempDir()
	gw := filepath.Join(dir, "gateway.yaml")
	raw := []byte(`gateway: { listen_port: 3000 }
paths: { tokens: "./t.yaml" }
indexer:
  supervised:
    enabled: true
    log_json: false
`)
	if err := os.WriteFile(gw, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := LoadGatewayYAML(gw, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.IndexerSupervisedLogJSON {
		t.Fatal("expected log_json false when set")
	}
}
