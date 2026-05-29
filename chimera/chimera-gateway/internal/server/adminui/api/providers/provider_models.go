package providers

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/operatorstore"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/adminui/apirut"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/adminui/handler"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/catalog"
	gruntime "github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/runtime"
	"github.com/lynn/porcelain/chimera/internal/config"
	"github.com/lynn/porcelain/chimera/internal/providerfreetier"
	"github.com/lynn/porcelain/internal/operatorapi"
)

func handleProviderModelsGET(h *handler.Handler, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	providerID := strings.ToLower(strings.TrimSpace(r.PathValue("provider_id")))
	if apirut.LookupProviderCatalogEntry(providerID) == nil {
		http.NotFound(w, r)
		return
	}
	tenantID := h.SessionTenantID(r)
	resp, err := buildProviderModelsResponse(r.Context(), h.RT, tenantID, providerID, nil)
	if err != nil {
		writeProviderModelsError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func handleProviderModelsPUT(h *handler.Handler, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	providerID := strings.ToLower(strings.TrimSpace(r.PathValue("provider_id")))
	if apirut.LookupProviderCatalogEntry(providerID) == nil {
		http.NotFound(w, r)
		return
	}
	var body operatorapi.ProviderModelsUpdateRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20))
	if err := dec.Decode(&body); err != nil {
		writeProviderModelsError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if body.Models == nil {
		writeProviderModelsError(w, http.StatusBadRequest, "models required")
		return
	}
	st := h.RT.OperatorStore()
	if st == nil {
		writeProviderModelsError(w, http.StatusServiceUnavailable, "operator store unavailable")
		return
	}
	tenantID := h.SessionTenantID(r)
	if err := st.ReplaceProviderModelAvailability(r.Context(), tenantID, providerID, body.Models); err != nil {
		writeProviderModelsError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := h.RT.ReloadProviderModelAvailability(r.Context()); err != nil {
		writeProviderModelsError(w, http.StatusInternalServerError, err.Error())
		return
	}
	resp, err := buildProviderModelsResponse(r.Context(), h.RT, tenantID, providerID, nil)
	if err != nil {
		writeProviderModelsError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func handleProviderModelsApplyFreeTierPOST(h *handler.Handler, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	providerID := strings.ToLower(strings.TrimSpace(r.PathValue("provider_id")))
	if apirut.LookupProviderCatalogEntry(providerID) == nil {
		http.NotFound(w, r)
		return
	}
	tenantID := h.SessionTenantID(r)
	h.RT.Sync()
	res, _, _ := h.RT.Snapshot()

	var draft map[string]bool
	note := ""
	if providerID == operatorstoreBootstrapOllama {
		note = "Ollama models are always treated as free tier; availability unchanged."
	} else {
		spec := providerFreeTierSpec(res)
		brokerIDs := brokerModelIDsForProvider(h.RT, providerID)
		draft = operatorstore.AvailabilityFromFreeTierSpec(spec, providerID, brokerIDs)
		if spec == nil {
			note = "provider-free-tier.yaml unavailable; proposed all models unavailable for this provider"
		}
	}
	resp, err := buildProviderModelsResponse(r.Context(), h.RT, tenantID, providerID, draft)
	if err != nil {
		writeProviderModelsError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := operatorapi.ProviderModelsApplyFreeTierResponse{
		OK:                     true,
		ProviderID:             providerID,
		TenantID:               tenantID,
		Models:                 resp.Models,
		ModelsAvailableCount:   resp.ModelsAvailableCount,
		ModelsUnavailableCount: resp.ModelsUnavailableCount,
		Note:                   note,
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

const operatorstoreBootstrapOllama = "ollama"

func writeProviderModelsError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(operatorapi.ErrorBody{Error: message})
}

func buildProviderModelsResponse(ctx context.Context, rt *gruntime.Runtime, tenantID, providerID string, draft map[string]bool) (operatorapi.ProviderModelsResponse, error) {
	brokerIDs := brokerModelIDsForProvider(rt, providerID)
	entries, configured, err := mergedProviderModelEntries(ctx, rt, tenantID, providerID, brokerIDs, draft)
	if err != nil {
		return operatorapi.ProviderModelsResponse{}, err
	}
	avail, unavail := countProviderModelAvailability(entries)
	return operatorapi.ProviderModelsResponse{
		ProviderID:             providerID,
		TenantID:               tenantID,
		Models:                 entries,
		ModelsAvailableCount:   avail,
		ModelsUnavailableCount: unavail,
		ModelsConfigured:       configured,
	}, nil
}

func mergedProviderModelEntries(ctx context.Context, rt *gruntime.Runtime, tenantID, providerID string, brokerIDs []string, draft map[string]bool) ([]operatorapi.ProviderModelEntry, bool, error) {
	usage := dayUsageByModel(ctx, rt, providerID)
	configured := false
	var merged []operatorstore.ProviderModelAvailabilityEntry

	if draft != nil {
		for _, id := range brokerIDs {
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			avail, ok := draft[id]
			if !ok {
				merged = append(merged, operatorstore.ProviderModelAvailabilityEntry{ModelID: id, Available: true, Explicit: false})
				continue
			}
			merged = append(merged, operatorstore.ProviderModelAvailabilityEntry{ModelID: id, Available: avail, Explicit: true})
		}
	} else {
		st := rt.OperatorStore()
		if st != nil {
			var err error
			configured, err = st.ProviderModelConfigConfigured(ctx, tenantID, providerID)
			if err != nil {
				return nil, false, err
			}
			stored, err := st.ListProviderModelAvailability(ctx, tenantID, providerID)
			if err != nil {
				return nil, false, err
			}
			merged = operatorstore.MergeProviderModelAvailability(brokerIDs, stored)
		} else {
			merged = operatorstore.MergeProviderModelAvailability(brokerIDs, nil)
		}
	}

	out := make([]operatorapi.ProviderModelEntry, 0, len(merged))
	for _, row := range merged {
		entry := operatorapi.ProviderModelEntry{
			ModelID:   row.ModelID,
			Available: row.Available,
			Explicit:  row.Explicit,
		}
		if u, ok := usage[row.ModelID]; ok {
			entry.Calls24h = u.calls
			entry.Errors24h = u.errors
		}
		out = append(out, entry)
	}
	return out, configured, nil
}

type modelDayUsage struct {
	calls  int
	errors int
}

func dayUsageByModel(ctx context.Context, rt *gruntime.Runtime, providerID string) map[string]modelDayUsage {
	out := map[string]modelDayUsage{}
	st := rt.MetricsStore()
	if st == nil {
		return out
	}
	day := time.Now().UTC().Format("2006-01-02")
	rows, err := st.QueryDayRollups(ctx, day, 500)
	if err != nil {
		return out
	}
	providerID = strings.ToLower(strings.TrimSpace(providerID))
	for _, row := range rows {
		if strings.ToLower(strings.TrimSpace(row.Provider)) != providerID {
			continue
		}
		u := out[row.ModelID]
		u.calls += row.Calls
		if row.Status >= 400 {
			u.errors += row.Calls
		}
		out[row.ModelID] = u
	}
	return out
}

func countProviderModelAvailability(entries []operatorapi.ProviderModelEntry) (available, unavailable int) {
	for _, e := range entries {
		if e.Available {
			available++
		} else {
			unavailable++
		}
	}
	return available, unavailable
}

func brokerModelIDsForProvider(rt *gruntime.Runtime, providerID string) []string {
	if rt == nil {
		return nil
	}
	snap := rt.CatalogSnapshot()
	return catalogModelIDsForProvider(snap, providerID)
}

func providerFreeTierSpec(res *config.Resolved) *providerfreetier.Spec {
	if res == nil {
		return nil
	}
	if res.ProviderFreeTierSpec != nil {
		return res.ProviderFreeTierSpec
	}
	path := strings.TrimSpace(res.ProviderFreeTierPath)
	if path == "" {
		return nil
	}
	spec, err := providerfreetier.Load(path)
	if err != nil {
		return nil
	}
	return spec
}

// EnrichStateProviderModelCounts fills model availability summary fields on GET /api/ui/state providers.
func EnrichStateProviderModelCounts(ctx context.Context, rt *gruntime.Runtime, tenantID string, entry *operatorapi.StateProviderEntry) {
	if rt == nil || entry == nil {
		return
	}
	brokerIDs := brokerModelIDsForProvider(rt, entry.Provider)
	entries, configured, err := mergedProviderModelEntries(ctx, rt, tenantID, entry.Provider, brokerIDs, nil)
	if err != nil {
		return
	}
	avail, unavail := countProviderModelAvailability(entries)
	entry.ModelsAvailableCount = avail
	entry.ModelsUnavailableCount = unavail
	entry.ModelsConfigured = configured
}

// CatalogSnapshotModelIDsForProvider exports catalog model ids for tests.
func CatalogSnapshotModelIDsForProvider(snap *catalog.CatalogSnapshot, providerID string) []string {
	return catalogModelIDsForProvider(snap, providerID)
}
