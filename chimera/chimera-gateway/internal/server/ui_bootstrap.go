package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/adminui"
	"github.com/lynn/porcelain/chimera/internal/platform/requestid"
	"github.com/lynn/porcelain/chimera/internal/tokens"
)

// NewBootstrapMux serves only the first-run setup surface (loopback-only in production).
func NewBootstrapMux(rt *Runtime, log *slog.Logger, overlay *StatusOverlay) http.Handler {
	configureAdminUIListenForEmbed(rt, overlay)
	mux := http.NewServeMux()

	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		http.Redirect(w, r, "/ui/setup", http.StatusFound)
	})

	mux.HandleFunc("GET /ui", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		http.Redirect(w, r, "/ui/setup", http.StatusFound)
	})

	mux.HandleFunc("GET /ui/setup", serveBootstrapHTML("embedui/setup.html"))
	mux.HandleFunc("GET /ui/assets/theme-tokens.css", serveBootstrapAsset("embedui/theme-tokens.css", "text/css; charset=utf-8"))
	mux.HandleFunc("GET /ui/assets/ui.css", serveBootstrapAsset("embedui/ui.css", "text/css; charset=utf-8"))
	mux.HandleFunc("GET /ui/login", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		http.Redirect(w, r, "/ui/setup", http.StatusFound)
	})
	mux.HandleFunc("GET /ui/panel", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		http.Redirect(w, r, "/ui/setup", http.StatusFound)
	})

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":    "ok",
			"bootstrap": true,
			"checks": map[string]any{
				"broker": map[string]any{
					"ok":     false,
					"detail": "bootstrap mode — chimera-broker not started",
				},
			},
		})
	})

	mux.HandleFunc("GET /status", func(w http.ResponseWriter, r *http.Request) {
		handleBootstrapStatus(w, r, rt, log, overlay)
	})

	mux.HandleFunc("POST /api/ui/setup/token", handleSetupTokenPOST(rt, log))

	return requestid.Middleware(loggingMiddleware(log, mux))
}

func serveBootstrapHTML(name string) http.HandlerFunc {
	return serveBootstrapAsset(name, "text/html; charset=utf-8")
}

func serveBootstrapAsset(name, contentType string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		b, err := adminui.ReadEmbedFile(name)
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		if contentType != "" {
			w.Header().Set("Content-Type", contentType)
		}
		_, _ = w.Write(b)
	}
}

func handleBootstrapStatus(w http.ResponseWriter, r *http.Request, rt *Runtime, log *slog.Logger, overlay *StatusOverlay) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	rt.Sync()
	res, _, _ := rt.Snapshot()
	listen := res.ListenAddr()
	if overlay != nil && overlay.EffectiveListen != "" {
		listen = overlay.EffectiveListen
	}
	body := map[string]any{
		"bootstrap": true,
		"supervisor": map[string]any{
			"active": false,
		},
		"gateway": map[string]any{
			"listen":          listen,
			"virtual_model":   res.VirtualModelID,
			"semver":          res.Semver,
			"broker_base_url": res.UpstreamBaseURL,
		},
		"broker": map[string]any{
			"health_url": res.HealthUpstreamURL,
			"base_url":   res.UpstreamBaseURL,
			"ok":         false,
			"status":     0,
			"detail":     "bootstrap mode — start chimera-broker after creating tokens and restarting",
		},
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(body)
}

func handleSetupTokenPOST(rt *Runtime, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !BootstrapMode(rt) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": "setup already completed — restart the gateway"})
			return
		}
		var body struct {
			Label string `json:"label"`
		}
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
		if err := dec.Decode(&body); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": "invalid json"})
			return
		}
		label := strings.TrimSpace(body.Label)
		if label == "" {
			label = "default"
		}
		_, tokStore, _ := rt.Snapshot()
		if tokStore == nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		path := tokStore.Path()
		plain, tenant, err := tokens.AppendToken(path, label)
		if err != nil {
			if log != nil {
				log.Error("setup append token", "msg", "gateway.auth.append_failed", "surface", "bootstrap", "err", err)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": "could not save token", "detail": err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":        true,
			"token":     plain,
			"tenant_id": tenant,
			"label":     label,
			"message":   "Copy this token now. Restart Chimera to start chimera-broker and use the full admin UI.",
		})
	}
}
