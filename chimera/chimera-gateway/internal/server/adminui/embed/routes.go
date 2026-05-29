package embed

import (
	"net/http"

	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/adminui/handler"
)

// Register mounts operator UI pages and static assets (session required except login).
func Register(mux *http.ServeMux, h *handler.Handler) {
	if h == nil {
		return
	}

	mux.HandleFunc("GET /ui", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if h.SessionOK(r) {
			h.RequireAuthPage(ServeHTML("embedui/index.html"))(w, r)
			return
		}
		http.Redirect(w, r, "/ui/login", http.StatusFound)
	})

	mux.HandleFunc("GET /ui/pwa", h.RequireAuthPage(ServeHTML("embedui/pwa.html")))
	mux.HandleFunc("GET /ui/chat", h.RequireAuthPage(ServeHTML("embedui/chat.html")))
	mux.HandleFunc("GET /ui/settings", h.RequireAuthPage(ServeHTML("embedui/settings.html")))
	mux.HandleFunc("GET /ui/settings/gallery", h.RequireAuthPage(ServeHTML("embedui/settings/gallery.html")))

	// Shared primitives (login/setup) — no session required; static CSS/JS only.
	mux.HandleFunc("GET /ui/assets/ui.css", ServeAsset("embedui/ui.css", "text/css; charset=utf-8"))
	mux.HandleFunc("GET /ui/assets/theme-tokens.css", ServeAsset("embedui/theme-tokens.css", "text/css; charset=utf-8"))
	mux.HandleFunc("GET /ui/assets/embed-theme.js", ServeAsset("embedui/embed-theme.js", "application/javascript; charset=utf-8"))

	mux.HandleFunc("GET /ui/assets/settings.css", h.RequireAuthPage(ServeAsset("embedui/settings.css", "text/css; charset=utf-8")))
	mux.HandleFunc("GET /ui/assets/styles/", h.RequireAuthPage(ServePathPrefix("embedui/styles/", "/ui/assets/styles/", "text/css; charset=utf-8")))
	mux.HandleFunc("GET /ui/assets/ui/", h.RequireAuthPage(ServePathPrefix("embedui/ui/", "/ui/assets/ui/", "application/javascript; charset=utf-8")))
	mux.HandleFunc("GET /ui/assets/settings.js", h.RequireAuthPage(ServeAsset("embedui/settings_entry.js", "application/javascript; charset=utf-8")))
	mux.HandleFunc("GET /ui/assets/settings/main.js", h.RequireAuthPage(ServeAsset("embedui/settings_app.js", "application/javascript; charset=utf-8")))
	mux.HandleFunc("GET /ui/assets/settings/", h.RequireAuthPage(ServePathPrefix("embedui/settings/", "/ui/assets/settings/", "application/javascript; charset=utf-8")))
	mux.HandleFunc("GET /ui/assets/gallery/", h.RequireAuthPage(ServePathPrefix("embedui/gallery/", "/ui/assets/gallery/", "application/javascript; charset=utf-8")))
	mux.HandleFunc("GET /ui/assets/chat/", h.RequireAuthPage(ServePathPrefix("embedui/chat/", "/ui/assets/chat/", "application/javascript; charset=utf-8")))
	mux.HandleFunc("GET /ui/assets/shell/", h.RequireAuthPage(ServePathPrefix("embedui/shell/", "/ui/assets/shell/", "application/javascript; charset=utf-8")))
}
