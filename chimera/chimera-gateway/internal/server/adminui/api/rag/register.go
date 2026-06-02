package rag

import (
	"net/http"

	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/adminui/handler"
)

// Register mounts operator RAG search API routes.
func Register(mux *http.ServeMux, h *handler.Handler) {
	if h == nil {
		return
	}
	mux.HandleFunc("POST /api/ui/rag/search", h.RequireAuthTenantJSON(func(w http.ResponseWriter, r *http.Request, tenantID string) {
		handleSearch(h, w, r, tenantID)
	}))
	mux.HandleFunc("GET /api/ui/rag/embedding", h.RequireAuthJSON(func(w http.ResponseWriter, r *http.Request) {
		handleEmbeddingGET(h, w, r)
	}))
	mux.HandleFunc("PUT /api/ui/rag/embedding", h.RequireAuthJSON(func(w http.ResponseWriter, r *http.Request) {
		handleEmbeddingPUT(h, w, r)
	}))
}
