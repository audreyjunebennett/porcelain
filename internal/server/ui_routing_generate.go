package server

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

	"github.com/lynn/claudia-gateway/internal/config"
	"github.com/lynn/claudia-gateway/internal/routing"
	"github.com/lynn/claudia-gateway/internal/routinggen"
	"github.com/lynn/claudia-gateway/internal/upstream"
	"gopkg.in/yaml.v3"
)

type routingDraft struct {
	IDs                []string
	Pool               []string
	Chain              []string
	RouterModels       []string
	RouteYAML          []byte
	FilterFreeTierFlag bool
}

func summarizeRoutingYAML(b []byte) map[string]any {
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
	ruleOut := make([]map[string]any, 0, len(doc.Rules))
	for _, r := range doc.Rules {
		init := ""
		if len(r.Models) > 0 {
			init = r.Models[0]
		}
		entry := map[string]any{
			"name":          r.Name,
			"initial_model": init,
		}
		if r.When.Min != nil {
			entry["min_message_chars"] = *r.When.Min
		}
		ruleOut = append(ruleOut, entry)
	}
	return map[string]any{
		"ambiguous_default_model": doc.AmbiguousDefault,
		"rules":                   ruleOut,
	}
}

func (a *adminUI) computeRoutingDraft(ctx context.Context, res *config.Resolved) (*routingDraft, int, map[string]any) {
	apiKey := a.rt.UpstreamAPIKey()
	if apiKey == "" {
		return nil, http.StatusServiceUnavailable, map[string]any{
			"error": map[string]any{"message": "missing upstream API key", "type": "gateway_config"},
		}
	}
	timeout := healthTimeout(res)
	ctx, cancel := context.WithTimeout(ctx, timeout+2*time.Second)
	defer cancel()
	st, body, ok := upstream.FetchOpenAIModels(ctx, res.UpstreamBaseURL, apiKey, timeout, a.log)
	if !ok {
		return nil, http.StatusBadGateway, map[string]any{
			"error": map[string]any{
				"message": "Failed to list models from upstream",
				"type":    "gateway_upstream",
				"status":  st,
			},
		}
	}
	ids, err := routinggen.ExtractCatalogModelIDs(body, res.VirtualModelID)
	if err != nil {
		return nil, http.StatusBadGateway, map[string]any{
			"error": map[string]any{"message": "invalid upstream models JSON", "type": "gateway_upstream"},
		}
	}
	pool := ids
	if res.FilterFreeTierModels {
		if res.ProviderFreeTierSpec == nil || res.ProviderFreeTierSpec.Empty() {
			return nil, http.StatusBadRequest, map[string]any{
				"error": map[string]any{
					"message": "routing.filter_free_tier_models is true but provider-free-tier.yaml is missing, invalid, or empty",
					"type":    "gateway_config",
				},
			}
		}
		pool = res.ProviderFreeTierSpec.Filter(ids)
	}
	if len(pool) == 0 {
		return nil, http.StatusBadRequest, map[string]any{
			"error": map[string]any{
				"message": "no models left after catalog and optional free-tier filter",
				"type":    "gateway_config",
			},
		}
	}
	chain := routinggen.OrderFallbackChain(pool)
	routerModels := routinggen.OrderRouterModels(pool, res.ProviderLimitsSpec)
	routeYAML, err := routinggen.BuildRoutingPolicyYAML(chain)
	if err != nil {
		return nil, http.StatusInternalServerError, map[string]any{
			"error": map[string]any{"message": err.Error(), "type": "gateway_config"},
		}
	}
	if err := routing.ValidatePolicyYAML(routeYAML); err != nil {
		return nil, http.StatusBadRequest, map[string]any{
			"error": map[string]any{
				"message": "generated routing policy failed validation: " + err.Error(),
				"type":    "gateway_config",
			},
		}
	}
	return &routingDraft{
		IDs: ids, Pool: pool, Chain: chain, RouterModels: routerModels, RouteYAML: routeYAML,
		FilterFreeTierFlag: res.FilterFreeTierModels,
	}, 0, nil
}

func (a *adminUI) routingDraftResponse(d *routingDraft, saved bool) map[string]any {
	out := map[string]any{
		"ok":                           true,
		"saved":                        saved,
		"fallback_chain":               d.Chain,
		"router_models":                d.RouterModels,
		"models_upstream":              len(d.IDs),
		"models_used":                  len(d.Pool),
		"routing_policy_yaml":          string(d.RouteYAML),
		"routing":                      summarizeRoutingYAML(d.RouteYAML),
		"filter_free_tier_models_flag": d.FilterFreeTierFlag,
	}
	return out
}

func (a *adminUI) handleRoutingPreviewPOST(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	a.rt.Sync()
	res, _, _ := a.rt.Snapshot()
	if res == nil {
		writeRoutingGenJSONError(w, http.StatusInternalServerError, "gateway not configured")
		return
	}
	draft, st, errObj := a.computeRoutingDraft(r.Context(), res)
	if errObj != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(st)
		_ = json.NewEncoder(w).Encode(errObj)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(a.routingDraftResponse(draft, false))
}

func (a *adminUI) handleRoutingGeneratePOST(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	a.rt.Sync()
	res, _, _ := a.rt.Snapshot()
	if res == nil {
		writeRoutingGenJSONError(w, http.StatusInternalServerError, "gateway not configured")
		return
	}
	draft, st, errObj := a.computeRoutingDraft(r.Context(), res)
	if errObj != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(st)
		_ = json.NewEncoder(w).Encode(errObj)
		return
	}

	gwRaw, err := os.ReadFile(res.GatewayYAMLPath)
	if err != nil {
		writeRoutingGenJSONError(w, http.StatusInternalServerError, "read gateway.yaml")
		return
	}
	gwPatched, err := config.PatchGatewayYAMLBytesWithFallbackChain(gwRaw, draft.Chain)
	if err != nil {
		writeRoutingGenJSONError(w, http.StatusBadRequest, "gateway.yaml patch validation failed: "+err.Error())
		return
	}
	gwPatched, err = config.PatchGatewayYAMLBytesWithRouterModels(gwPatched, draft.RouterModels)
	if err != nil {
		writeRoutingGenJSONError(w, http.StatusBadRequest, "gateway.yaml router_models patch failed: "+err.Error())
		return
	}
	tmpValidate, err := os.CreateTemp(filepath.Dir(res.GatewayYAMLPath), "claudia-gw-validate-*.yaml")
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
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"message": "gateway.yaml after patch failed to load: " + err.Error(),
				"type":    "gateway_config",
			},
		})
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
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"message": "routing policy on disk failed validation after write",
				"type":    "gateway_config",
				"detail":  err.Error(),
			},
		})
		return
	}
	if _, err := config.LoadGatewayYAML(res.GatewayYAMLPath, nil); err != nil {
		writeRoutingGenJSONError(w, http.StatusInternalServerError, "reload gateway.yaml after write: "+err.Error())
		return
	}

	a.rt.Sync()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(a.routingDraftResponse(draft, true))
}

func (a *adminUI) handleRoutingEvaluatePOST(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	a.rt.Sync()
	res, _, _ := a.rt.Snapshot()
	if res == nil {
		writeRoutingGenJSONError(w, http.StatusInternalServerError, "gateway not configured")
		return
	}

	var body struct {
		RoutingPolicyYAML string          `json:"routing_policy_yaml"`
		FallbackChain     []string        `json:"fallback_chain"`
		VirtualModelID    string          `json:"virtual_model_id"`
		Messages          json.RawMessage `json:"messages"`
		SmokeCompletion   bool            `json:"smoke_completion"`
	}
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
	initial, via, err := routing.EvaluatePick(policyBytes, reqMap, body.FallbackChain, vm, a.log)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{"message": err.Error(), "type": "gateway_config"},
		})
		return
	}
	idx := routing.StartingFallbackIndex(initial, body.FallbackChain)
	slice := append([]string(nil), body.FallbackChain[idx:]...)

	out := map[string]any{
		"ok":                    true,
		"initial_model":         initial,
		"via":                   string(via),
		"fallback_start_index":  idx,
		"fallback_from_initial": slice,
	}

	if body.SmokeCompletion {
		apiKey := a.rt.UpstreamAPIKey()
		if apiKey == "" {
			out["smoke_completion"] = map[string]any{"ok": false, "error": "missing upstream API key"}
		} else if initial == "" {
			out["smoke_completion"] = map[string]any{"ok": false, "error": "no initial model to probe"}
		} else {
			to := healthTimeout(res)
			if to > 45*time.Second {
				to = 45 * time.Second
			}
			ctx, cancel := context.WithTimeout(r.Context(), to+2*time.Second)
			defer cancel()
			st, ok, det := upstream.SmokeChatCompletion(ctx, res.UpstreamBaseURL, apiKey, initial, to, a.log)
			sm := map[string]any{"ok": ok, "status": st, "detail": det}
			out["smoke_completion"] = sm
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

func (a *adminUI) handleRoutingRouterToolingPOST(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		RouterModels        []string `json:"router_models"`
		ToolRouterEnabled   bool     `json:"tool_router_enabled"`
		ConfidenceThreshold float64  `json:"confidence_threshold"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 512<<10))
	if err := dec.Decode(&body); err != nil {
		writeRoutingGenJSONError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if body.ConfidenceThreshold < 0 || body.ConfidenceThreshold > 1 {
		writeRoutingGenJSONError(w, http.StatusBadRequest, "confidence_threshold must be between 0 and 1")
		return
	}
	a.rt.Sync()
	res, _, _ := a.rt.Snapshot()
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
	a.rt.Sync()
	res2, _, _ := a.rt.Snapshot()

	missing := []string(nil)
	if len(ch) > 0 {
		apiKey := a.rt.UpstreamAPIKey()
		if apiKey != "" {
			to := healthTimeout(res2)
			ctx, cancel := context.WithTimeout(r.Context(), to+2*time.Second)
			defer cancel()
			st, catBody, ok := upstream.FetchOpenAIModels(ctx, res2.UpstreamBaseURL, apiKey, to, a.log)
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
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":                            true,
		"router_models":                 res2.RouterModels,
		"tool_router_enabled":           res2.ToolRouterEnabled,
		"confidence_threshold":          res2.ToolRouterConfidenceThreshold,
		"router_models_missing_catalog": missing,
	})
}

func (a *adminUI) handleRoutingFilterFreeTierPOST(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Enabled bool `json:"enabled"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<14))
	if err := dec.Decode(&body); err != nil {
		writeRoutingGenJSONError(w, http.StatusBadRequest, "invalid json")
		return
	}
	a.rt.Sync()
	res, _, _ := a.rt.Snapshot()
	if res == nil {
		writeRoutingGenJSONError(w, http.StatusInternalServerError, "gateway not configured")
		return
	}
	if err := config.WriteGatewayFilterFreeTierModels(res.GatewayYAMLPath, body.Enabled); err != nil {
		writeRoutingGenJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	a.rt.Sync()
	res2, _, _ := a.rt.Snapshot()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":                      true,
		"filter_free_tier_models": res2.FilterFreeTierModels,
	})
}

func (a *adminUI) handleRoutingPolicySavePOST(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		RoutingPolicyYAML string `json:"routing_policy_yaml"`
	}
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
	a.rt.Sync()
	res, _, _ := a.rt.Snapshot()
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
	a.rt.Sync()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":                    true,
		"saved":                 true,
		"routing_policy_yaml": strings.TrimSpace(string(rb)),
	})
}

func (a *adminUI) handleRoutingFallbackChainSavePOST(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		FallbackChain []string `json:"fallback_chain"`
	}
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
	a.rt.Sync()
	res, _, _ := a.rt.Snapshot()
	if res == nil || strings.TrimSpace(res.GatewayYAMLPath) == "" {
		writeRoutingGenJSONError(w, http.StatusInternalServerError, "gateway not configured")
		return
	}
	gwRaw, err := os.ReadFile(res.GatewayYAMLPath)
	if err != nil {
		writeRoutingGenJSONError(w, http.StatusInternalServerError, "read gateway.yaml: "+err.Error())
		return
	}
	gwPatched, err := config.PatchGatewayYAMLBytesWithFallbackChain(gwRaw, body.FallbackChain)
	if err != nil {
		writeRoutingGenJSONError(w, http.StatusBadRequest, "gateway.yaml patch validation failed: "+err.Error())
		return
	}
	tmpValidate, err := os.CreateTemp(filepath.Dir(res.GatewayYAMLPath), "claudia-gw-fb-validate-*.yaml")
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
		writeRoutingGenJSONError(w, http.StatusBadRequest, "gateway.yaml after patch failed to load: "+err.Error())
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
		writeRoutingGenJSONError(w, http.StatusInternalServerError, "reload gateway.yaml after write: "+err.Error())
		return
	}
	a.rt.Sync()
	res2, _, _ := a.rt.Snapshot()
	fb := []string(nil)
	if res2 != nil {
		fb = res2.FallbackChain
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":              true,
		"saved":           true,
		"fallback_chain": fb,
	})
}

func writeRoutingGenJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{"message": message, "type": "gateway_config"},
	})
}
