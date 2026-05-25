package apirut

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/brokeradmin"
)

func TestConfiguredProviderIDsResolved_fallsBackToProbe(t *testing.T) {
	brokeradmin.InvalidateProviderConfigIndex()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/governance/providers":
			_ = json.NewEncoder(w).Encode(map[string]any{"count": 0, "providers": nil})
		case "/api/providers/ollama":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name":           "ollama",
				"keys":           []any{},
				"network_config": map[string]any{"base_url": "http://127.0.0.1:11434"},
			})
		case "/api/providers/groq":
			w.WriteHeader(http.StatusNotFound)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	client := &brokeradmin.Client{BaseURL: srv.URL}
	configured, listOK := brokeradmin.ListConfiguredProviders(context.Background(), client)
	if listOK {
		t.Fatal("expected empty governance list to be unavailable")
	}
	got := ConfiguredProviderIDsResolved(context.Background(), client, configured, listOK)
	if len(got) != 1 || got[0] != "ollama" {
		t.Fatalf("got %v want [ollama]", got)
	}
}

func TestConfiguredProviderIDsFromGovernance(t *testing.T) {
	got := ConfiguredProviderIDsFromGovernance(map[string]struct{}{
		"ollama": {},
		"groq":   {},
	}, true)
	if len(got) != 2 || got[0] != "groq" || got[1] != "ollama" {
		t.Fatalf("got %v", got)
	}
	if len(ConfiguredProviderIDsFromGovernance(nil, false)) != 0 {
		t.Fatal("expected empty when list unavailable")
	}
}

func TestBrokerProviderNames_matchesCatalog(t *testing.T) {
	if len(BrokerProviderNames) != 3 {
		t.Fatalf("BrokerProviderNames=%v", BrokerProviderNames)
	}
}

func TestKeyedProviderCatalogIDs(t *testing.T) {
	got := KeyedProviderCatalogIDs()
	if len(got) != 2 || got[0] != "groq" || got[1] != "gemini" {
		t.Fatalf("got %v", got)
	}
	if !IsKeyedCatalogProvider("groq") || IsKeyedCatalogProvider("ollama") {
		t.Fatal("keyed catalog check")
	}
}

func TestBrokerProviderNamesForProbes_ollamaOnly(t *testing.T) {
	brokeradmin.InvalidateProviderConfigIndex()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/governance/providers":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"providers": []map[string]any{{"provider": "ollama"}},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	client := &brokeradmin.Client{BaseURL: srv.URL}
	got := BrokerProviderNamesForProbes(context.Background(), client)
	if len(got) != 1 || got[0] != "ollama" {
		t.Fatalf("got %v", got)
	}
}

func TestLookupProviderCatalogEntry(t *testing.T) {
	e := LookupProviderCatalogEntry("gemini")
	if e == nil || e.Title != "Gemini" {
		t.Fatalf("lookup: %+v", e)
	}
	if LookupProviderCatalogEntry("unknown") != nil {
		t.Fatal("expected nil for unknown id")
	}
}
