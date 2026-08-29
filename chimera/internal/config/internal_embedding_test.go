package config

import (
	"os"
	"path/filepath"
	"testing"
)

func boolPtr(v bool) *bool { return &v }

func TestInternalEmbeddingDefaultsOffAndResolvesPaths(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	ie := (internalEmbeddingDoc{}).effective(dir)
	if ie.Enabled {
		t.Fatal("internal embedding must default off")
	}
	if ie.Model != "internal/nomic-embed-text" || ie.Dim != 768 {
		t.Fatalf("model=%q dim=%d", ie.Model, ie.Dim)
	}
	wantModel := filepath.Clean(filepath.Join(dir, "..", "data", "embedding", "models", "nomic-embed-text.gguf"))
	if ie.ModelPath != wantModel {
		t.Fatalf("model path=%q want %q", ie.ModelPath, wantModel)
	}
}

func TestLoadGatewayYAMLInternalEmbeddingEnabled(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "gateway.yaml")
	raw := `gateway:
  listen_port: 3000
vectorstore:
  url: "http://127.0.0.1:6333"
rag:
  enabled: true
  embedding:
    model: "ollama/nomic-embed-text:latest"
    dim: 768
internal_embedding:
  enabled: true
  model_path: "../models/nomic.gguf"
  base_url: "http://127.0.0.1:18090"
`
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := LoadGatewayYAML(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !res.InternalEmbedding.Enabled || res.RAG.EmbeddingModel != "internal/nomic-embed-text" || res.RAG.EmbeddingURL(res.UpstreamBaseURL) != "http://127.0.0.1:18090/v1/embeddings" {
		t.Fatalf("resolved=%+v rag=%+v", res.InternalEmbedding, res.RAG)
	}
	wantPath := filepath.Clean(filepath.Join(dir, "..", "models", "nomic.gguf"))
	if res.InternalEmbedding.ModelPath != wantPath {
		t.Fatalf("model path=%q want %q", res.InternalEmbedding.ModelPath, wantPath)
	}
}

func TestInternalEmbeddingEnabledRewiresRAG(t *testing.T) {
	t.Parallel()
	ie := (internalEmbeddingDoc{Enabled: boolPtr(true)}).effective(t.TempDir())
	rag := RAG{
		Enabled:          true,
		EmbeddingBaseURL: "http://broker:8080",
		EmbeddingModel:   "ollama/nomic-embed-text:latest",
		EmbeddingDim:     768,
	}
	applyInternalEmbeddingToRAG(&rag, ie)
	if rag.EmbeddingBaseURL != "http://127.0.0.1:8090" || rag.EmbeddingModel != ie.Model || rag.EmbeddingDim != ie.Dim {
		t.Fatalf("rag not rewired: %+v", rag)
	}
}

func TestInternalEmbeddingEndpoint(t *testing.T) {
	t.Parallel()
	ie := InternalEmbedding{BaseURL: "http://localhost:18090"}
	if got, err := ie.Endpoint(); err != nil || got != "localhost:18090" {
		t.Fatalf("endpoint=%q err=%v", got, err)
	}
}

func TestUsesInternalProvider(t *testing.T) {
	t.Parallel()
	ie := InternalEmbedding{Enabled: true, Provider: "internal", Model: "internal/nomic-embed-text"}
	if !UsesInternalProvider("internal/nomic-embed-text", ie) {
		t.Fatal("expected internal model match")
	}
	if UsesInternalProvider("ollama/nomic-embed-text:latest", ie) {
		t.Fatal("ollama model must not match")
	}
}
