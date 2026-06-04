package indexerapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/corpusstale"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/gwhttp"
	gruntime "github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/runtime"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/scope"
	"github.com/lynn/porcelain/chimera/internal/config"
)

// HandleCorpusStaleGET returns indexer-reported stale sources for the scoped corpus.
func HandleCorpusStaleGET(w http.ResponseWriter, r *http.Request, rt *gruntime.Runtime, store *corpusstale.Store) {
	rt.Sync()
	res, tokStore := rt.Snapshot()
	token := gwhttp.BearerToken(r.Header.Get("Authorization"))
	sess := tokStore.Validate(token)
	if token == "" || sess == nil {
		gwhttp.WriteJSONError(w, http.StatusUnauthorized, "Unauthorized", "invalid_api_key")
		return
	}
	if !res.RAG.Enabled || rt.RAG() == nil {
		gwhttp.WriteJSONError(w, http.StatusServiceUnavailable, "RAG is not enabled", "gateway_config")
		return
	}
	if store == nil {
		gwhttp.WriteJSONError(w, http.StatusServiceUnavailable, "stale store unavailable", "gateway_config")
		return
	}
	proj := scope.ResolveProject(r.Header.Get(scope.HeaderProject), res.RAG.DefaultProject)
	flav := scope.ResolveFlavor(r.Header.Get(scope.HeaderFlavor), res.RAG.DefaultFlavor)
	entries := store.ListScope(sess.TenantID, proj, flav)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"object":         "indexer.corpus.stale",
		"tenant_id":      sess.TenantID,
		"project_id":     proj,
		"flavor_id":      flav,
		"coherence_mode": res.RAG.CoherenceMode,
		"entries":        entries,
	})
}

// HandleCorpusStalePUT replaces stale entries for a scope (indexer → gateway).
func HandleCorpusStalePUT(w http.ResponseWriter, r *http.Request, rt *gruntime.Runtime, store *corpusstale.Store) {
	rt.Sync()
	res, tokStore := rt.Snapshot()
	token := gwhttp.BearerToken(r.Header.Get("Authorization"))
	sess := tokStore.Validate(token)
	if token == "" || sess == nil {
		gwhttp.WriteJSONError(w, http.StatusUnauthorized, "Unauthorized", "invalid_api_key")
		return
	}
	if !res.RAG.Enabled || rt.RAG() == nil {
		gwhttp.WriteJSONError(w, http.StatusServiceUnavailable, "RAG is not enabled", "gateway_config")
		return
	}
	if store == nil {
		gwhttp.WriteJSONError(w, http.StatusServiceUnavailable, "stale store unavailable", "gateway_config")
		return
	}
	var body struct {
		Entries []corpusstale.Entry `json:"entries"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		gwhttp.WriteJSONError(w, http.StatusBadRequest, "invalid JSON", "invalid_request")
		return
	}
	proj := scope.ResolveProject(r.Header.Get(scope.HeaderProject), res.RAG.DefaultProject)
	flav := scope.ResolveFlavor(r.Header.Get(scope.HeaderFlavor), res.RAG.DefaultFlavor)
	store.ReplaceScope(sess.TenantID, proj, flav, body.Entries)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"object":     "indexer.corpus.stale.updated",
		"tenant_id":  sess.TenantID,
		"project_id": proj,
		"flavor_id":  flav,
		"count":      len(body.Entries),
	})
}

// CoherenceMode returns normalized coherence mode from resolved config.
func CoherenceMode(res *config.Resolved) string {
	if res == nil {
		return "warn"
	}
	m := strings.ToLower(strings.TrimSpace(res.RAG.CoherenceMode))
	switch m {
	case "off", "warn", "strict":
		return m
	default:
		return "warn"
	}
}
