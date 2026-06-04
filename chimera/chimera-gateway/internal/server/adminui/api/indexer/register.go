package indexer

import (
	"net/http"

	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/adminui/handler"
)

// Register mounts indexer workspace operator API routes.
func Register(mux *http.ServeMux, h *handler.Handler) {
	if h == nil {
		return
	}
	mux.HandleFunc("GET /api/ui/indexer/config", h.RequireAuthJSON(func(w http.ResponseWriter, r *http.Request) {
		handleIndexerConfigGET(h, w, r)
	}))
	mux.HandleFunc("PUT /api/ui/indexer/config", h.RequireAuthJSON(func(w http.ResponseWriter, r *http.Request) {
		handleIndexerConfigPUT(h, w, r)
	}))
	mux.HandleFunc("GET /api/ui/indexer/workspaces", h.RequireAuthJSON(func(w http.ResponseWriter, r *http.Request) {
		handleIndexerWorkspacesGET(h, w, r)
	}))
	mux.HandleFunc("POST /api/ui/indexer/workspaces", h.RequireAuthJSON(func(w http.ResponseWriter, r *http.Request) {
		handleIndexerWorkspacesPOST(h, w, r)
	}))
	mux.HandleFunc("PUT /api/ui/indexer/workspaces/{id}", h.RequireAuthJSON(func(w http.ResponseWriter, r *http.Request) {
		handleIndexerWorkspacePUT(h, w, r)
	}))
	mux.HandleFunc("DELETE /api/ui/indexer/workspaces/{id}", h.RequireAuthJSON(func(w http.ResponseWriter, r *http.Request) {
		handleIndexerWorkspaceDELETE(h, w, r)
	}))
	mux.HandleFunc("POST /api/ui/indexer/workspaces/{id}/paths", h.RequireAuthJSON(func(w http.ResponseWriter, r *http.Request) {
		handleIndexerWorkspacePathPOST(h, w, r)
	}))
	mux.HandleFunc("PUT /api/ui/indexer/workspace-paths/{pathid}", h.RequireAuthJSON(func(w http.ResponseWriter, r *http.Request) {
		handleIndexerWorkspacePathPUT(h, w, r)
	}))
	mux.HandleFunc("DELETE /api/ui/indexer/workspace-paths/{pathid}", h.RequireAuthJSON(func(w http.ResponseWriter, r *http.Request) {
		handleIndexerWorkspacePathDELETE(h, w, r)
	}))
	mux.HandleFunc("POST /api/ui/indexer/workspaces/{id}/reindex", h.RequireAuthJSON(func(w http.ResponseWriter, r *http.Request) {
		handleIndexerWorkspaceReindexPOST(h, w, r)
	}))
	mux.HandleFunc("POST /api/ui/indexer/reindex-all", h.RequireAuthJSON(func(w http.ResponseWriter, r *http.Request) {
		handleIndexerReindexAllPOST(h, w, r)
	}))
	mux.HandleFunc("GET /api/ui/indexer/corpus/stale", h.RequireAuthJSON(func(w http.ResponseWriter, r *http.Request) {
		handleIndexerCorpusStaleGET(h, w, r)
	}))
}
