package server

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/lynn/porcelain/chimera/internal/brokerclient"
	"github.com/lynn/porcelain/internal/naming"
)

// StatusOverlay configures GET /status (operator + GUI). Pass nil from tests; production
// should set EffectiveListen to the address passed to http.ListenAndServe.
type StatusOverlay struct {
	EffectiveListen string
	Supervisor      *SupervisorInfo
}

// SupervisorInfo describes subprocesses when started via "chimera serve".
type SupervisorInfo struct {
	BrokerListen          string
	VectorstoreSupervised bool
	VectorstoreHTTP       string // host:port for chimera-vectorstore /readyz when supervised
	IndexerSupervised     bool
	IndexerConfigPath     string // single --config path when supervised (may be empty if not started)
}

func handleStatus(w http.ResponseWriter, r *http.Request, rt *Runtime, log *slog.Logger, overlay *StatusOverlay) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	rt.Sync()
	res, _ := rt.Snapshot()
	apiKey := rt.UpstreamAPIKey()
	ctx := r.Context()
	ok, st, detail := brokerclient.ProbeHealth(ctx, res.HealthUpstreamURL, apiKey, healthTimeout(res), log)

	listen := res.ListenAddr()
	if overlay != nil && overlay.EffectiveListen != "" {
		listen = overlay.EffectiveListen
	}

	sup := map[string]any{"active": false}
	if overlay != nil && overlay.Supervisor != nil {
		s := overlay.Supervisor
		sup = map[string]any{
			"active":                 true,
			"chimera_broker_listen":  s.BrokerListen,
			"vectorstore_supervised": s.VectorstoreSupervised,
			"vectorstore_http":       s.VectorstoreHTTP,
			"indexer_supervised":     s.IndexerSupervised,
			"indexer_config_path":    s.IndexerConfigPath,
		}
	}

	body := map[string]any{
		"supervisor": sup,
		"gateway": map[string]any{
			"listen":          listen,
			"virtual_model":   rt.PrimaryVirtualModelID(),
			"semver":          res.Semver,
			"broker_base_url": res.UpstreamBaseURL,
		},
		"broker": map[string]any{
			"health_url": res.HealthUpstreamURL,
			"base_url":   res.UpstreamBaseURL,
			"ok":         ok,
			"status":     st,
			"detail":     detail,
			"upstream": map[string]any{
				"implementation": naming.ProductBifrostHTTPBinName,
			},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	if !ok {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	_ = json.NewEncoder(w).Encode(body)
}
