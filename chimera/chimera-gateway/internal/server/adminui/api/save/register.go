package save

import (
	"net/http"

	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/adminui/apirut"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/adminui/handler"
)

// Register mounts provider save and logout routes.
func Register(mux *http.ServeMux, h *handler.Handler) {
	if h == nil {
		return
	}
	for _, prov := range apirut.KeyedProviderCatalogIDs() {
		provider := prov
		mux.HandleFunc("POST /api/ui/provider/"+provider+"/keys", h.RequireAuthJSON(saveAppendProviderKey(h, provider)))
		mux.HandleFunc("POST /api/ui/provider/"+provider+"/keys/delete", h.RequireAuthJSON(saveRemoveProviderKey(h, provider)))
	}
	mux.HandleFunc("POST /api/ui/provider/ollama/base_url", h.RequireAuthJSON(func(w http.ResponseWriter, r *http.Request) {
		saveOllamaBaseURL(h, w, r)
	}))
	mux.HandleFunc("POST /api/ui/logout", func(w http.ResponseWriter, r *http.Request) {
		handleLogoutPOST(h, w, r)
	})
}
