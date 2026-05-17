package naming

// Naming contracts after final v0.3 cutover.
const (
	// Product-layer names.
	ProductSuiteName      = "Porcelain"
	ProductGatewayName    = "Chimera"
	ProductWorkspaceName  = "Locus"
	ProductSupervisorName = "chimera-supervisor"
	ProductDesktopName    = "locus-desktop"

	// Target contract prefixes (selected in Phase 2).
	TargetEnvPrefix    = "CHIMERA"
	TargetHeaderPrefix = "X-Chimera"

	// Environment-variable contracts.
	EnvGatewayConfigTarget  = "CHIMERA_GATEWAY_CONFIG"
	EnvUpstreamAPIKeyTarget = "CHIMERA_UPSTREAM_API_KEY"
	EnvGatewayURLTarget     = "CHIMERA_GATEWAY_URL"
	EnvGatewayTokenTarget          = "CHIMERA_GATEWAY_TOKEN"
	EnvSupervisorControlURLTarget  = "CHIMERA_SUPERVISOR_CONTROL_URL"

	// Header contracts.
	HeaderProjectTarget                 = "X-Chimera-Project"
	HeaderFlavorTarget                  = "X-Chimera-Flavor-Id"
	HeaderIndexRunTarget                = "X-Chimera-Index-Run-Id"
	HeaderChunkIndexTarget              = "X-Chimera-Chunk-Index"
	HeaderConversationIDTarget          = "X-Chimera-Conversation-Id"
	HeaderRequestFingerprintTarget      = "X-Chimera-Request-Fingerprint"
	HeaderRollingFingerprintTarget      = "X-Chimera-Rolling-Fingerprint"
	HeaderToolRouterTarget              = "X-Chimera-Tool-Router"
	HeaderToolConfidenceThresholdTarget = "X-Chimera-Tool-Confidence-Threshold"

	// Config/file naming contracts.
	PathsAPIKeysKeyTarget = "api_keys"
	APIKeysFileTarget     = "api-keys.yaml"

	// Local hidden state directories.
	IndexerHiddenStateDirTarget = ".locus"
)
