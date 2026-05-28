package providers

import (
	"net/http"

	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/adminui/handler"
)

// Register mounts /api/ui/chimera-broker/providers.
func Register(mux *http.ServeMux, h *handler.Handler) {
	if h == nil {
		return
	}
	mux.HandleFunc("GET /api/ui/chimera-broker/providers", h.RequireAuthJSON(func(w http.ResponseWriter, r *http.Request) {
		handleChimeraBrokerProviderHealth(h, w, r)
	}))
	mux.HandleFunc("GET /api/ui/providers/catalog", h.RequireAuthJSON(func(w http.ResponseWriter, r *http.Request) {
		handleProviderCatalog(h, w, r)
	}))
	mux.HandleFunc("GET /api/ui/providers/{provider_id}/models", h.RequireAuthJSON(func(w http.ResponseWriter, r *http.Request) {
		handleProviderModelsGET(h, w, r)
	}))
	mux.HandleFunc("PUT /api/ui/providers/{provider_id}/models", h.RequireAuthJSON(func(w http.ResponseWriter, r *http.Request) {
		handleProviderModelsPUT(h, w, r)
	}))
	mux.HandleFunc("POST /api/ui/providers/{provider_id}/models/apply-free-tier", h.RequireAuthJSON(func(w http.ResponseWriter, r *http.Request) {
		handleProviderModelsApplyFreeTierPOST(h, w, r)
	}))
}
