package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEnrichOllamaCatalog(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/show" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model_info":{"qwen.context_length":262144},"capabilities":["completion","tools"]}`))
	}))
	t.Cleanup(server.Close)

	row := map[string]any{"id": "ollama/qwen:8b"}
	warnings := enrichOllamaCatalog(context.Background(), server.Client(), server.URL, []any{row})
	if len(warnings) != 0 {
		t.Fatalf("warnings: %v", warnings)
	}
	if row["context_length"] != int64(262144) {
		t.Fatalf("context_length=%v", row["context_length"])
	}
	caps, ok := row["capabilities"].([]string)
	if !ok || len(caps) != 2 || caps[0] != "completion" {
		t.Fatalf("capabilities=%#v", row["capabilities"])
	}
}
