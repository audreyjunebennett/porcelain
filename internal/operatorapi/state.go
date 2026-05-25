package operatorapi

// StateResponse is GET /api/ui/state.
type StateResponse struct {
	Gateway               GatewayState                  `json:"gateway"`
	Providers             map[string]StateProviderEntry `json:"providers"`
	ConfiguredProviderIDs []string                      `json:"configured_provider_ids,omitempty"`
}

// GatewayState is the gateway section of GET /api/ui/state.
type GatewayState struct {
	Semver                        string          `json:"semver"`
	VirtualModelID                string          `json:"virtual_model_id"`
	PublicBaseURL                 string          `json:"public_base_url"`
	TokenHint                     string          `json:"token_hint"`
	FilterFreeTierModels          bool            `json:"filter_free_tier_models"`
	FallbackChain                 []string        `json:"fallback_chain"`
	RoutingPolicyBasename         string          `json:"routing_policy_basename"`
	RouterModels                  []string        `json:"router_models"`
	ToolRouterEnabled             bool            `json:"tool_router_enabled"`
	ToolRouterConfidenceThreshold float64         `json:"tool_router_confidence_threshold"`
	ToolRouterLastModel           string          `json:"tool_router_last_model"`
	ToolRouterLastError           string          `json:"tool_router_last_error"`
	ToolRouterLastAt              string          `json:"tool_router_last_at"`
	RoutingPolicyYAML             string          `json:"routing_policy_yaml"`
	ServiceOverview               ServiceOverview `json:"service_overview"`
	IndexerSupervisedConfigPath   string          `json:"indexer_supervised_config_path"`
	IndexerSupervisedEnabled      bool            `json:"indexer_supervised_enabled"`
	OperatorSQLitePath            string          `json:"operator_sqlite_path"`
	OperatorStoreOpen             bool            `json:"operator_store_open"`
}

// ServiceOverview is gateway.service_overview in GET /api/ui/state.
type ServiceOverview struct {
	OverallState       string               `json:"overall_state"`
	Gateway            ServiceState         `json:"gateway"`
	ChimeraBroker      ServiceEndpointState `json:"chimera-broker"`
	ChimeraVectorstore VectorstoreState     `json:"chimera-vectorstore"`
	ChimeraIndexer     IndexerOverviewState `json:"chimera-indexer"`
	RefreshedAt        string               `json:"refreshed_at"`
}

// ServiceState is a minimal {state} service block.
type ServiceState struct {
	State string `json:"state"`
}

// ServiceEndpointState is chimera-broker style {state, url, detail}.
type ServiceEndpointState struct {
	State  string `json:"state"`
	URL    string `json:"url,omitempty"`
	Detail string `json:"detail,omitempty"`
}

// VectorstoreState is chimera-vectorstore in service_overview.
type VectorstoreState struct {
	Enabled bool   `json:"enabled"`
	State   string `json:"state"`
	URL     string `json:"url,omitempty"`
}

// IndexerOverviewState is chimera-indexer in service_overview.
type IndexerOverviewState struct {
	Enabled            bool   `json:"enabled"`
	InScope            bool   `json:"in_scope"`
	Worker             string `json:"worker"`
	State              string `json:"state,omitempty"`
	LastHeartbeatAt    string `json:"last_heartbeat_at,omitempty"`
	LastLogAt          string `json:"last_log_at,omitempty"`
	Detail             string `json:"detail,omitempty"`
	SupervisionSignals string `json:"supervision_signals,omitempty"`
}
