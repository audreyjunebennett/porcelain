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
			http.Redirect(w, r, "/ui/desktop", http.StatusFound)
			return
		}
		http.Redirect(w, r, "/ui/login", http.StatusFound)
	})

	mux.HandleFunc("GET /ui/panel", h.RequireAuthPage(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/ui/logs?focus=admin", http.StatusFound)
	}))
	mux.HandleFunc("GET /ui/metrics", h.RequireAuthPage(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/ui/logs?focus=metrics", http.StatusFound)
	}))
	mux.HandleFunc("GET /ui/desktop", h.RequireAuthPage(ServeHTML("embedui/shell.html")))
	mux.HandleFunc("GET /ui/pwa", h.RequireAuthPage(ServeHTML("embedui/pwa.html")))
	mux.HandleFunc("GET /ui/indexer", h.RequireAuthPage(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/ui/logs", http.StatusFound)
	}))

	// Shared primitives (login/setup) — no session required; static CSS only.
	mux.HandleFunc("GET /ui/assets/ui.css", ServeAsset("embedui/ui.css", "text/css; charset=utf-8"))
	mux.HandleFunc("GET /ui/assets/theme-tokens.css", ServeAsset("embedui/theme-tokens.css", "text/css; charset=utf-8"))

	if h.Opts.LogStore == nil {
		return
	}
	mux.HandleFunc("GET /ui/assets/reload.svg", h.RequireAuthPage(ServeAsset("embedui/reload.svg", "image/svg+xml; charset=utf-8")))
	mux.HandleFunc("GET /ui/assets/logs.css", h.RequireAuthPage(ServeAsset("embedui/logs.css", "text/css; charset=utf-8")))
	mux.HandleFunc("GET /ui/assets/styles/", h.RequireAuthPage(ServePathPrefix("embedui/styles/", "/ui/assets/styles/", "text/css; charset=utf-8")))
	mux.HandleFunc("GET /ui/assets/ui/", h.RequireAuthPage(ServePathPrefix("embedui/ui/", "/ui/assets/ui/", "application/javascript; charset=utf-8")))
	mux.HandleFunc("GET /ui/assets/logs.js", h.RequireAuthPage(ServeAsset("embedui/logs_entry.js", "application/javascript; charset=utf-8")))
	mux.HandleFunc("GET /ui/assets/logs/main.js", h.RequireAuthPage(ServeAsset("embedui/logs_app.js", "application/javascript; charset=utf-8")))
	mux.HandleFunc("GET /ui/assets/logs/", h.RequireAuthPage(ServePathPrefix("embedui/logs/", "/ui/assets/logs/", "application/javascript; charset=utf-8")))
	mux.HandleFunc("GET /ui/logs", h.RequireAuthPage(ServeHTML("embedui/logs.html")))
}
