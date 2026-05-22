// GET /api/ui/chimera-broker/providers — live provider/key snapshot for the chimera-broker
// service card "Provider health" strip in the logs UI. Pulls per-provider config from the
// broker management API (GET /api/providers/{name}) via internal/brokeradmin and classifies
// each entry for adminui/embedui/settings_app.js regardless of subprocess log slugs in a given build.
package providers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/brokeradmin"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/adminui/apirut"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/adminui/handler"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/catalog"
	gruntime "github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/runtime"
	"github.com/lynn/porcelain/internal/operatorapi"
)

// ClassifyBrokerProviderResult turns one (status, body, transport-err) tuple from
// brokeradmin.GetProvider into a strip-ready entry. Pure for unit testing.
//
// transportErr signals the Chimera Broker subprocess itself was unreachable; in that case the
// provider state is "down" and KeyConfigured/KeyCount are zeroed.
//
// liveSnapshot, when non-nil and fresh, overrides the config-only verdict: a provider that is
// configured + key-present but absent from the live `/v1/models` catalog is downgraded to
// "down" (Chimera Broker is pruning its models because the upstream is unreachable). Pass nil to
// skip the override for tests that only exercise config-view classification.
// ClassifyBrokerProviderResult turns one provider probe into an operator-facing health entry.
func ClassifyBrokerProviderResult(name string, body []byte, status int, transportErr error, liveSnapshot *catalog.CatalogSnapshot) operatorapi.ProviderHealthEntry {
	out := operatorapi.ProviderHealthEntry{ID: name, State: "unknown"}
	if transportErr != nil {
		out.State = "down"
		out.Error = transportErr.Error()
		return out
	}
	out.HTTPStatus = status
	if brokeradmin.IsProviderMissingGET(status, body) {
		out.State = "not_configured"
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
	sum, err := brokeradmin.SummarizeProvider(name, body)
	if err != nil {
		out.State = "unknown"
		out.Error = err.Error()
		return out
	}
	keys, _ := brokeradmin.SummarizeProviderKeys(name, body)
	out.KeyConfigured = sum.KeyConfigured
	out.KeyCount = len(keys)
	out.KeyHint = sum.KeyHint
	if sum.OllamaBaseURL != "" {
		out.OllamaBaseURL = sum.OllamaBaseURL
	}
	if !providerConfigPresent(name, sum) {
		out.State = "key_missing"
		return out
	}
	out.State, out.Error = providerLivenessState(name, liveSnapshot, out.Error)
	return out
}

func providerConfigPresent(name string, sum brokeradmin.ProviderSummary) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "ollama":
		return sum.OllamaBaseURL != "" || sum.KeyConfigured
	default:
		return sum.KeyConfigured
	}
}

// providerLivenessState maps configured providers to up/down/unknown using the live /v1/models
// catalog. Config alone never yields "up" — only a fresh catalog snapshot with models present does.
// A fresh failed catalog poll (e.g. chimera-broker /v1/models 400 when ollama is unreachable)
// yields "down" for configured providers.
func providerLivenessState(name string, liveSnapshot *catalog.CatalogSnapshot, errHint string) (state string, errText string) {
	now := time.Now()
	if liveSnapshot != nil && liveSnapshot.IsFresh(now, catalog.CatalogSnapshotFreshness) {
		if liveSnapshot.OK {
			if liveSnapshot.HasProvider(name) {
				return "up", errHint
			}
			if errHint == "" {
				errHint = "no models available in live catalog"
			}
			return "down", errHint
		}
		errText := strings.TrimSpace(liveSnapshot.FetchErr)
		if errText == "" {
			errText = "model catalog unavailable"
		}
		return "down", errText
	}
	return "unknown", errHint
}

// fetchChimeraBrokerProviderHealth queries Chimera Broker for each well-known provider in parallel-by-loop
// and aggregates results. Pure of net/http handler glue so it can be exercised under test
// with a stub brokeradmin.Client.
//
// liveSnapshot, when non-nil and fresh, is used by [classifyChimeraBrokerProviderResult] to
// override the config-view verdict with the live `/v1/models` catalog (see that function for
// the override rules). Pass nil from tests that only need to exercise the config branch.
func fetchChimeraBrokerProviderHealth(ctx context.Context, client *brokeradmin.Client, names []string, liveSnapshot *catalog.CatalogSnapshot) operatorapi.ProviderHealthResponse {
	configured, listOK := brokeradmin.ListConfiguredProviders(ctx, client)
	return fetchChimeraBrokerProviderHealthWithList(ctx, client, names, configured, listOK, liveSnapshot)
}

func fetchChimeraBrokerProviderHealthWithList(ctx context.Context, client *brokeradmin.Client, names []string, configured map[string]struct{}, listOK bool, liveSnapshot *catalog.CatalogSnapshot) operatorapi.ProviderHealthResponse {
	out := operatorapi.ProviderHealthResponse{
		FetchedAt: time.Now().UTC(),
		Providers: make([]operatorapi.ProviderHealthEntry, 0, len(names)),
	}
	snapshotFresh := liveSnapshot != nil && liveSnapshot.OK && liveSnapshot.IsFresh(time.Now(), catalog.CatalogSnapshotFreshness)
	if client == nil || strings.TrimSpace(client.BaseURL) == "" {
		out.Error = "chimera-broker upstream not configured"
		out.BrokerUp = false
		for _, n := range names {
			out.Providers = append(out.Providers, operatorapi.ProviderHealthEntry{ID: n, State: "down", Error: out.Error})
		}
		return out
	}
	anySuccess := false
	for _, name := range names {
		body, status, err, httpProbed := brokeradmin.GetProviderForProbeWithList(ctx, client, name, configured, listOK)
		entry := ClassifyBrokerProviderResult(name, body, status, err, liveSnapshot)
		if snapshotFresh {
			entry.ModelIDs = catalogModelIDsForProvider(liveSnapshot, name)
		}
		if httpProbed && err == nil {
			anySuccess = true
		}
		out.Providers = append(out.Providers, entry)
	}
	out.BrokerUp = anySuccess
	if !anySuccess {
		// All live HTTP probes failed — annotate the response so the strip caption can explain
		// the empty state instead of looking like "no providers".
		out.Error = "chimera-broker-http unreachable"
		for i := range out.Providers {
			switch out.Providers[i].State {
			case "", "unknown", "key_missing":
				out.Providers[i].State = "down"
			}
		}
	}
	return out
}

func catalogModelIDsForProvider(snap *catalog.CatalogSnapshot, providerName string) []string {
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

// FetchBrokerProviderHealth queries Chimera Broker for provider health (tests and handlers).
func FetchBrokerProviderHealth(ctx context.Context, client *brokeradmin.Client, names []string, liveSnapshot *catalog.CatalogSnapshot) operatorapi.ProviderHealthResponse {
	return fetchChimeraBrokerProviderHealth(ctx, client, names, liveSnapshot)
}

func handleChimeraBrokerProviderHealth(h *handler.Handler, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	h.RT.Sync()
	res, _, _ := h.RT.Snapshot()
	if res == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(operatorapi.ErrorBody{Error: "gateway not configured"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()
	client := apirut.BrokerAdminClient(h.RT)
	liveSnapshot := catalogSnapshotForProviderHealth(ctx, h.RT, h.Log)
	configured, listOK := brokeradmin.ListConfiguredProviders(ctx, client)
	// Full UI roster so provider cards can distinguish configured vs not_configured; the health
	// strip filters out not_configured entries client-side.
	names := append([]string(nil), apirut.BrokerProviderNames...)
	resp := fetchChimeraBrokerProviderHealthWithList(ctx, client, names, configured, listOK, liveSnapshot)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func catalogSnapshotForProviderHealth(ctx context.Context, rt *gruntime.Runtime, log *slog.Logger) *catalog.CatalogSnapshot {
	if rt == nil {
		return nil
	}
	snap := rt.CatalogSnapshot()
	if snap != nil && snap.OK && snap.IsFresh(time.Now(), catalog.CatalogSnapshotFreshness) {
		return snap
	}
	return gruntime.RefreshAvailableModels(ctx, rt, log)
}
