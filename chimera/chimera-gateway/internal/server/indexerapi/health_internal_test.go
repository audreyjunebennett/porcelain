package indexerapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	gwconfig "github.com/lynn/porcelain/chimera/internal/config"
)

func TestBuildInternalEmbeddingCheck_ok(t *testing.T) {
	embedSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embeddings" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		vec := make([]float32, 768)
		vec[0] = 0.5
		b, _ := json.Marshal(map[string]any{"data": []map[string]any{{"index": 0, "embedding": vec}}})
		_, _ = w.Write(b)
	}))
	t.Cleanup(embedSrv.Close)

	res := &gwconfig.Resolved{
		UpstreamBaseURL: "http://broker:8080",
		RAG: gwconfig.RAG{
			Enabled:          true,
			EmbeddingBaseURL: embedSrv.URL,
			EmbeddingPath:    "/v1/embeddings",
			EmbeddingModel:   "internal/nomic-embed-text",
			EmbeddingDim:     768,
		},
		InternalEmbedding: gwconfig.InternalEmbedding{
			Enabled:  true,
			Provider: "internal",
			Model:    "internal/nomic-embed-text",
			Dim:      768,
			BaseURL:  embedSrv.URL,
		},
	}

	out, ok := buildInternalEmbeddingCheck(context.Background(), nil, res.RAG.EmbeddingModel, res, map[string]any{
		"model": res.RAG.EmbeddingModel,
	})
	if !ok {
		t.Fatalf("expected ok: %+v", out)
	}
	if out["provider"] != "internal" || out["model_in_catalog"] != true {
		t.Fatalf("unexpected out: %+v", out)
	}
}

func TestBuildInternalEmbeddingCheck_down(t *testing.T) {
	res := &gwconfig.Resolved{
		UpstreamBaseURL: "http://broker:8080",
		RAG: gwconfig.RAG{
			Enabled:          true,
			EmbeddingBaseURL: "http://127.0.0.1:1",
			EmbeddingPath:    "/v1/embeddings",
			EmbeddingModel:   "internal/nomic-embed-text",
			EmbeddingDim:     768,
		},
		InternalEmbedding: gwconfig.InternalEmbedding{
			Enabled:  true,
			Provider: "internal",
		},
	}
	out, ok := buildInternalEmbeddingCheck(context.Background(), nil, res.RAG.EmbeddingModel, res, map[string]any{
		"model": res.RAG.EmbeddingModel,
	})
	if ok || out["reason_code"] != ReasonEmbedProviderDown {
		t.Fatalf("expected down: ok=%v out=%+v", ok, out)
	}
}
