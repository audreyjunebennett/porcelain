package config

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/lynn/porcelain/chimera/internal/providerfreetier"
	"github.com/lynn/porcelain/chimera/internal/providerlimits"
	"github.com/lynn/porcelain/internal/naming"
	"gopkg.in/yaml.v3"
)

// Resolved matches TypeScript ResolvedGatewayConfig (src/config.ts).
type Resolved struct {
	Semver                string
	VirtualModelID        string
	ListenPort            int
	ListenHost            string
	LogLevel              string
	ChimeraBrokerLogLevel string // supervised chimera-broker-http -log-level (upstream.chimera_broker_log_level); `-chimera-broker-log-level` overrides when non-empty.
	UpstreamBaseURL       string
	UpstreamAPIKeyEnv     string
	// UpstreamAPIKey is the Bearer token from gateway.yaml (upstream.api_key). Non-empty process env named by UpstreamAPIKeyEnv overrides at runtime.
	UpstreamAPIKey    string
	HealthUpstreamURL string
	HealthTimeoutMs   int
	ChatTimeoutMs     int
	// AvailableModelsPollMs is the period for the BiFrost `/v1/models` catalog poller that
	// drives the Provider health strip and future routing/embedding/router-model auditors.
	// 0 disables polling (one-shot startup refresh only). See internal/server/availablemodels.go.
	AvailableModelsPollMs int
	TokensPath            string
	RoutingPolicyPath     string
	FallbackChain         []string
	GatewayYAMLPath       string
	// ProviderFreeTierPath is the resolved filesystem path to provider-free-tier.yaml.
	ProviderFreeTierPath string
	// FilterFreeTierModels requests intersecting merged /v1/models with the allowlist when spec loaded.
	FilterFreeTierModels bool
	ProviderFreeTierSpec *providerfreetier.Spec
	// Metrics (G6): SQLite under data/gateway; see docs/plans/version-v0.1.1.md §3.6.
	MetricsEnabled       bool
	MetricsSQLitePath    string // absolute path to metrics.sqlite
	MetricsMigrationsDir string // absolute path to migrations/chimera-gateway/metrics directory
	// Operator SQLite: workspaces for supervised indexer (separate from metrics).
	OperatorSQLitePath    string
	OperatorMigrationsDir string
	// Provider/model limits (G5 / §3.7). Path is always resolved; Spec is non-nil (empty when
	// file is missing or blank).
	ProviderLimitsPath string
	ProviderLimitsSpec *providerlimits.Config
	// RouterModels is an ordered list of upstream model ids used for the tool-router transformer
	// (see docs/plans/version-v0.1.1.md). Empty disables router calls.
	RouterModels []string
	// ToolRouterEnabled gates the tool-slimming transformer when RouterModels is non-empty.
	// When RouterModels is empty, the transformer never runs regardless of this flag.
	ToolRouterEnabled bool
	// ToolRouterConfidenceThreshold keeps tools with confidence >= threshold (0–1).
	ToolRouterConfidenceThreshold float64
	// RAG holds gateway v0.2 retrieval-augmented-generation settings; RAG.Enabled
	// gates ingest, indexer REST, retrieval, and the /health Qdrant probe.
	RAG RAG

	// IndexerSupervised* configures optional chimera-index child under chimera serve / desktop (v0.5).
	IndexerSupervisedEnabled              bool
	IndexerSupervisedBin                  string // empty → resolve next to chimera binary or PATH
	IndexerSupervisedConfigPath           string // absolute path to single merged --config file
	IndexerSupervisedStartWhenRAGDisabled bool
	IndexerSupervisedLogJSON              bool

	// ConversationMerge joins chat requests into one logical conversation_id per tenant
	// and RAG scope using embeddings + similarity (requires metrics SQLite + embeddings API).
	ConversationMerge ConversationMerge

	// WitnessSampleMaxChars caps head/tail runes for conversation.payload.sample (Phase 8).
	// When zero, defaults to 256 in WitnessSampleMaxRunes().
	WitnessSampleMaxChars int
	// WitnessSampleForceAtDebug enables payload samples at debug log level (still redacted).
	// Trace log level always enables payload samples when the gateway logger is configured for trace.
	WitnessSampleForceAtDebug bool
}

// ShouldEmitPayloadSample reports whether conversation.payload.sample may be emitted
// (trace log level, or debug with WitnessSampleForceAtDebug).
func (r *Resolved) ShouldEmitPayloadSample() bool {
	if r == nil {
		return false
	}
	ll := strings.ToLower(strings.TrimSpace(r.LogLevel))
	if ll == "trace" {
		return true
	}
	return r.WitnessSampleForceAtDebug && (ll == "debug" || ll == "trace")
}

// WitnessSampleMaxRunes returns the configured max runes per head/tail for payload samples.
func (r *Resolved) WitnessSampleMaxRunes() int {
	if r == nil || r.WitnessSampleMaxChars <= 0 {
		return 256
	}
	if r.WitnessSampleMaxChars > 4096 {
		return 4096
	}
	if r.WitnessSampleMaxChars < 32 {
		return 32
	}
	return r.WitnessSampleMaxChars
}

type upstreamBlock struct {
	BaseURL               string `yaml:"base_url"`
	APIKeyEnv             string `yaml:"api_key_env"`
	APIKey                string `yaml:"api_key"`
	ChimeraBrokerLogLevel string `yaml:"chimera_broker_log_level"`
}

type gatewayDoc struct {
	Gateway struct {
		Semver     string `yaml:"semver"`
		ListenPort int    `yaml:"listen_port"`
		ListenHost string `yaml:"listen_host"`
		LogLevel   string `yaml:"log_level"`
		LogWitness struct {
			PayloadSampleMaxChars     *int  `yaml:"payload_sample_max_chars"`
			ForcePayloadSampleAtDebug *bool `yaml:"force_payload_sample_at_debug"`
		} `yaml:"log_witness"`
	} `yaml:"gateway"`
	Upstream upstreamBlock `yaml:"upstream"`
	Health   struct {
		UpstreamURL           string `yaml:"upstream_url"`
		TimeoutMs             int    `yaml:"timeout_ms"`
		ChatMs                int    `yaml:"chat_timeout_ms"`
		AvailableModelsPollMs int    `yaml:"available_models_poll_ms"`
	} `yaml:"health"`
	Paths struct {
		APIKeys             string `yaml:"api_keys"`
		RoutingPolicy       string `yaml:"routing_policy"`
		ProviderFreeTier    string `yaml:"provider_free_tier"`
		ProviderModelLimits string `yaml:"provider_model_limits"`
	} `yaml:"paths"`
	Routing struct {
		FallbackChain        []string `yaml:"fallback_chain"`
		FilterFreeTierModels *bool    `yaml:"filter_free_tier_models"`
		RouterModels         []string `yaml:"router_models"`
		ToolRouter           struct {
			Enabled             *bool    `yaml:"enabled"`
			ConfidenceThreshold *float64 `yaml:"confidence_threshold"`
		} `yaml:"tool_router"`
	} `yaml:"routing"`
	Metrics struct {
		Enabled       *bool  `yaml:"enabled"`
		SQLitePath    string `yaml:"sqlite_path"`
		MigrationsDir string `yaml:"migrations_dir"`
	} `yaml:"metrics"`
	Operator struct {
		SQLitePath    string `yaml:"sqlite_path"`
		MigrationsDir string `yaml:"migrations_dir"`
	} `yaml:"operator"`
	RAG ragDoc `yaml:"rag"`

	Indexer struct {
		Supervised struct {
			Enabled              *bool  `yaml:"enabled"`
			Bin                  string `yaml:"bin"`
			ConfigPath           string `yaml:"config_path"`
			StartWhenRAGDisabled *bool  `yaml:"start_when_rag_disabled"`
			LogJSON              *bool  `yaml:"log_json"`
		} `yaml:"supervised"`
	} `yaml:"indexer"`

	ConversationMerge conversationMergeDoc `yaml:"conversation_merge"`
}

const (
	defaultSemver          = "0.1.0"
	defaultListenPort      = 3000
	defaultListenHost      = "0.0.0.0"
	defaultLogLevel        = "info"
	defaultBaseURL         = "http://chimera-broker:8080"
	defaultAPIKeyEnv       = naming.EnvBrokerAPIKeyTarget
	defaultHealthTimeoutMs = 5000
	defaultChatTimeoutMs   = 300_000
	// defaultAvailableModelsPollMs polls BiFrost `/v1/models` every 30s. Set to 0 in
	// gateway.yaml (`health.available_models_poll_ms`) to disable periodic polling.
	defaultAvailableModelsPollMs = 30_000
)

// LoadGatewayYAML reads and parses gateway.yaml at filePath (absolute or cwd-relative).
func LoadGatewayYAML(filePath string, log *slog.Logger) (*Resolved, error) {
	raw, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	var doc gatewayDoc
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parse gateway yaml: %w", err)
	}

	semver := doc.Gateway.Semver
	if semver == "" {
		semver = defaultSemver
	}

	upBase := strings.TrimSuffix(doc.Upstream.BaseURL, "/")
	if upBase == "" {
		upBase = strings.TrimSuffix(defaultBaseURL, "/")
	}

	apiKeyEnv := doc.Upstream.APIKeyEnv
	if apiKeyEnv == "" {
		apiKeyEnv = defaultAPIKeyEnv
	}

	apiKey := strings.TrimSpace(doc.Upstream.APIKey)

	healthURL := strings.TrimSpace(doc.Health.UpstreamURL)
	if healthURL == "" {
		healthURL = upBase + "/health"
	}

	baseDir := filepath.Dir(filePath)
	apiKeysRel := strings.TrimSpace(doc.Paths.APIKeys)
	if apiKeysRel == "" {
		apiKeysRel = "./" + naming.APIKeysFileTarget
	}
	routeRel := doc.Paths.RoutingPolicy
	if routeRel == "" {
		routeRel = "./routing-policy.yaml"
	}
	tokensPath := filepath.Join(baseDir, apiKeysRel)
	if filepath.IsAbs(apiKeysRel) {
		tokensPath = apiKeysRel
	}
	routingPath := filepath.Join(baseDir, routeRel)
	if filepath.IsAbs(routeRel) {
		routingPath = routeRel
	}

	ftRel := strings.TrimSpace(doc.Paths.ProviderFreeTier)
	if ftRel == "" {
		ftRel = "./provider-free-tier.yaml"
	}
	ftPath := filepath.Join(baseDir, ftRel)
	if filepath.IsAbs(ftRel) {
		ftPath = ftRel
	}
	var ftSpec *providerfreetier.Spec
	if st, err := os.Stat(ftPath); err == nil && !st.IsDir() {
		s, err := providerfreetier.Load(ftPath)
		if err != nil {
			if log != nil {
				log.Error("provider free tier yaml invalid", "msg", "chat.provider_limits.config_invalid", "path", ftPath, "err", err)
			}
		} else {
			ftSpec = s
		}
	} else if err != nil && !os.IsNotExist(err) && log != nil {
		log.Warn("provider free tier path not stat-able", "msg", "chat.provider_limits.config_missing", "path", ftPath, "err", err)
	}

	limitsRel := strings.TrimSpace(doc.Paths.ProviderModelLimits)
	if limitsRel == "" {
		limitsRel = "./provider-model-limits.yaml"
	}
	limitsPath := filepath.Join(baseDir, limitsRel)
	if filepath.IsAbs(limitsRel) {
		limitsPath = limitsRel
	}
	limitsSpec, err := providerlimits.LoadOrEmpty(limitsPath)
	if err != nil {
		if log != nil {
			log.Error("provider-model-limits.yaml invalid; using empty spec (no enforcement)", "msg", "chat.provider_limits.config_invalid", "path", limitsPath, "err", err)
		}
		limitsSpec = &providerlimits.Config{}
	}

	filterFT := true
	if doc.Routing.FilterFreeTierModels != nil {
		filterFT = *doc.Routing.FilterFreeTierModels
	}
	if filterFT && ftSpec == nil && log != nil {
		log.Warn("routing.filter_free_tier_models is true but provider-free-tier.yaml missing or invalid; skipping catalog filter", "msg", "chat.provider_limits.config_missing")
	}

	listenPort := doc.Gateway.ListenPort
	if listenPort == 0 {
		listenPort = defaultListenPort
	}
	listenHost := doc.Gateway.ListenHost
	if listenHost == "" {
		listenHost = defaultListenHost
	}

	ht := doc.Health.TimeoutMs
	if ht == 0 {
		ht = defaultHealthTimeoutMs
	}
	ct := doc.Health.ChatMs
	if ct == 0 {
		ct = defaultChatTimeoutMs
	}
	// Negative explicitly disables; zero falls through to the default. The poller treats <=0
	// as "no periodic refresh" (one-shot startup only).
	availPoll := doc.Health.AvailableModelsPollMs
	if availPoll == 0 {
		availPoll = defaultAvailableModelsPollMs
	}
	if availPoll < 0 {
		availPoll = 0
	}

	chain := doc.Routing.FallbackChain
	if chain == nil {
		chain = []string{}
	}
	if len(chain) == 0 && log != nil {
		log.Warn("routing.fallback_chain is empty or missing; virtual model requests will fail until configured", "msg", "routing.fallback_chain.empty")
	}

	routerModels := doc.Routing.RouterModels
	if routerModels == nil {
		routerModels = []string{}
	}
	toolRouterOn := len(routerModels) > 0
	if doc.Routing.ToolRouter.Enabled != nil {
		toolRouterOn = *doc.Routing.ToolRouter.Enabled && len(routerModels) > 0
	}
	toolThresh := 0.5
	if doc.Routing.ToolRouter.ConfidenceThreshold != nil {
		toolThresh = *doc.Routing.ToolRouter.ConfidenceThreshold
	}

	metricsEnabled := true
	if doc.Metrics.Enabled != nil {
		metricsEnabled = *doc.Metrics.Enabled
	}
	sqliteRel := strings.TrimSpace(doc.Metrics.SQLitePath)
	if sqliteRel == "" {
		sqliteRel = filepath.Join("..", "data", "gateway", "metrics.sqlite")
	}
	metricsSQLite := filepath.Join(baseDir, sqliteRel)
	if filepath.IsAbs(sqliteRel) {
		metricsSQLite = sqliteRel
	}
	migRel := strings.TrimSpace(doc.Metrics.MigrationsDir)
	if migRel == "" {
		migRel = filepath.Join("..", "migrations", "chimera-gateway", "metrics")
	}
	metricsMig := filepath.Join(baseDir, migRel)
	if filepath.IsAbs(migRel) {
		metricsMig = migRel
	}

	opSqliteRel := strings.TrimSpace(doc.Operator.SQLitePath)
	if opSqliteRel == "" {
		opSqliteRel = filepath.Join("..", "data", "gateway", "operator.sqlite")
	}
	operatorSQLite := filepath.Join(baseDir, opSqliteRel)
	if filepath.IsAbs(opSqliteRel) {
		operatorSQLite = opSqliteRel
	}
	opMigRel := strings.TrimSpace(doc.Operator.MigrationsDir)
	if opMigRel == "" {
		opMigRel = filepath.Join("..", "migrations", "chimera-gateway", "operator")
	}
	operatorMig := filepath.Join(baseDir, opMigRel)
	if filepath.IsAbs(opMigRel) {
		operatorMig = opMigRel
	}

	logLevel := doc.Gateway.LogLevel
	if logLevel == "" {
		logLevel = defaultLogLevel
	}
	chimeraBrokerLogLevel := strings.TrimSpace(doc.Upstream.ChimeraBrokerLogLevel)

	witnessMax := 256
	if doc.Gateway.LogWitness.PayloadSampleMaxChars != nil && *doc.Gateway.LogWitness.PayloadSampleMaxChars > 0 {
		witnessMax = *doc.Gateway.LogWitness.PayloadSampleMaxChars
	}
	witnessForceDebug := false
	if doc.Gateway.LogWitness.ForcePayloadSampleAtDebug != nil {
		witnessForceDebug = *doc.Gateway.LogWitness.ForcePayloadSampleAtDebug
	}

	rag := doc.RAG.effective()
	if err := rag.Validate(); err != nil {
		if log != nil {
			log.Error("rag config invalid; disabling RAG", "msg", "rag.config.invalid", "err", err)
		}
		rag = RAG{Enabled: false}
	}

	mergeCfg := conversationMergeEffective(doc.ConversationMerge)

	idxSupEnabled := doc.Indexer.Supervised.Enabled != nil && *doc.Indexer.Supervised.Enabled
	idxStartWhenRAGOff := doc.Indexer.Supervised.StartWhenRAGDisabled != nil && *doc.Indexer.Supervised.StartWhenRAGDisabled
	idxLogJSON := doc.Indexer.Supervised.LogJSON != nil && *doc.Indexer.Supervised.LogJSON
	idxCfgRel := strings.TrimSpace(doc.Indexer.Supervised.ConfigPath)
	if idxCfgRel == "" {
		// Same layout as metrics.sqlite: repo `config/` → data under ../data/gateway/
		idxCfgRel = filepath.Join("..", "data", "gateway", "indexer.supervised.yaml")
	}
	idxCfgPath := filepath.Join(baseDir, idxCfgRel)
	if filepath.IsAbs(idxCfgRel) {
		idxCfgPath = idxCfgRel
	}

	if log != nil {
		log.Info("gateway config resolved", "msg", "gateway.startup.config_resolved",
			"filePath", filePath, "api_keys_path", tokensPath, "routingPolicyPath", routingPath)
	}

	return &Resolved{
		Semver:                                semver,
		VirtualModelID:                        "Chimera-" + semver,
		ListenPort:                            listenPort,
		ListenHost:                            listenHost,
		LogLevel:                              logLevel,
		ChimeraBrokerLogLevel:                 chimeraBrokerLogLevel,
		UpstreamBaseURL:                       upBase,
		UpstreamAPIKeyEnv:                     apiKeyEnv,
		UpstreamAPIKey:                        apiKey,
		HealthUpstreamURL:                     healthURL,
		HealthTimeoutMs:                       ht,
		ChatTimeoutMs:                         ct,
		AvailableModelsPollMs:                 availPoll,
		TokensPath:                            tokensPath,
		RoutingPolicyPath:                     routingPath,
		FallbackChain:                         chain,
		GatewayYAMLPath:                       filePath,
		ProviderFreeTierPath:                  ftPath,
		FilterFreeTierModels:                  filterFT,
		ProviderFreeTierSpec:                  ftSpec,
		MetricsEnabled:                        metricsEnabled,
		MetricsSQLitePath:                     metricsSQLite,
		MetricsMigrationsDir:                  metricsMig,
		OperatorSQLitePath:                    operatorSQLite,
		OperatorMigrationsDir:                 operatorMig,
		ProviderLimitsPath:                    limitsPath,
		ProviderLimitsSpec:                    limitsSpec,
		RouterModels:                          routerModels,
		ToolRouterEnabled:                     toolRouterOn,
		ToolRouterConfidenceThreshold:         toolThresh,
		RAG:                                   rag,
		ConversationMerge:                     mergeCfg,
		WitnessSampleMaxChars:                 witnessMax,
		WitnessSampleForceAtDebug:             witnessForceDebug,
		IndexerSupervisedEnabled:              idxSupEnabled,
		IndexerSupervisedBin:                  strings.TrimSpace(doc.Indexer.Supervised.Bin),
		IndexerSupervisedConfigPath:           idxCfgPath,
		IndexerSupervisedStartWhenRAGDisabled: idxStartWhenRAGOff,
		IndexerSupervisedLogJSON:              idxLogJSON,
	}, nil
}

// ResolveGatewayConfigPath returns CHIMERA gateway config env var or ./config/gateway.yaml relative to cwd.
func ResolveGatewayConfigPath() (string, error) {
	if e := strings.TrimSpace(os.Getenv(naming.EnvGatewayConfigTarget)); e != "" {
		return filepath.Clean(e), nil
	}
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return filepath.Join(wd, naming.GatewayConfigDirTarget, naming.GatewayConfigFileTarget), nil
}

// ListenAddr returns "host:port" for net.Listen.
func (r *Resolved) ListenAddr() string {
	return fmt.Sprintf("%s:%d", r.ListenHost, r.ListenPort)
}

// ShouldApplyFreeTierCatalogFilter reports whether merged /v1/models should list only allowlisted upstream ids.
func (r *Resolved) ShouldApplyFreeTierCatalogFilter() bool {
	return r != nil && r.FilterFreeTierModels && r.ProviderFreeTierSpec != nil && !r.ProviderFreeTierSpec.Empty()
}
