// Package server: GET /api/ui/chimera-broker/providers — live provider/key snapshot for the
// BiFrost service card's "Provider health" strip in the logs UI. Pulls per-provider config
// from Chimera Broker's management API (GET /api/providers/{name}) via internal/chimeraBrokeradmin and
// classifies each entry into a small operator-facing state vocabulary so the strip in
// internal/server/embedui/logs.js can render meaningful colors regardless of what the
// supervised chimera-broker-http subprocess happens to log about providers in this build.
package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	chimeraBrokeradmin "github.com/lynn/porcelain/chimera/chimera-gateway/internal/bifrostadmin"
)

// chimeraBrokerUIProviderNames is the fixed roster the operator UI surfaces today (mirrors
// handleState in ui_handlers.go). Keep these two lists in sync if either changes.
var chimeraBrokerUIProviderNames = []string{"groq", "gemini", "ollama"}

// chimeraBrokerProviderHealthEntry is one row in the provider health strip JSON.
//
// state values:
//   - "up"          provider is registered in BiFrost and has a usable key (or, for ollama,
//     has a base_url configured).
//   - "down"        BiFrost itself is unreachable (transport error or 5xx).
//   - "key_missing" provider is registered but has no configured key (groq / gemini).
//   - "unknown"     provider is not registered (404), or the GET response could not be parsed.
type chimeraBrokerProviderHealthEntry struct {
	ID            string   `json:"id"`
	State         string   `json:"state"`
	KeyConfigured bool     `json:"key_configured"`
	KeyCount      int      `json:"key_count"`
	KeyHint       string   `json:"key_hint,omitempty"`
	ModelIDs      []string `json:"model_ids,omitempty"`
	OllamaBaseURL string   `json:"ollama_base_url,omitempty"`
	HTTPStatus    int      `json:"http_status,omitempty"`
	Error         string   `json:"error,omitempty"`
}

type chimeraBrokerProviderHealthResponse struct {
	FetchedAt time.Time                          `json:"fetched_at"`
	BifrostUp bool                               `json:"chimera_broker_up"`
	Error     string                             `json:"error,omitempty"`
	Providers []chimeraBrokerProviderHealthEntry `json:"providers"`
}

// classifyBifrostProviderResult turns one (status, body, transport-err) tuple from
// chimeraBrokeradmin.GetProvider into a strip-ready entry. Pure for unit testing.
//
// transportErr signals the Chimera Broker subprocess itself was unreachable; in that case the
// provider state is "down" and KeyConfigured/KeyCount are zeroed.
//
// liveSnapshot, when non-nil and fresh, overrides the config-only verdict: a provider that is
// configured + key-present but absent from the live `/v1/models` catalog is downgraded to
// "down" (Chimera Broker is pruning its models because the upstream is unreachable). Pass nil to
// skip the override for tests that only exercise config-view classification.
func classifyBifrostProviderResult(name string, body []byte, status int, transportErr error, liveSnapshot *CatalogSnapshot) chimeraBrokerProviderHealthEntry {
	out := chimeraBrokerProviderHealthEntry{ID: name, State: "unknown"}
	if transportErr != nil {
		out.State = "down"
		out.Error = transportErr.Error()
		return out
	}
	out.HTTPStatus = status
	if chimeraBrokeradmin.IsProviderMissingGET(status, body) {
		out.State = "unknown"
		return out
	}
	if status < 200 || status >= 300 {
		if status >= 500 {
			out.State = "down"
		} else {
			out.State = "unknown"
		}
		errText := strings.TrimSpace(string(body))
		if errText == "" {
			errText = http.StatusText(status)
		}
		if len(errText) > 200 {
			errText = errText[:199] + "…"
		}
		out.Error = errText
		return out
	}
	sum, err := chimeraBrokeradmin.SummarizeProvider(name, body)
	if err != nil {
		out.State = "unknown"
		out.Error = err.Error()
		return out
	}
	keys, _ := chimeraBrokeradmin.SummarizeProviderKeys(name, body)
	out.KeyConfigured = sum.KeyConfigured
	out.KeyCount = len(keys)
	out.KeyHint = sum.KeyHint
	if sum.OllamaBaseURL != "" {
		out.OllamaBaseURL = sum.OllamaBaseURL
	}
	switch strings.ToLower(name) {
	case "ollama":
		// Ollama can run without API keys; treat a configured base_url as "up". A registered
		// provider entry with neither base_url nor keys is still surface-able as key_missing
		// because the operator did add it but didn't finish wiring it.
		if sum.OllamaBaseURL != "" || sum.KeyConfigured {
			out.State = "up"
		} else {
			out.State = "key_missing"
		}
	default:
		if sum.KeyConfigured {
			out.State = "up"
		} else {
			out.State = "key_missing"
		}
	}
	// Live override: BiFrost prunes a provider's models from /v1/models the moment it can't
	// reach the upstream (most visible with local ollama: stop the daemon and `ollama/...`
	// disappears from the merged catalog). When a fresh snapshot says "configured but no
	// models present", treat that as the runtime liveness verdict and downgrade to "down".
	// We only override the otherwise-positive states ("up") because a config-side
	// "key_missing" / "unknown" already explains why the provider can't serve traffic.
	if out.State == "up" && liveSnapshot != nil && liveSnapshot.OK &&
		liveSnapshot.IsFresh(time.Now(), CatalogSnapshotFreshness) &&
		!liveSnapshot.HasProvider(name) {
		out.State = "down"
		if out.Error == "" {
			out.Error = "no models available in live catalog"
		}
	}
	return out
}

// fetchChimeraBrokerProviderHealth queries Chimera Broker for each well-known provider in parallel-by-loop
// and aggregates results. Pure of net/http handler glue so it can be exercised under test
// with a stub chimeraBrokeradmin.Client.
//
// liveSnapshot, when non-nil and fresh, is used by [classifyChimeraBrokerProviderResult] to
// override the config-view verdict with the live `/v1/models` catalog (see that function for
// the override rules). Pass nil from tests that only need to exercise the config branch.
func fetchChimeraBrokerProviderHealth(ctx context.Context, client *chimeraBrokeradmin.Client, names []string, liveSnapshot *CatalogSnapshot) chimeraBrokerProviderHealthResponse {
	out := chimeraBrokerProviderHealthResponse{
		FetchedAt: time.Now().UTC(),
		Providers: make([]chimeraBrokerProviderHealthEntry, 0, len(names)),
	}
	snapshotFresh := liveSnapshot != nil && liveSnapshot.OK && liveSnapshot.IsFresh(time.Now(), CatalogSnapshotFreshness)
	if client == nil || strings.TrimSpace(client.BaseURL) == "" {
		out.Error = "chimera-broker upstream not configured"
		out.BifrostUp = false
		for _, n := range names {
			out.Providers = append(out.Providers, chimeraBrokerProviderHealthEntry{ID: n, State: "down", Error: out.Error})
		}
		return out
	}
	anySuccess := false
	for _, name := range names {
		body, status, err := client.GetProvider(ctx, name)
		entry := classifyBifrostProviderResult(name, body, status, err, liveSnapshot)
		if snapshotFresh {
			entry.ModelIDs = catalogModelIDsForProvider(liveSnapshot, name)
		}
		if err == nil {
			anySuccess = true
		}
		out.Providers = append(out.Providers, entry)
	}
	out.BifrostUp = anySuccess
	if !anySuccess {
		// All probes failed at the transport level — annotate the response so the strip
		// caption can explain the empty state instead of looking like "no providers".
		out.Error = "chimera-broker-http unreachable"
		for i := range out.Providers {
			if out.Providers[i].State == "" || out.Providers[i].State == "unknown" {
				out.Providers[i].State = "down"
			}
		}
	}
	return out
}

func catalogModelIDsForProvider(snap *CatalogSnapshot, providerName string) []string {
	if snap == nil || !snap.OK || len(snap.ModelIDs) == 0 {
		return nil
	}
	prefix := strings.ToLower(strings.TrimSpace(providerName)) + "/"
	if prefix == "/" {
		return nil
	}
	out := make([]string, 0, 24)
	for _, mid := range snap.ModelIDs {
		id := strings.TrimSpace(mid)
		if id == "" {
			continue
		}
		if strings.HasPrefix(strings.ToLower(id), prefix) {
			out = append(out, id)
		}
	}
	return out
}

func fetchBifrostProviderHealth(ctx context.Context, client *chimeraBrokeradmin.Client, names []string, liveSnapshot *CatalogSnapshot) chimeraBrokerProviderHealthResponse {
	return fetchChimeraBrokerProviderHealth(ctx, client, names, liveSnapshot)
}

func (a *adminUI) handleChimeraBrokerProviderHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	a.rt.Sync()
	res, _, _ := a.rt.Snapshot()
	if res == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "gateway not configured"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()
	client := chimeraBrokerAdminClient(a.rt)
	// Read the live `/v1/models` snapshot maintained by the periodic catalog poller (see
	// availablemodels.go). When fresh, missing providers in that catalog override the
	// config-view "up" verdict so the Provider health strip reflects runtime reality.
	resp := fetchChimeraBrokerProviderHealth(ctx, client, chimeraBrokerUIProviderNames, a.rt.CatalogSnapshot())
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
