package operatorapi

// VirtualModelSummary is a list entry for GET /api/ui/state and virtual-models list.
type VirtualModelSummary struct {
	ID                   int64    `json:"id"`
	ModelID              string   `json:"model_id"`
	Name                 string   `json:"name"`
	Version              string   `json:"version"`
	Description          string   `json:"description,omitempty"`
	Enabled              bool     `json:"enabled"`
	Visibility           string   `json:"visibility"`
	FallbackDepth        int      `json:"fallback_depth"`
	RoutingPolicyEnabled bool     `json:"routing_policy_enabled"`
	ToolRouterEnabled    bool     `json:"tool_router_enabled"`
	RouterModels         []string `json:"router_models,omitempty"`
}

// VirtualModelDetail is GET /api/ui/virtual-models/{id}.
type VirtualModelDetail struct {
	VirtualModelSummary
	RoutingPolicyYAML    string   `json:"routing_policy_yaml,omitempty"`
	FallbackChain        []string `json:"fallback_chain"`
	FallbackUnavailable  []string `json:"fallback_unavailable,omitempty"`
	ToolRouterConfidence float64  `json:"tool_router_confidence_threshold"`
	CreatedByPrincipalID string   `json:"created_by_principal_id,omitempty"`
	CreatedAt            string   `json:"created_at"`
	UpdatedAt            string   `json:"updated_at"`
}

// VirtualModelCreateRequest is POST /api/ui/virtual-models body.
type VirtualModelCreateRequest struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description,omitempty"`
	Visibility  string `json:"visibility,omitempty"`
	ModelID     string `json:"model_id,omitempty"`
}

// VirtualModelUpdateRequest is PUT /api/ui/virtual-models/{id} body.
type VirtualModelUpdateRequest struct {
	Name        *string `json:"name,omitempty"`
	Version     *string `json:"version,omitempty"`
	Description *string `json:"description,omitempty"`
	Enabled     *bool   `json:"enabled,omitempty"`
	Visibility  *string `json:"visibility,omitempty"`
}

// VirtualModelFallbackSaveRequest is PUT /api/ui/virtual-models/{id}/fallback.
type VirtualModelFallbackSaveRequest struct {
	FallbackChain []string `json:"fallback_chain"`
}

// VirtualModelRoutingPolicySaveRequest is PUT /api/ui/virtual-models/{id}/routing-policy.
type VirtualModelRoutingPolicySaveRequest struct {
	Enabled           bool   `json:"enabled"`
	RoutingPolicyYAML string `json:"routing_policy_yaml"`
}

// VirtualModelToolRouterSaveRequest is PUT /api/ui/virtual-models/{id}/tool-router.
type VirtualModelToolRouterSaveRequest struct {
	Enabled             bool     `json:"tool_router_enabled"`
	RouterModels        []string `json:"router_models"`
	ConfidenceThreshold float64  `json:"confidence_threshold"`
}

// VirtualModelListResponse is GET /api/ui/virtual-models.
type VirtualModelListResponse struct {
	VirtualModels []VirtualModelSummary `json:"virtual_models"`
}

// VirtualModelGenerateRequest is POST /api/ui/virtual-models/{id}/routing/generate.
type VirtualModelGenerateRequest struct {
	ProviderPrefix string `json:"provider_prefix,omitempty"`
	Save           bool   `json:"save"`
}

// RoutingRuleDefinitionSummary is one catalog entry.
type RoutingRuleDefinitionSummary struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Description string `json:"description,omitempty"`
}
