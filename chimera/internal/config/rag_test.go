package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRAG_Defaults_WhenDisabled(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	gw := filepath.Join(dir, "gateway.yaml")
	raw := `gateway: { listen_port: 3000 }
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
	if res.RAG.Enabled {
		t.Fatal("RAG should default disabled when block missing")
	}
}

func TestRAG_EnabledFillsDefaults(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	gw := filepath.Join(dir, "gateway.yaml")
	raw := `
gateway: { listen_port: 3000 }
paths: { tokens: "./t.yaml", routing_policy: "./r.yaml" }
routing: { fallback_chain: ["a/b"] }
rag:
  enabled: true
  qdrant:
    url: "http://127.0.0.1:6333"
  embedding:
    model: "text-embedding-3-small"
    dim: 1536
`
	if err := os.WriteFile(gw, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := LoadGatewayYAML(gw, nil)
	if err != nil {
		t.Fatal(err)
	}
	r := res.RAG
	if !r.Enabled {
		t.Fatal("expected RAG enabled")
	}
	if r.ChunkSize != 512 || r.ChunkOverlap != 128 {
		t.Fatalf("chunk defaults: size=%d overlap=%d", r.ChunkSize, r.ChunkOverlap)
	}
	if r.TopK != 8 {
		t.Fatalf("top_k default: %d", r.TopK)
	}
	if r.ScoreThreshold < 0.71 || r.ScoreThreshold > 0.73 {
		t.Fatalf("score_threshold default: %v", r.ScoreThreshold)
	}
	if r.QdrantURL != "http://127.0.0.1:6333" {
		t.Fatalf("qdrant url: %q", r.QdrantURL)
	}
	if !strings.HasSuffix(r.EmbeddingURL("http://up:8080"), "/v1/embeddings") {
		t.Fatalf("embedding url: %q", r.EmbeddingURL("http://up:8080"))
	}
}

func TestRAG_InvalidConfigDisables(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	gw := filepath.Join(dir, "gateway.yaml")
	raw := `
gateway: { listen_port: 3000 }
paths: { tokens: "./t.yaml", routing_policy: "./r.yaml" }
routing: { fallback_chain: ["a/b"] }
rag:
  enabled: true
  qdrant:
    url: "ftp://nope"
  embedding:
    model: "x"
    dim: 64
`
	if err := os.WriteFile(gw, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := LoadGatewayYAML(gw, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.RAG.Enabled {
		t.Fatal("invalid qdrant scheme should disable RAG")
	}
}

func TestRAG_EmbeddingURL_FallbackToUpstream(t *testing.T) {
	t.Parallel()
	r := RAG{Enabled: true, EmbeddingPath: "/v1/embeddings"}
	if got := r.EmbeddingURL("http://upstream:8080"); got != "http://upstream:8080/v1/embeddings" {
		t.Fatalf("got %q", got)
	}
}
