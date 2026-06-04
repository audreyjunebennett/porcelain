package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestKnownEmbeddingDim(t *testing.T) {
	t.Parallel()
	cases := []struct {
		model string
		want  int
		ok    bool
	}{
		{"ollama/nomic-embed-text:latest", 768, true},
		{"groq/nomic-embed-text-v1", 768, true},
		{"openai/text-embedding-3-small", 1536, true},
		{"text-embedding-3-large", 3072, true},
		{"groq/llama3", 0, false},
	}
	for _, tc := range cases {
		got, ok := KnownEmbeddingDim(tc.model)
		if ok != tc.ok || got != tc.want {
			t.Fatalf("KnownEmbeddingDim(%q) = (%d, %v), want (%d, %v)", tc.model, got, ok, tc.want, tc.ok)
		}
	}
}

func TestPatchGatewayYAMLBytesWithEmbeddingModel(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	gw := filepath.Join(dir, "gateway.yaml")
	raw := `gateway: { listen_port: 3000 }
paths: { tokens: "./t.yaml" }
vectorstore:
  url: "http://127.0.0.1:6333"
rag:
  enabled: true
  embedding:
    model: "old-model"
    dim: 512
`
	if err := os.WriteFile(gw, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := PatchGatewayYAMLBytesWithEmbeddingModel([]byte(raw), "ollama/nomic-embed-text:latest", 768)
	if err != nil {
		t.Fatal(err)
	}
	patched := filepath.Join(dir, "patched.yaml")
	if err := os.WriteFile(patched, out, 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := LoadGatewayYAML(patched, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.RAG.EmbeddingModel != "ollama/nomic-embed-text:latest" {
		t.Fatalf("model=%q", res.RAG.EmbeddingModel)
	}
	if res.RAG.EmbeddingDim != 768 {
		t.Fatalf("dim=%d", res.RAG.EmbeddingDim)
	}
	if err := WriteGatewayEmbeddingModel(gw, "groq/custom-embed", 1024); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadGatewayYAML(gw, nil)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.RAG.EmbeddingModel != "groq/custom-embed" || loaded.RAG.EmbeddingDim != 1024 {
		t.Fatalf("loaded: model=%q dim=%d", loaded.RAG.EmbeddingModel, loaded.RAG.EmbeddingDim)
	}
}

func TestPatchGatewayYAMLBytesWithEmbeddingModel_rejectsEmpty(t *testing.T) {
	t.Parallel()
	_, err := PatchGatewayYAMLBytesWithEmbeddingModel([]byte("rag: {}\n"), "  ", 768)
	if err == nil || !strings.Contains(err.Error(), "required") {
		t.Fatalf("err=%v", err)
	}
}
