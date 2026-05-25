package apirut

import (
	"context"
	"strings"

	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/brokeradmin"
	"github.com/lynn/porcelain/internal/operatorapi"
)

// ProviderCatalog is the operator UI catalog of addable upstream providers.
var ProviderCatalog = []operatorapi.ProviderCatalogEntry{
	{
		ID:             "groq",
		Title:          "Groq",
		Avatar:         "Gq",
		Subtitle:       "LPU inference provider with key management.",
		Kind:           "keyed",
		KeyPlaceholder: "gsk-…",
	},
	{
		ID:             "gemini",
		Title:          "Gemini",
		Avatar:         "Gm",
		Subtitle:       "Google Gemini provider with key management.",
		Kind:           "keyed",
		KeyPlaceholder: "AIza…",
	},
	{
		ID:       "ollama",
		Title:    "Ollama",
		Avatar:   "Ol",
		Subtitle: "Local/remote Ollama endpoint for chat and embeddings.",
		Kind:     "ollama",
	},
}

// ProviderCatalogIDs returns catalog provider ids in display order.
func ProviderCatalogIDs() []string {
	out := make([]string, len(ProviderCatalog))
	for i, e := range ProviderCatalog {
		out[i] = e.ID
	}
	return out
}

// KeyedProviderCatalogIDs returns catalog ids with kind "keyed" (API-key providers).
func KeyedProviderCatalogIDs() []string {
	out := make([]string, 0, len(ProviderCatalog))
	for _, e := range ProviderCatalog {
		if e.Kind == "keyed" {
			out = append(out, e.ID)
		}
	}
	return out
}

// IsKeyedCatalogProvider is true when id is a catalog keyed provider.
func IsKeyedCatalogProvider(id string) bool {
	e := LookupProviderCatalogEntry(id)
	return e != nil && e.Kind == "keyed"
}

// LookupProviderCatalogEntry returns a catalog entry by id, or nil when unknown.
func LookupProviderCatalogEntry(id string) *operatorapi.ProviderCatalogEntry {
	name := strings.ToLower(strings.TrimSpace(id))
	for i := range ProviderCatalog {
		if ProviderCatalog[i].ID == name {
			e := ProviderCatalog[i]
			return &e
		}
	}
	return nil
}

// ConfiguredProviderIDsFromGovernance returns catalog ids registered in chimera-broker config.
func ConfiguredProviderIDsFromGovernance(configured map[string]struct{}, listOK bool) []string {
	if !listOK || len(configured) == 0 {
		return nil
	}
	out := make([]string, 0, len(ProviderCatalog))
	for _, e := range ProviderCatalog {
		if _, ok := configured[e.ID]; ok {
			out = append(out, e.ID)
		}
	}
	return out
}

// ConfiguredProviderIDsResolved returns visible catalog ids for the settings UI.
// It prefers GET /api/governance/providers; when that list is empty or unavailable it
// falls back to per-provider GET probes (same path as GET /api/ui/state).
func ConfiguredProviderIDsResolved(ctx context.Context, client *brokeradmin.Client, configured map[string]struct{}, listOK bool) []string {
	if ids := ConfiguredProviderIDsFromGovernance(configured, listOK); len(ids) > 0 {
		return ids
	}
	if client == nil || strings.TrimSpace(client.BaseURL) == "" {
		return nil
	}
	out := make([]string, 0, len(ProviderCatalog))
	for _, e := range ProviderCatalog {
		body, st, err, _ := brokeradmin.GetProviderForProbeWithList(ctx, client, e.ID, configured, listOK)
		if err != nil {
			continue
		}
		if !brokeradmin.IsProviderMissingGET(st, body) {
			out = append(out, e.ID)
		}
	}
	return out
}
