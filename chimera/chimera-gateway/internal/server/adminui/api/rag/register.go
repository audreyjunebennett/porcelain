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
}
