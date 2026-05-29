package conversations

import (
	"net/http"

	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/adminui/handler"
)

// Register mounts conversation history operator API routes.
func Register(mux *http.ServeMux, h *handler.Handler) {
	if h == nil {
		return
	}
	mux.HandleFunc("GET /api/ui/conversations", h.RequireAuthJSON(func(w http.ResponseWriter, r *http.Request) {
		handleListGET(h, w, r)
	}))
	mux.HandleFunc("GET /api/ui/conversations/{conversation_id}", h.RequireAuthJSON(func(w http.ResponseWriter, r *http.Request) {
		handleDetailGET(h, w, r)
	}))
	mux.HandleFunc("PATCH /api/ui/conversations/{conversation_id}", h.RequireAuthJSON(func(w http.ResponseWriter, r *http.Request) {
		handlePatchTitle(h, w, r)
	}))
	mux.HandleFunc("POST /api/ui/conversations/{conversation_id}/flag", h.RequireAuthJSON(func(w http.ResponseWriter, r *http.Request) {
		handleFlagPOST(h, w, r)
	}))
	mux.HandleFunc("DELETE /api/ui/conversations/{conversation_id}", h.RequireAuthJSON(func(w http.ResponseWriter, r *http.Request) {
		handleDeleteDELETE(h, w, r)
	}))
}
