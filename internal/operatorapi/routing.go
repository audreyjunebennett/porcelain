package operatorapi

import "encoding/json"

// RoutingRuleSummary is one rule in routing policy summary JSON.
type RoutingRuleSummary struct {
	Name            string `json:"name"`
	InitialModel    string `json:"initial_model"`
	MinMessageChars *int   `json:"min_message_chars,omitempty"`
}

// RoutingPolicySummary is the routing object inside generate/preview responses.
type RoutingPolicySummary struct {
	AmbiguousDefaultModel string               `json:"ambiguous_default_model"`
	Rules                 []RoutingRuleSummary `json:"rules"`
}

// RoutingGenerateResponse is preview/generate success JSON.
type RoutingGenerateResponse struct {
	OK                       bool                 `json:"ok"`
	Saved                    bool                 `json:"saved"`
	FallbackChain            []string             `json:"fallback_chain"`
	RouterModels             []string             `json:"router_models"`
	ModelsBrokerCatalog      int                  `json:"models_broker_catalog"`
	ModelsUsed               int                  `json:"models_used"`
	RoutingPolicyYAML        string               `json:"routing_policy_yaml"`
	Routing                  RoutingPolicySummary `json:"routing"`
	FilterFreeTierModelsFlag bool                 `json:"filter_free_tier_models_flag"`
}

// RoutingEvaluateRequest is POST /api/ui/routing/evaluate body.
type RoutingEvaluateRequest struct {
	RoutingPolicyYAML string          `json:"routing_policy_yaml"`
	FallbackChain     []string        `json:"fallback_chain"`
	VirtualModelID    string          `json:"virtual_model_id"`
	Messages          json.RawMessage `json:"messages"`
	SmokeCompletion   bool            `json:"smoke_completion"`
}

// SmokeCompletionResult is nested under RoutingEvaluateResponse.
type SmokeCompletionResult struct {
	OK     bool   `json:"ok"`
	Status int    `json:"status,omitempty"`
	Detail string `json:"detail,omitempty"`
	Error  string `json:"error,omitempty"`
}

// RoutingEvaluateResponse is POST /api/ui/routing/evaluate success JSON.
type RoutingEvaluateResponse struct {
	OK                  bool                   `json:"ok"`
	InitialModel        string                 `json:"initial_model"`
	Via                 string                 `json:"via"`
	FallbackStartIndex  int                    `json:"fallback_start_index"`
	FallbackFromInitial []string               `json:"fallback_from_initial"`
	SmokeCompletion     *SmokeCompletionResult `json:"smoke_completion,omitempty"`
}

// RoutingRouterToolingRequest is POST /api/ui/routing/router_tooling body.
type RoutingRouterToolingRequest struct {
	RouterModels        []string `json:"router_models"`
	ToolRouterEnabled   bool     `json:"tool_router_enabled"`
	ConfidenceThreshold float64  `json:"confidence_threshold"`
}

// RoutingRouterToolingResponse is POST /api/ui/routing/router_tooling success JSON.
type RoutingRouterToolingResponse struct {
	OK                         bool     `json:"ok"`
	RouterModels               []string `json:"router_models"`
	ToolRouterEnabled          bool     `json:"tool_router_enabled"`
	ConfidenceThreshold        float64  `json:"confidence_threshold"`
	RouterModelsMissingCatalog []string `json:"router_models_missing_catalog"`
}

// RoutingFilterFreeTierRequest is POST /api/ui/routing/filter_free_tier_models body.
type RoutingFilterFreeTierRequest struct {
	Enabled bool `json:"enabled"`
}

// RoutingFilterFreeTierResponse is POST /api/ui/routing/filter_free_tier_models success JSON.
type RoutingFilterFreeTierResponse struct {
	OK                   bool `json:"ok"`
	FilterFreeTierModels bool `json:"filter_free_tier_models"`
}

// RoutingPolicySaveRequest is POST /api/ui/routing/policy body.
type RoutingPolicySaveRequest struct {
	RoutingPolicyYAML string `json:"routing_policy_yaml"`
}

// RoutingPolicySaveResponse is POST /api/ui/routing/policy success JSON.
type RoutingPolicySaveResponse struct {
	OK                bool   `json:"ok"`
	Saved             bool   `json:"saved"`
	RoutingPolicyYAML string `json:"routing_policy_yaml"`
}

// RoutingFallbackChainSaveRequest is POST /api/ui/routing/fallback_chain body.
type RoutingFallbackChainSaveRequest struct {
	FallbackChain []string `json:"fallback_chain"`
}

// RoutingFallbackChainSaveResponse is POST /api/ui/routing/fallback_chain success JSON.
type RoutingFallbackChainSaveResponse struct {
	OK            bool     `json:"ok"`
	Saved         bool     `json:"saved"`
	FallbackChain []string `json:"fallback_chain"`
}
