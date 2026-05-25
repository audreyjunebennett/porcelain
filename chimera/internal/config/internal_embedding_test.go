package config

import (
	"strings"
	"testing"
)

func TestInternalEmbedding_effectiveDefaults(t *testing.T) {
	enabled := true
	ie := internalEmbeddingDoc{Enabled: &enabled}.effective()
	if !ie.Enabled {
		t.Fatal("expected enabled")
	}
	if ie.Provider != "internal" {
		t.Fatalf("provider: %q", ie.Provider)
	}
	if ie.Model != "internal/nomic-embed-text" {
		t.Fatalf("model: %q", ie.Model)
	}
	if ie.Dim != 768 {
		t.Fatalf("dim: %d", ie.Dim)
	}
	if ie.BaseURL != "http://127.0.0.1:8090" {
		t.Fatalf("base_url: %q", ie.BaseURL)
	}
}

func TestApplyInternalEmbeddingToRAG_replacesOllama(t *testing.T) {
	rag := RAG{
		Enabled:          true,
		EmbeddingBaseURL: "",
		EmbeddingModel:   "ollama/nomic-embed-text:latest",
		EmbeddingDim:     768,
	}
	ie := internalEmbeddingDoc{Enabled: ptrBool(true)}.effective()
	applyInternalEmbeddingToRAG(&rag, ie, "http://127.0.0.1:8080")
	if rag.EmbeddingBaseURL != ie.BaseURL {
		t.Fatalf("base_url: got %q want %q", rag.EmbeddingBaseURL, ie.BaseURL)
	}
	if rag.EmbeddingModel != ie.Model {
		t.Fatalf("model: got %q want %q", rag.EmbeddingModel, ie.Model)
	}
}

func TestUsesInternalProvider(t *testing.T) {
	ie := internalEmbeddingDoc{Enabled: ptrBool(true)}.effective()
	if !UsesInternalProvider("internal/nomic-embed-text", ie) {
		t.Fatal("expected internal provider match")
	}
	if UsesInternalProvider("ollama/nomic-embed-text:latest", ie) {
		t.Fatal("ollama should not match internal provider")
	}
}

func TestInternalEmbedding_Validate(t *testing.T) {
	ie := internalEmbeddingDoc{Enabled: ptrBool(true), BaseURL: "ftp://bad"}.effective()
	if err := ie.Validate(); err == nil || !strings.Contains(err.Error(), "base_url") {
		t.Fatalf("validate: %v", err)
	}
}

func ptrBool(v bool) *bool { return &v }
