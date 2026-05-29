package routing

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/routing"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/routinggen"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/adminui/handler"
	gruntime "github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/runtime"
	"github.com/lynn/porcelain/chimera/internal/brokerclient"
	"github.com/lynn/porcelain/chimera/internal/config"
	"github.com/lynn/porcelain/internal/naming"
	"github.com/lynn/porcelain/internal/operatorapi"
	"gopkg.in/yaml.v3"
)

func gatewayConfigLabel() string { return naming.GatewayConfigFileTarget }

type routingDraft struct {
	IDs                []string
	Pool               []string
	Chain              []string
	RouterModels       []string
	RouteYAML          []byte
	FilterFreeTierFlag bool
}

func summarizeRoutingYAML(b []byte) operatorapi.RoutingPolicySummary {
	var doc struct {
		AmbiguousDefault string `yaml:"ambiguous_default_model"`
		Rules            []struct {
			Name string `yaml:"name"`
			When struct {
				Min *int `yaml:"min_message_chars"`
			} `yaml:"when"`
			Models []string `yaml:"models"`
		} `yaml:"rules"`
	}
	_ = yaml.Unmarshal(b, &doc)
	ruleOut := make([]operatorapi.RoutingRuleSummary, 0, len(doc.Rules))
	for _, r := range doc.Rules {
		init := ""
		if len(r.Models) > 0 {
			init = r.Models[0]
		}
		entry := operatorapi.RoutingRuleSummary{
			Name:         r.Name,
			InitialModel: init,
		}
		if r.When.Min != nil {
			entry.MinMessageChars = r.When.Min
		}
		ruleOut = append(ruleOut, entry)
	}
	return operatorapi.RoutingPolicySummary{
		AmbiguousDefaultModel: doc.AmbiguousDefault,
		Rules:                 ruleOut,
	}
}

func routingConfigErr(message, typ string) operatorapi.RoutingConfigError {
	return operatorapi.RoutingConfigError{
		Error: operatorapi.RoutingConfigErrorDetail{Message: message, Type: typ},
	}
}

func computeRoutingDraft(h *handler.Handler, ctx context.Context, res *config.Resolved, tenantID string) (*routingDraft, int, *operatorapi.RoutingConfigError) {
	apiKey := h.RT.UpstreamAPIKey()
	if apiKey == "" {
		err := routingConfigErr("missing chimera-broker API key", "gateway_config")
		return nil, http.StatusServiceUnavailable, &err
	}
	timeout := gruntime.HealthTimeout(res)
	ctx, cancel := context.WithTimeout(ctx, timeout+2*time.Second)
	defer cancel()
	st, body, ok := brokerclient.FetchOpenAIModels(ctx, res.UpstreamBaseURL, apiKey, timeout, h.Log)
	if !ok {
		err := operatorapi.RoutingConfigError{
			Error: operatorapi.RoutingConfigErrorDetail{
				Message: "Failed to list models from chimera-broker",
				Type:    "gateway_upstream",
				Status:  st,
			},
		}
		return nil, http.StatusBadGateway, &err
	}
	ids, err := routinggen.ExtractCatalogModelIDs(body, res.VirtualModelID)
	if err != nil {
		errBody := routingConfigErr("invalid chimera-broker models JSON", "gateway_upstream")
		return nil, http.StatusBadGateway, &errBody
	}
	pool := filterModelsByTenantAvailability(h.RT, tenantID, ids)
	if len(pool) == 0 {
		errBody := routingConfigErr("no models left after operator availability filter", "gateway_config")
		return nil, http.StatusBadRequest, &errBody
	}
	chain := routinggen.OrderFallbackChain(pool)
	routerModels := routinggen.OrderRouterModels(pool, res.ProviderLimitsSpec)
	routeYAML, err := routinggen.BuildRoutingPolicyYAML(chain)
	if err != nil {
		errBody := routingConfigErr(err.Error(), "gateway_config")
		return nil, http.StatusInternalServerError, &errBody
	}
	if err := routing.ValidatePolicyYAML(routeYAML); err != nil {
		errBody := routingConfigErr("generated routing policy failed validation: "+err.Error(), "gateway_config")
		return nil, http.StatusBadRequest, &errBody
	}
	return &routingDraft{
		IDs: ids, Pool: pool, Chain: chain, RouterModels: routerModels, RouteYAML: routeYAML,
		FilterFreeTierFlag: false,
	}, 0, nil
}

func filterModelsByTenantAvailability(rt *gruntime.Runtime, tenantID string, ids []string) []string {
	if rt == nil {
		return ids
	}
	reg := rt.ProviderModels()
	if reg == nil {
		return ids
	}
	return reg.FilterAvailable(tenantID, ids)
}

func routingDraftResponse(d *routingDraft, saved bool) operatorapi.RoutingGenerateResponse {
	return operatorapi.RoutingGenerateResponse{
		OK:                       true,
		Saved:                    saved,
		FallbackChain:            d.Chain,
		RouterModels:             d.RouterModels,
		ModelsBrokerCatalog:      len(d.IDs),
		ModelsUsed:               len(d.Pool),
		RoutingPolicyYAML:        string(d.RouteYAML),
		Routing:                  summarizeRoutingYAML(d.RouteYAML),
		FilterFreeTierModelsFlag: d.FilterFreeTierFlag,
	}
}

func handleRoutingPreviewPOST(h *handler.Handler, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	h.RT.Sync()
	res, _, _ := h.RT.Snapshot()
	if res == nil {
		writeRoutingGenJSONError(w, http.StatusInternalServerError, "gateway not configured")
		return
	}
	draft, st, errObj := computeRoutingDraft(h, r.Context(), res, h.SessionTenantID(r))
	if errObj != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(st)
		_ = json.NewEncoder(w).Encode(errObj)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(routingDraftResponse(draft, false))
}

func handleRoutingGeneratePOST(h *handler.Handler, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	h.RT.Sync()
	res, _, _ := h.RT.Snapshot()
	if res == nil {
		writeRoutingGenJSONError(w, http.StatusInternalServerError, "gateway not configured")
		return
	}
	draft, st, errObj := computeRoutingDraft(h, r.Context(), res, h.SessionTenantID(r))
	if errObj != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(st)
		_ = json.NewEncoder(w).Encode(errObj)
		return
	}

	gwRaw, err := os.ReadFile(res.GatewayYAMLPath)
	if err != nil {
		writeRoutingGenJSONError(w, http.StatusInternalServerError, "read "+gatewayConfigLabel())
		return
	}
	gwPatched, err := config.PatchGatewayYAMLBytesWithFallbackChain(gwRaw, draft.Chain)
	if err != nil {
		writeRoutingGenJSONError(w, http.StatusBadRequest, gatewayConfigLabel()+" patch validation failed: "+err.Error())
		return
	}
	gwPatched, err = config.PatchGatewayYAMLBytesWithRouterModels(gwPatched, draft.RouterModels)
	if err != nil {
		writeRoutingGenJSONError(w, http.StatusBadRequest, gatewayConfigLabel()+" router_models patch failed: "+err.Error())
		return
	}
	tmpValidate, err := os.CreateTemp(filepath.Dir(res.GatewayYAMLPath), "chimera-gw-validate-*.yaml")
	if err != nil {
		writeRoutingGenJSONError(w, http.StatusInternalServerError, "temp file")
		return
	}
	tmpPath := tmpValidate.Name()
	_ = tmpValidate.Close()
	defer func() { _ = os.Remove(tmpPath) }()
	if err := os.WriteFile(tmpPath, gwPatched, 0o600); err != nil {
		writeRoutingGenJSONError(w, http.StatusInternalServerError, "stage gateway validate")
		return
	}
	if _, err := config.LoadGatewayYAML(tmpPath, nil); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(routingConfigErr(gatewayConfigLabel()+" after patch failed to load: "+err.Error(), "gateway_config"))
		return
	}

	routePerm := fs.FileMode(0o644)
	if st, err := os.Stat(res.RoutingPolicyPath); err == nil {
		routePerm = st.Mode() & fs.ModePerm
	}
	gwPerm := fs.FileMode(0o644)
	if st, err := os.Stat(res.GatewayYAMLPath); err == nil {
		gwPerm = st.Mode() & fs.ModePerm
	}

	if err := config.CommitRoutingAndGateway(res.RoutingPolicyPath, draft.RouteYAML, routePerm, res.GatewayYAMLPath, gwPatched, gwPerm); err != nil {
		writeRoutingGenJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	rb, rerr := os.ReadFile(res.RoutingPolicyPath)
	if rerr != nil {
		writeRoutingGenJSONError(w, http.StatusInternalServerError, "read back routing-policy.yaml")
		return
	}
	if err := routing.ValidatePolicyYAML(rb); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(operatorapi.RoutingConfigError{
			Error: operatorapi.RoutingConfigErrorDetail{
				Message: "routing policy on disk failed validation after write",
				Type:    "gateway_config",
				Detail:  err.Error(),
			},
		})
		return
	}
	if _, err := config.LoadGatewayYAML(res.GatewayYAMLPath, nil); err != nil {
		writeRoutingGenJSONError(w, http.StatusInternalServerError, "reload "+gatewayConfigLabel()+" after write: "+err.Error())
		return
	}

	h.RT.Sync()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(routingDraftResponse(draft, true))
}

func handleRoutingEvaluatePOST(h *handler.Handler, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	h.RT.Sync()
	res, _, _ := h.RT.Snapshot()
	if res == nil {
		writeRoutingGenJSONError(w, http.StatusInternalServerError, "gateway not configured")
		return
	}

	var body operatorapi.RoutingEvaluateRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 512<<10))
	if err := dec.Decode(&body); err != nil {
		writeRoutingGenJSONError(w, http.StatusBadRequest, "invalid json")
		return
	}
	yamlStr := strings.TrimSpace(body.RoutingPolicyYAML)
	if yamlStr == "" {
		writeRoutingGenJSONError(w, http.StatusBadRequest, "routing_policy_yaml required")
		return
	}
	if len(body.FallbackChain) == 0 {
		writeRoutingGenJSONError(w, http.StatusBadRequest, "fallback_chain required")
		return
	}
	vm := strings.TrimSpace(body.VirtualModelID)
	if vm == "" {
		vm = res.VirtualModelID
	}
	policyBytes := []byte(yamlStr)

	rawMsgs := body.Messages
	if len(rawMsgs) == 0 {
		rawMsgs = json.RawMessage(`[{"role":"user","content":"Hello."}]`)
	}
	modelField, err := json.Marshal(vm)
	if err != nil {
		writeRoutingGenJSONError(w, http.StatusBadRequest, "virtual model id")
		return
	}
	reqMap := map[string]json.RawMessage{
		"model":    modelField,
		"messages": rawMsgs,
	}
	initial, via, err := routing.EvaluatePick(policyBytes, reqMap, body.FallbackChain, vm, h.Log)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(routingConfigErr(err.Error(), "gateway_config"))
		return
	}
	idx := routing.StartingFallbackIndex(initial, body.FallbackChain)
	slice := append([]string(nil), body.FallbackChain[idx:]...)

	out := operatorapi.RoutingEvaluateResponse{
		OK:                  true,
		InitialModel:        initial,
		Via:                 string(via),
		FallbackStartIndex:  idx,
		FallbackFromInitial: slice,
	}

	if body.SmokeCompletion {
		apiKey := h.RT.UpstreamAPIKey()
		if apiKey == "" {
			out.SmokeCompletion = &operatorapi.SmokeCompletionResult{OK: false, Error: "missing chimera-broker API key"}
		} else if initial == "" {
			out.SmokeCompletion = &operatorapi.SmokeCompletionResult{OK: false, Error: "no initial model to probe"}
		} else {
			to := gruntime.HealthTimeout(res)
			if to > 45*time.Second {
				to = 45 * time.Second
			}
			ctx, cancel := context.WithTimeout(r.Context(), to+2*time.Second)
			defer cancel()
			st, ok, det := brokerclient.SmokeChatCompletion(ctx, res.UpstreamBaseURL, apiKey, initial, to, h.Log)
			out.SmokeCompletion = &operatorapi.SmokeCompletionResult{OK: ok, Status: st, Detail: det}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

func handleRoutingRouterToolingPOST(h *handler.Handler, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body operatorapi.RoutingRouterToolingRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 512<<10))
	if err := dec.Decode(&body); err != nil {
		writeRoutingGenJSONError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if body.ConfidenceThreshold < 0 || body.ConfidenceThreshold > 1 {
		writeRoutingGenJSONError(w, http.StatusBadRequest, "confidence_threshold must be between 0 and 1")
		return
	}
	h.RT.Sync()
	res, _, _ := h.RT.Snapshot()
	if res == nil {
		writeRoutingGenJSONError(w, http.StatusInternalServerError, "gateway not configured")
		return
	}
	ch := make([]string, 0, len(body.RouterModels))
	for _, id := range body.RouterModels {
		if s := strings.TrimSpace(id); s != "" {
			ch = append(ch, s)
		}
	}
	if err := config.WriteGatewayRouterTooling(res.GatewayYAMLPath, ch, body.ToolRouterEnabled, body.ConfidenceThreshold); err != nil {
		writeRoutingGenJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	h.RT.Sync()
	res2, _, _ := h.RT.Snapshot()

	missing := []string(nil)
	if len(ch) > 0 {
		apiKey := h.RT.UpstreamAPIKey()
		if apiKey != "" {
			to := gruntime.HealthTimeout(res2)
			ctx, cancel := context.WithTimeout(r.Context(), to+2*time.Second)
			defer cancel()
			st, catBody, ok := brokerclient.FetchOpenAIModels(ctx, res2.UpstreamBaseURL, apiKey, to, h.Log)
			if ok && st >= 200 && st < 300 {
				ids, err := routinggen.ExtractCatalogModelIDs(catBody, res2.VirtualModelID)
				if err == nil {
					set := make(map[string]struct{}, len(ids))
					for _, id := range ids {
						set[id] = struct{}{}
					}
					for _, m := range ch {
						if _, ok := set[m]; !ok {
							missing = append(missing, m)
						}
					}
				}
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(operatorapi.RoutingRouterToolingResponse{
		OK:                         true,
		RouterModels:               res2.RouterModels,
		ToolRouterEnabled:          res2.ToolRouterEnabled,
		ConfidenceThreshold:        res2.ToolRouterConfidenceThreshold,
		RouterModelsMissingCatalog: missing,
	})
}

func handleRoutingFilterFreeTierPOST(h *handler.Handler, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body operatorapi.RoutingFilterFreeTierRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<14))
	if err := dec.Decode(&body); err != nil {
		writeRoutingGenJSONError(w, http.StatusBadRequest, "invalid json")
		return
	}
	h.RT.Sync()
	res, _, _ := h.RT.Snapshot()
	if res == nil {
		writeRoutingGenJSONError(w, http.StatusInternalServerError, "gateway not configured")
		return
	}
	if err := config.WriteGatewayFilterFreeTierModels(res.GatewayYAMLPath, body.Enabled); err != nil {
		writeRoutingGenJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	h.RT.Sync()
	res2, _, _ := h.RT.Snapshot()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(operatorapi.RoutingFilterFreeTierResponse{
		OK:                   true,
		FilterFreeTierModels: res2.FilterFreeTierModels,
	})
}

func handleRoutingPolicySavePOST(h *handler.Handler, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body operatorapi.RoutingPolicySaveRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 512<<10))
	if err := dec.Decode(&body); err != nil {
		writeRoutingGenJSONError(w, http.StatusBadRequest, "invalid json")
		return
	}
	yamlStr := strings.TrimSpace(body.RoutingPolicyYAML)
	if err := routing.ValidatePolicyYAML([]byte(yamlStr)); err != nil {
		writeRoutingGenJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	h.RT.Sync()
	res, _, _ := h.RT.Snapshot()
	if res == nil || strings.TrimSpace(res.RoutingPolicyPath) == "" {
		writeRoutingGenJSONError(w, http.StatusInternalServerError, "gateway not configured")
		return
	}
	routePerm := fs.FileMode(0o644)
	if st, err := os.Stat(res.RoutingPolicyPath); err == nil {
		routePerm = st.Mode() & fs.ModePerm
	}
	if err := config.ReplaceFile(res.RoutingPolicyPath, []byte(yamlStr), routePerm); err != nil {
		writeRoutingGenJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	rb, err := os.ReadFile(res.RoutingPolicyPath)
	if err != nil {
		writeRoutingGenJSONError(w, http.StatusInternalServerError, "read back routing policy: "+err.Error())
		return
	}
	if err := routing.ValidatePolicyYAML(rb); err != nil {
		writeRoutingGenJSONError(w, http.StatusInternalServerError, "routing policy on disk failed validation after write: "+err.Error())
		return
	}
	h.RT.Sync()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(operatorapi.RoutingPolicySaveResponse{
		OK:                true,
		Saved:             true,
		RoutingPolicyYAML: strings.TrimSpace(string(rb)),
	})
}

func handleRoutingFallbackChainSavePOST(h *handler.Handler, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body operatorapi.RoutingFallbackChainSaveRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 512<<10))
	if err := dec.Decode(&body); err != nil {
		writeRoutingGenJSONError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if len(body.FallbackChain) == 0 {
		writeRoutingGenJSONError(w, http.StatusBadRequest, "fallback_chain must be non-empty")
		return
	}
	for i, id := range body.FallbackChain {
		if strings.TrimSpace(id) == "" {
			writeRoutingGenJSONError(w, http.StatusBadRequest, fmt.Sprintf("fallback_chain[%d] is empty", i))
			return
		}
	}
	h.RT.Sync()
	res, _, _ := h.RT.Snapshot()
	if res == nil || strings.TrimSpace(res.GatewayYAMLPath) == "" {
		writeRoutingGenJSONError(w, http.StatusInternalServerError, "gateway not configured")
		return
	}
	gwRaw, err := os.ReadFile(res.GatewayYAMLPath)
	if err != nil {
		writeRoutingGenJSONError(w, http.StatusInternalServerError, "read "+gatewayConfigLabel()+": "+err.Error())
		return
	}
	gwPatched, err := config.PatchGatewayYAMLBytesWithFallbackChain(gwRaw, body.FallbackChain)
	if err != nil {
		writeRoutingGenJSONError(w, http.StatusBadRequest, gatewayConfigLabel()+" patch validation failed: "+err.Error())
		return
	}
	tmpValidate, err := os.CreateTemp(filepath.Dir(res.GatewayYAMLPath), "chimera-gw-fb-validate-*.yaml")
	if err != nil {
		writeRoutingGenJSONError(w, http.StatusInternalServerError, "temp file")
		return
	}
	tmpPath := tmpValidate.Name()
	_ = tmpValidate.Close()
	defer func() { _ = os.Remove(tmpPath) }()
	if err := os.WriteFile(tmpPath, gwPatched, 0o600); err != nil {
		writeRoutingGenJSONError(w, http.StatusInternalServerError, "stage gateway validate")
		return
	}
	if _, err := config.LoadGatewayYAML(tmpPath, nil); err != nil {
		writeRoutingGenJSONError(w, http.StatusBadRequest, gatewayConfigLabel()+" after patch failed to load: "+err.Error())
		return
	}
	gwPerm := fs.FileMode(0o644)
	if st, err := os.Stat(res.GatewayYAMLPath); err == nil {
		gwPerm = st.Mode() & fs.ModePerm
	}
	if err := config.ReplaceFile(res.GatewayYAMLPath, gwPatched, gwPerm); err != nil {
		writeRoutingGenJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if _, err := config.LoadGatewayYAML(res.GatewayYAMLPath, nil); err != nil {
		writeRoutingGenJSONError(w, http.StatusInternalServerError, "reload "+gatewayConfigLabel()+" after write: "+err.Error())
		return
	}
	h.RT.Sync()
	res2, _, _ := h.RT.Snapshot()
	fb := []string(nil)
	if res2 != nil {
		fb = res2.FallbackChain
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(operatorapi.RoutingFallbackChainSaveResponse{
		OK:            true,
		Saved:         true,
		FallbackChain: fb,
	})
}

func writeRoutingGenJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(routingConfigErr(message, "gateway_config"))
}
