package server

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/lynn/porcelain/chimera/internal/upstream"
)

// StatusOverlay configures GET /status (operator + GUI). Pass nil from tests; production
// should set EffectiveListen to the address passed to http.ListenAndServe.
type StatusOverlay struct {
	EffectiveListen string
	Supervisor      *SupervisorInfo
}

// SupervisorInfo describes subprocesses when started via "chimera serve".
type SupervisorInfo struct {
	BifrostListen     string
	QdrantSupervised  bool
	QdrantHTTP        string // host:port for HTTP API /readyz when supervised
	IndexerSupervised bool
	IndexerConfigPath string // single --config path when supervised (may be empty if not started)
}

func handleStatus(w http.ResponseWriter, r *http.Request, rt *Runtime, log *slog.Logger, overlay *StatusOverlay) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	rt.Sync()
	res, _, _ := rt.Snapshot()
	apiKey := rt.UpstreamAPIKey()
	ctx := r.Context()
	ok, st, detail := upstream.ProbeHealth(ctx, res.HealthUpstreamURL, apiKey, healthTimeout(res), log)

	listen := res.ListenAddr()
	if overlay != nil && overlay.EffectiveListen != "" {
		listen = overlay.EffectiveListen
	}

	sup := map[string]any{"active": false}
	if overlay != nil && overlay.Supervisor != nil {
		s := overlay.Supervisor
		sup = map[string]any{
			"active":              true,
			"chimera_broker_listen":      s.BifrostListen,
			"qdrant_supervised":   s.QdrantSupervised,
			"qdrant_http":         s.QdrantHTTP,
			"indexer_supervised":  s.IndexerSupervised,
			"indexer_config_path": s.IndexerConfigPath,
		}
	}

	body := map[string]any{
		"supervisor": sup,
		"gateway": map[string]any{
			"listen":            listen,
			"virtual_model":     res.VirtualModelID,
			"semver":            res.Semver,
			"upstream_base_url": res.UpstreamBaseURL,
		},
		"upstream": map[string]any{
			"health_url": res.HealthUpstreamURL,
			"ok":         ok,
			"status":     st,
			"detail":     detail,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	if !ok {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	_ = json.NewEncoder(w).Encode(body)
}
