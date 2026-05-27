package state

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/brokeradmin"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/adminui/apirut"
)

func TestConfiguredProviderIDsResolved_ollamaOnlySkipsOtherProbes(t *testing.T) {
	brokeradmin.InvalidateProviderConfigIndex()
	chimeraBroker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/governance/providers":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"providers": []map[string]any{{"provider": "ollama"}},
				"count":     1,
			})
		case "/api/providers/ollama":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name":           "ollama",
				"keys":           []any{},
				"network_config": map[string]any{"base_url": "http://127.0.0.1:11434"},
			})
		case "/api/providers/groq", "/api/providers/gemini":
			t.Errorf("unexpected GET %s", r.URL.Path)
			http.NotFound(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(chimeraBroker.Close)

	ctx := context.Background()
	client := &brokeradmin.Client{BaseURL: chimeraBroker.URL}
	configured, listOK := brokeradmin.ListConfiguredProviders(ctx, client)
	names := apirut.ConfiguredProviderIDsResolved(ctx, client, configured, listOK)
	if len(names) != 1 || names[0] != "ollama" {
		t.Fatalf("names=%v", names)
	}
}
