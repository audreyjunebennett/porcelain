package indexer

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/adminui/handler"
)

func handleIndexerWorkspaceReindexPOST(h *handler.Handler, w http.ResponseWriter, r *http.Request) {
	st := h.RT.OperatorStore()
	if st == nil {
		http.Error(w, "operator store unavailable", http.StatusServiceUnavailable)
		return
	}
	idStr := r.PathValue("id")
	wsID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || wsID <= 0 {
		http.Error(w, "invalid workspace id", http.StatusBadRequest)
		return
	}
	tenant := operatorIndexerTenantID()
	gen, err := st.BumpReindexGeneration(r.Context(), tenant, wsID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if h.Log != nil {
		h.Log.Info("workspace reindex requested",
			"msg", "gateway.operator.workspace.reindex_requested",
			"workspace_id", wsID,
			"reindex_generation", gen,
		)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"workspace_id":         wsID,
		"reindex_generation":   gen,
		"indexer_poll_seconds": 30,
	})
}

func handleIndexerReindexAllPOST(h *handler.Handler, w http.ResponseWriter, r *http.Request) {
	st := h.RT.OperatorStore()
	if st == nil {
		http.Error(w, "operator store unavailable", http.StatusServiceUnavailable)
		return
	}
	tenant := operatorIndexerTenantID()
	n, err := st.BumpAllReindexGenerations(r.Context(), tenant)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if h.Log != nil {
		h.Log.Info("all workspaces reindex requested",
			"msg", "gateway.operator.workspace.reindex_all_requested",
			"workspaces", n,
		)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"workspaces_bumped":    n,
		"indexer_poll_seconds": 30,
	})
}
