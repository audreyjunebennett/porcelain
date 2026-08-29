package indexerapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	gwconfig "github.com/lynn/porcelain/chimera/internal/config"
)

func TestBuildInternalEmbeddingCheck(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "" {
			t.Errorf("internal probe Authorization = %q, want empty", got)
		}
		vector := make([]float32, 3)
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{{"index": 0, "embedding": vector}}})
	}))
	t.Cleanup(server.Close)
	res := &gwconfig.Resolved{
		RAG:               gwconfig.RAG{Enabled: true, EmbeddingBaseURL: server.URL, EmbeddingPath: "/v1/embeddings", EmbeddingModel: "internal/nomic-embed-text", EmbeddingDim: 3},
		InternalEmbedding: gwconfig.InternalEmbedding{Enabled: true, Provider: "internal", Model: "internal/nomic-embed-text", Dim: 3, BaseURL: server.URL},
	}
	out, ok := buildInternalEmbeddingCheck(context.Background(), nil, res.RAG.EmbeddingModel, res, map[string]any{"ok": true, "status": "ok"})
	if !ok || out["provider_state"] != "up" || out["model_in_catalog"] != true {
		t.Fatalf("out=%+v ok=%v", out, ok)
	}
}

func TestBuildInternalEmbeddingCheckDimensionMismatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{{"index": 0, "embedding": []float32{1}}}})
	}))
	t.Cleanup(server.Close)
	res := &gwconfig.Resolved{
		RAG:               gwconfig.RAG{Enabled: true, EmbeddingBaseURL: server.URL, EmbeddingPath: "/v1/embeddings", EmbeddingModel: "internal/nomic-embed-text", EmbeddingDim: 3},
		InternalEmbedding: gwconfig.InternalEmbedding{Enabled: true, Provider: "internal", Model: "internal/nomic-embed-text", Dim: 3, BaseURL: server.URL},
	}
	out, ok := buildInternalEmbeddingCheck(context.Background(), nil, res.RAG.EmbeddingModel, res, map[string]any{"ok": true, "status": "ok"})
	if ok || out["reason_code"] != ReasonEmbedProviderDown {
		t.Fatalf("out=%+v ok=%v", out, ok)
	}
}
