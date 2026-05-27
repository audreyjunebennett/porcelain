package virtualmodels

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/operatorstore"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/routing"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/routinggen"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/adminui/handler"
	"github.com/lynn/porcelain/chimera/internal/brokerclient"
	"github.com/lynn/porcelain/chimera/internal/config"
	"github.com/lynn/porcelain/internal/operatorapi"
)

const operatorTenantID = ""

func vmSummary(vm operatorstore.VirtualModel) operatorapi.VirtualModelSummary {
	return operatorapi.VirtualModelSummary{
		ID:                   vm.ID,
		ModelID:              vm.ModelID,
		Name:                 vm.Name,
		Version:              vm.Version,
		Description:          vm.Description,
		Enabled:              vm.Enabled,
		Visibility:           vm.Visibility,
		FallbackDepth:        len(vm.FallbackChain),
		RoutingPolicyEnabled: vm.RoutingPolicyEnabled,
		ToolRouterEnabled:    vm.ToolRouterEnabled,
		RouterModels:         vm.RouterModels,
	}
}

func vmDetail(vm operatorstore.VirtualModel) operatorapi.VirtualModelDetail {
	return operatorapi.VirtualModelDetail{
		VirtualModelSummary:  vmSummary(vm),
		RoutingPolicyYAML:    vm.RoutingPolicyYAML,
		FallbackChain:        vm.FallbackChain,
		ToolRouterConfidence: vm.ToolRouterConfidence,
		CreatedByPrincipalID: vm.CreatedByPrincipalID,
		CreatedAt:            vm.CreatedAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt:            vm.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func operatorStore(h *handler.Handler) *operatorstore.Store {
	if h == nil || h.RT == nil {
		return nil
	}
	return h.RT.OperatorStore()
}

func reloadRegistry(h *handler.Handler, ctx context.Context) {
	if h == nil || h.RT == nil {
		return
	}
	_ = h.RT.ReloadVirtualModels(ctx)
}

func parseVMID(r *http.Request) (int64, bool) {
	idStr := strings.TrimSpace(r.PathValue("id"))
	if idStr == "" {
		return 0, false
	}
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

func handleListGET(h *handler.Handler, w http.ResponseWriter, r *http.Request) {
	st := operatorStore(h)
	if st == nil {
		http.Error(w, "operator store unavailable", http.StatusServiceUnavailable)
		return
	}
	vms, err := st.ListVirtualModels(r.Context(), operatorTenantID, operatorTenantID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	out := make([]operatorapi.VirtualModelSummary, 0, len(vms))
	for _, vm := range vms {
		out = append(out, vmSummary(vm))
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(operatorapi.VirtualModelListResponse{VirtualModels: out})
}

func handleCreatePOST(h *handler.Handler, w http.ResponseWriter, r *http.Request) {
	st := operatorStore(h)
	if st == nil {
		http.Error(w, "operator store unavailable", http.StatusServiceUnavailable)
		return
	}
	var body operatorapi.VirtualModelCreateRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	vm, err := st.CreateVirtualModel(r.Context(), operatorstore.CreateVirtualModelInput{
		ModelID:     body.ModelID,
		Name:        body.Name,
		Version:     body.Version,
		Description: body.Description,
		Visibility:  body.Visibility,
		TenantID:    operatorTenantID,
		Enabled:     true,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	reloadRegistry(h, r.Context())
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(vmDetail(*vm))
}

func handleGetGET(h *handler.Handler, w http.ResponseWriter, r *http.Request) {
	st := operatorStore(h)
	if st == nil {
		http.Error(w, "operator store unavailable", http.StatusServiceUnavailable)
		return
	}
	id, ok := parseVMID(r)
	if !ok {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	vm, err := st.GetVirtualModelByID(r.Context(), operatorTenantID, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if vm == nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(vmDetail(*vm))
}

func handleUpdatePUT(h *handler.Handler, w http.ResponseWriter, r *http.Request) {
	st := operatorStore(h)
	if st == nil {
		http.Error(w, "operator store unavailable", http.StatusServiceUnavailable)
		return
	}
	id, ok := parseVMID(r)
	if !ok {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	var body operatorapi.VirtualModelUpdateRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	name, version, desc := "", "", ""
	if body.Name != nil {
		name = *body.Name
	}
	if body.Version != nil {
		version = *body.Version
	}
	if body.Description != nil {
		desc = *body.Description
	}
	if err := st.UpdateVirtualModelMetadata(r.Context(), operatorTenantID, id, name, version, desc, body.Enabled, body.Visibility); err != nil {
		if strings.Contains(err.Error(), "not found") {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	reloadRegistry(h, r.Context())
	handleGetGET(h, w, r)
}

func handleDeleteDELETE(h *handler.Handler, w http.ResponseWriter, r *http.Request) {
	st := operatorStore(h)
	if st == nil {
		http.Error(w, "operator store unavailable", http.StatusServiceUnavailable)
		return
	}
	id, ok := parseVMID(r)
	if !ok {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	if err := st.DeleteVirtualModel(r.Context(), operatorTenantID, id); err != nil {
		if strings.Contains(err.Error(), "not found") {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	reloadRegistry(h, r.Context())
	w.WriteHeader(http.StatusNoContent)
}

func handleFallbackPUT(h *handler.Handler, w http.ResponseWriter, r *http.Request) {
	st := operatorStore(h)
	if st == nil {
		http.Error(w, "operator store unavailable", http.StatusServiceUnavailable)
		return
	}
	id, ok := parseVMID(r)
	if !ok {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	var body operatorapi.VirtualModelFallbackSaveRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if err := st.SetVirtualModelFallback(r.Context(), operatorTenantID, id, body.FallbackChain); err != nil {
		if strings.Contains(err.Error(), "not found") {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	reloadRegistry(h, r.Context())
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "fallback_chain": body.FallbackChain})
}

func handleRoutingPolicyPUT(h *handler.Handler, w http.ResponseWriter, r *http.Request) {
	st := operatorStore(h)
	if st == nil {
		http.Error(w, "operator store unavailable", http.StatusServiceUnavailable)
		return
	}
	id, ok := parseVMID(r)
	if !ok {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	var body operatorapi.VirtualModelRoutingPolicySaveRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<20)).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if body.Enabled && len(strings.TrimSpace(body.RoutingPolicyYAML)) > 0 {
		if err := routing.ValidatePolicyYAML([]byte(body.RoutingPolicyYAML)); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
	if err := st.SetVirtualModelRoutingPolicy(r.Context(), operatorTenantID, id, body.Enabled, body.RoutingPolicyYAML); err != nil {
		if strings.Contains(err.Error(), "not found") {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	reloadRegistry(h, r.Context())
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

func handleToolRouterPUT(h *handler.Handler, w http.ResponseWriter, r *http.Request) {
	st := operatorStore(h)
	if st == nil {
		http.Error(w, "operator store unavailable", http.StatusServiceUnavailable)
		return
	}
	id, ok := parseVMID(r)
	if !ok {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	var body operatorapi.VirtualModelToolRouterSaveRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	th := body.ConfidenceThreshold
	if th <= 0 {
		th = 0.5
	}
	if err := st.SetVirtualModelToolRouter(r.Context(), operatorTenantID, id, body.Enabled, body.RouterModels, th); err != nil {
		if strings.Contains(err.Error(), "not found") {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	reloadRegistry(h, r.Context())
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

func filterByProviderPrefix(ids []string, prefix string) []string {
	prefix = strings.TrimSpace(strings.ToLower(prefix))
	if prefix == "" {
		return ids
	}
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if strings.HasPrefix(strings.ToLower(id), prefix) {
			out = append(out, id)
		}
	}
	return out
}

func handleGeneratePOST(h *handler.Handler, w http.ResponseWriter, r *http.Request) {
	st := operatorStore(h)
	if st == nil {
		http.Error(w, "operator store unavailable", http.StatusServiceUnavailable)
		return
	}
	id, ok := parseVMID(r)
	if !ok {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	vm, err := st.GetVirtualModelByID(r.Context(), operatorTenantID, id)
	if err != nil || vm == nil {
		http.NotFound(w, r)
		return
	}
	var body operatorapi.VirtualModelGenerateRequest
	_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&body)

	h.RT.Sync()
	res, _, _ := h.RT.Snapshot()
	apiKey := h.RT.UpstreamAPIKey()
	if apiKey == "" {
		http.Error(w, "missing chimera-broker API key", http.StatusServiceUnavailable)
		return
	}
	stCode, catBody, ok := brokerclient.FetchOpenAIModels(r.Context(), res.UpstreamBaseURL, apiKey, configHealthTimeout(res), h.Log)
	if !ok {
		http.Error(w, "failed to list models from chimera-broker", http.StatusBadGateway)
		return
	}
	ids, err := routinggen.ExtractCatalogModelIDs(catBody, vm.ModelID)
	if err != nil {
		http.Error(w, "invalid chimera-broker models JSON", http.StatusBadGateway)
		return
	}
	_ = stCode
	pool := filterByProviderPrefix(ids, body.ProviderPrefix)
	if res.FilterFreeTierModels && body.ProviderPrefix == "" {
		if res.ProviderFreeTierSpec != nil && !res.ProviderFreeTierSpec.Empty() {
			pool = res.ProviderFreeTierSpec.Filter(pool)
		}
	}
	if len(pool) == 0 {
		http.Error(w, "no models left after catalog filter", http.StatusBadRequest)
		return
	}
	chain := routinggen.OrderFallbackChain(pool)
	routeYAML, err := routinggen.BuildRoutingPolicyYAML(chain)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := routing.ValidatePolicyYAML(routeYAML); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if body.Save {
		if err := st.SetVirtualModelFallback(r.Context(), operatorTenantID, id, chain); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := st.SetVirtualModelRoutingPolicy(r.Context(), operatorTenantID, id, true, string(routeYAML)); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		reloadRegistry(h, r.Context())
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(operatorapi.RoutingGenerateResponse{
		OK:                  true,
		Saved:               body.Save,
		FallbackChain:       chain,
		RoutingPolicyYAML:   string(routeYAML),
		ModelsBrokerCatalog: len(ids),
		ModelsUsed:          len(pool),
	})
}

func handleEvaluatePOST(h *handler.Handler, w http.ResponseWriter, r *http.Request) {
	st := operatorStore(h)
	if st == nil {
		http.Error(w, "operator store unavailable", http.StatusServiceUnavailable)
		return
	}
	id, ok := parseVMID(r)
	if !ok {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	vm, err := st.GetVirtualModelByID(r.Context(), operatorTenantID, id)
	if err != nil || vm == nil {
		http.NotFound(w, r)
		return
	}
	var body operatorapi.RoutingEvaluateRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20)).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	policyYAML := []byte(body.RoutingPolicyYAML)
	if len(policyYAML) == 0 {
		policyYAML = []byte(vm.RoutingPolicyYAML)
	}
	chain := body.FallbackChain
	if len(chain) == 0 {
		chain = vm.FallbackChain
	}
	vmID := strings.TrimSpace(body.VirtualModelID)
	if vmID == "" {
		vmID = vm.ModelID
	}
	reqBody := map[string]json.RawMessage{"model": json.RawMessage(`"` + vmID + `"`)}
	if len(body.Messages) > 0 {
		reqBody["messages"] = body.Messages
	}
	initial, via, err := routing.EvaluatePick(policyYAML, reqBody, chain, vmID, h.Log)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	start := routing.StartingFallbackIndex(initial, chain)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(operatorapi.RoutingEvaluateResponse{
		OK:                  true,
		InitialModel:        initial,
		Via:                 string(via),
		FallbackStartIndex:  start,
		FallbackFromInitial: chain[start:],
	})
}

func configHealthTimeout(res *config.Resolved) time.Duration {
	if res == nil || res.HealthTimeoutMs <= 0 {
		return 5 * time.Second
	}
	return time.Duration(res.HealthTimeoutMs) * time.Millisecond
}
