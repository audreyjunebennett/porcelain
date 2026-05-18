package naming

// Naming contracts after final v0.3 cutover.
// Shell scripts mirror binary basenames in scripts/chimera-names.sh.
const (
	// Product-layer names.
	ProductSuiteName       = "Porcelain"
	ProductGatewayName     = "Chimera"
	ProductWorkspaceName   = "Locus"
	ProductSupervisorName  = "chimera-supervisor"
	ProductDesktopName     = "locus-desktop"
	ProductBrokerName          = "chimera-broker"
	ProductBrokerHTTPBinName   = "chimera-broker-http" // supervised BiFrost HTTP upstream
	ProductBifrostHTTPBinName  = "bifrost-http"        // BiFrost build artifact basename (install scripts)
	ProductVectorstoreName     = "chimera-vectorstore"
	ProductQdrantBinName       = "qdrant"
	ProductGatewayBinName      = "chimera-gateway"
	ProductIndexerBinName      = "chimera-indexer"

	// chimera-broker wrapper environment (BROKER__*).
	EnvBrokerListen              = "BROKER__LISTEN"
	EnvBrokerBin                 = "BROKER__BIN"
	EnvBrokerBackend             = "BROKER__BACKEND"
	EnvBrokerEndpoint            = "BROKER__ENDPOINT"
	EnvBrokerDataPath            = "BROKER__DATA_PATH"
	EnvBrokerLogLevel            = "BROKER__LOG_LEVEL"
	EnvBrokerTimeoutsStartup     = "BROKER__TIMEOUTS__STARTUP"
	EnvBrokerTimeoutsShutdown    = "BROKER__TIMEOUTS__SHUTDOWN"
	EnvBrokerChimeraBrokerConfig = "BROKER__CHIMERA_BROKER_CONFIG"
	EnvBrokerChimeraBrokerLogStyle = "BROKER__CHIMERA_BROKER_LOG_STYLE"

	// chimera-broker wrapper defaults.
	DefaultBrokerListen     = "127.0.0.1:7730"
	DefaultBrokerEndpoint   = "127.0.0.1:8080"
	DefaultBrokerDataPath   = "data/chimera-broker"
	DefaultBrokerConfigPath = "config/chimera-broker.config.json"
	DefaultBrokerLogLevel   = "info"
	DefaultBrokerLogStyle   = "json"

	// chimera-vectorstore wrapper environment (VECTORSTORE__*).
	EnvVectorstoreListen           = "VECTORSTORE__LISTEN"
	EnvVectorstoreBin              = "VECTORSTORE__BIN"
	EnvVectorstoreBackend          = "VECTORSTORE__BACKEND"
	EnvVectorstoreEndpoint         = "VECTORSTORE__ENDPOINT"
	EnvVectorstoreDataPath         = "VECTORSTORE__DATA_PATH"
	EnvVectorstoreLogLevel         = "VECTORSTORE__LOG_LEVEL"
	EnvVectorstoreTimeoutsStartup  = "VECTORSTORE__TIMEOUTS__STARTUP"
	EnvVectorstoreTimeoutsShutdown = "VECTORSTORE__TIMEOUTS__SHUTDOWN"
	EnvVectorstoreGRPCPort         = "VECTORSTORE__GRPC_PORT"

	// chimera-vectorstore wrapper defaults.
	DefaultVectorstoreListen   = "127.0.0.1:7740"
	DefaultVectorstoreEndpoint = "127.0.0.1:6333"
	DefaultVectorstoreDataPath = "data/qdrant"
	DefaultVectorstoreLogLevel = "info"
	DefaultVectorstoreGRPCPort = 6334

	// Locus desktop environment variables.
	EnvDesktopTrace  = "LOCUS_DESKTOP_TRACE"
	EnvDesktopLogDir = "LOCUS_DESKTOP_LOG_DIR"

	// Target contract prefixes (selected in Phase 2).
	TargetEnvPrefix    = "CHIMERA"
	TargetHeaderPrefix = "X-Chimera"

	// Environment-variable contracts.
	EnvGatewayConfigTarget        = "CHIMERA_GATEWAY_CONFIG"
	EnvUpstreamAPIKeyTarget       = "CHIMERA_UPSTREAM_API_KEY"
	EnvGatewayURLTarget           = "CHIMERA_GATEWAY_URL"
	EnvGatewayTokenTarget         = "CHIMERA_GATEWAY_TOKEN"
	EnvSupervisorControlURLTarget = "CHIMERA_SUPERVISOR_CONTROL_URL"

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
