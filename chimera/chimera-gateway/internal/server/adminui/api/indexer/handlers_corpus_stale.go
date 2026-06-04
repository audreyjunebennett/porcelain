package indexer

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/adminui/handler"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/indexerapi"
)

func handleIndexerCorpusStaleGET(h *handler.Handler, w http.ResponseWriter, r *http.Request) {
	rt := h.RT
	if rt == nil {
		http.Error(w, "runtime unavailable", http.StatusServiceUnavailable)
		return
	}
	res, _ := rt.Snapshot()
	store := rt.CorpusStaleStore()
	if store == nil {
		http.Error(w, "stale store unavailable", http.StatusServiceUnavailable)
		return
	}
	proj := strings.TrimSpace(r.URL.Query().Get("project_id"))
	flav := strings.TrimSpace(r.URL.Query().Get("flavor_id"))
	if proj == "" {
		proj = res.RAG.DefaultProject
	}
	if flav == "" {
		flav = res.RAG.DefaultFlavor
	}
	entries := store.ListScope(operatorIndexerTenantID(), proj, flav)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"object":         "indexer.corpus.stale",
		"coherence_mode": indexerapi.CoherenceMode(res),
		"project_id":     proj,
		"flavor_id":      flav,
		"entries":        entries,
	})
}
