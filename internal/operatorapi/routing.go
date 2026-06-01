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

// RoutingGenerateResponse is virtual-model generate success JSON.
type RoutingGenerateResponse struct {
	OK                  bool                 `json:"ok"`
	Saved               bool                 `json:"saved"`
	FallbackChain       []string             `json:"fallback_chain"`
	RouterModels        []string             `json:"router_models,omitempty"`
	ModelsBrokerCatalog int                  `json:"models_broker_catalog"`
	ModelsUsed          int                  `json:"models_used"`
	RoutingPolicyYAML   string               `json:"routing_policy_yaml"`
	Routing             RoutingPolicySummary `json:"routing,omitempty"`
}

// RoutingEvaluateRequest is POST /api/ui/virtual-models/{id}/routing/evaluate body.
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

// RoutingEvaluateResponse is POST /api/ui/virtual-models/{id}/routing/evaluate success JSON.
type RoutingEvaluateResponse struct {
	OK                  bool                   `json:"ok"`
	InitialModel        string                 `json:"initial_model"`
	Via                 string                 `json:"via"`
	FallbackStartIndex  int                    `json:"fallback_start_index"`
	FallbackFromInitial []string               `json:"fallback_from_initial"`
	SmokeCompletion     *SmokeCompletionResult `json:"smoke_completion,omitempty"`
}
