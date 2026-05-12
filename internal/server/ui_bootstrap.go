package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/lynn/claudia-gateway/internal/platform/requestid"
	"github.com/lynn/claudia-gateway/internal/tokens"
)

// NewBootstrapMux serves only the first-run setup surface (loopback-only in production).
func NewBootstrapMux(rt *Runtime, log *slog.Logger, overlay *StatusOverlay) http.Handler {
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
				"upstream": map[string]any{
					"ok":     false,
					"detail": "bootstrap mode — BiFrost not started",
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
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		b, err := adminEmbedUI.ReadFile(name)
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
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
			"listen":            listen,
			"virtual_model":     res.VirtualModelID,
			"semver":            res.Semver,
			"upstream_base_url": res.UpstreamBaseURL,
		},
		"upstream": map[string]any{
			"health_url": res.HealthUpstreamURL,
			"ok":         false,
			"status":     0,
			"detail":     "bootstrap mode — start BiFrost after creating tokens and restarting",
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
			"message":   "Copy this token now. Restart Claudia to start BiFrost and use the full admin UI.",
		})
	}
}
