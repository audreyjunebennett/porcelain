package runtime

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/gatewaymetrics"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/operatorstore"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/providermodels"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/rag"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/rag/ragembed"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/catalog"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/corpusstale"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/vectorstore/qdrant"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/virtualmodel"
	indexeradapter "github.com/lynn/porcelain/chimera/chimera-indexer/adapter"
	"github.com/lynn/porcelain/chimera/internal/config"
	"github.com/lynn/porcelain/chimera/internal/providerlimits"
	"github.com/lynn/porcelain/chimera/internal/servicelogs"
	"github.com/lynn/porcelain/chimera/internal/tokens"
	"github.com/lynn/porcelain/internal/naming"
)

// Runtime mirrors src/runtime.ts RuntimeState.
type Runtime struct {
	log                   *slog.Logger
	gatewayPath           string
	mu                    sync.RWMutex
	gatewayMtime          time.Time
	freeTierMtime         time.Time
	resolved              *config.Resolved
	tokens                *tokens.Store
	metrics               *gatewaymetrics.Store // optional; nil when disabled or init failed
	operator              *operatorstore.Store  // optional; nil when init failed
	virtualModels         *virtualmodel.Registry
	providerModels        *providermodels.Registry
	brokerBaseURLOverride string // non-empty: after each yaml load, patch broker base + health (supervised chimera-broker)

	toolRouterMu      sync.Mutex
	toolRouterModel   string
	toolRouterAt      time.Time
	toolRouterLastErr string

	// rag is the resolved retrieval-augmented-generation service when
	// res.RAG.Enabled is true. nil when RAG is disabled or init failed.
	rag *rag.Service

	// ingestSessions buffers v0.4 chunked uploads until complete.
	ingestSessions *ingestSessionStore

	// chatTurns counts user turns per conversation_id for structured logs (Phase 3 / 6).
	chatTurnMu sync.Mutex
	chatTurns  map[string]int

	// catalogSnapshot is the most recent chimera-broker `/v1/models` view, refreshed by the periodic
	// poller in cmd/chimera/serve.go. nil until the first refresh completes. See
	// availablemodels.go for the snapshot type and consumers (provider-health classifier and
	// future routing/embedding/router-model auditors).
	catalogSnapshot atomic.Pointer[catalog.CatalogSnapshot]

	indexerStatusMu sync.Mutex
	indexerStatus   IndexerSupervisorStatus

	corpusStaleStore *corpusstale.Store
}

// IndexerSupervisorStatus is the gateway-owned view of supervised indexer process health.
// It is updated by cmd/chimera/serve.go (process lifecycle + parsed indexer.state heartbeats)
// and consumed by /api/ui/state for operator cards.
type IndexerSupervisorStatus struct {
	WorkerState     string
	LastState       string
	LastHeartbeatAt time.Time
	LastLogAt       time.Time
	LastError       string
	UpdatedAt       time.Time
}

func NewRuntime(gatewayPath string, log *slog.Logger) (*Runtime, error) {
	return NewRuntimeWithBrokerOverride(gatewayPath, log, "")
}

// NewRuntimeWithBrokerOverride loads gateway config; if brokerBaseURLOverride is set (e.g. http://127.0.0.1:8080),
// it replaces upstream.base_url and health probe URL on every reload (supervised chimera-broker).
func NewRuntimeWithBrokerOverride(gatewayPath string, log *slog.Logger, brokerBaseURLOverride string) (*Runtime, error) {
	res, err := config.LoadGatewayYAML(gatewayPath, log)
	if err != nil {
		return nil, err
	}
	res, err = config.EnsureGeneratedUpstreamAPIKey(gatewayPath, res, log)
	if err != nil {
		return nil, err
	}
	if brokerBaseURLOverride != "" {
		res = config.CloneResolved(res)
		config.PatchResolvedUpstream(res, brokerBaseURLOverride)
	}
	rt := &Runtime{
		corpusStaleStore: corpusstale.NewStore(),
		log:                   log,
		gatewayPath:           gatewayPath,
		brokerBaseURLOverride: brokerBaseURLOverride,
		resolved:              res,
		tokens:                tokens.NewStore(res.TokensPath, log),
		ingestSessions:        newIngestSessionStore(),
	}
	if res.MetricsEnabled {
		if s, err := gatewaymetrics.Open(res.MetricsSQLitePath, res.MetricsMigrationsDir, log); err != nil {
			if log != nil {
				log.Warn("gateway metrics init failed; continuing without SQLite metrics", "msg", "gateway.metrics.init_failed", "err", err,
					"sqlite", res.MetricsSQLitePath, "migrations_dir", res.MetricsMigrationsDir)
			}
		} else {
			rt.metrics = s
		}
	}
	if s, err := operatorstore.Open(res.OperatorSQLitePath, res.OperatorMigrationsDir, log); err != nil {
		if log != nil {
			log.Warn("operator sqlite init failed; workspace CRUD disabled", "msg", "gateway.operator.init_failed", "err", err,
				"sqlite", res.OperatorSQLitePath, "migrations_dir", res.OperatorMigrationsDir)
		}
	} else {
		rt.operator = s
		if err := operatorstore.ImportSupervisedYAMLRootsIfEmpty(context.Background(), s, "", res.IndexerSupervisedConfigPath, log); err != nil && log != nil {
			log.Warn("legacy supervised roots import failed", "msg", "gateway.operator.workspaces_import_failed", "err", err)
		}
		if err := operatorstore.BootstrapVirtualModels(context.Background(), s, res, log); err != nil {
			if log != nil {
				log.Warn("virtual model bootstrap failed", "msg", "gateway.virtual_model.bootstrap_failed", "err", err)
			}
		} else {
			rt.virtualModels = virtualmodel.NewRegistry()
			if err := rt.virtualModels.Reload(context.Background(), s); err != nil && log != nil {
				log.Warn("virtual model registry reload failed", "msg", "gateway.virtual_model.reload_failed", "err", err)
			}
		}
		rt.providerModels = providermodels.NewRegistry()
		if err := rt.providerModels.Reload(context.Background(), s); err != nil && log != nil {
			log.Warn("provider model registry reload failed", "msg", "gateway.provider_models.reload_failed", "err", err)
		}
		EnsureFallbackAvailabilityCatalogAuditor(rt)
	}
	if res.RAG.Enabled {
		if s, err := buildRAGService(res, log); err != nil {
			if log != nil {
				log.Warn("rag init failed; continuing without RAG", "msg", "gateway.rag.init_failed", "err", err)
			}
		} else {
			rt.rag = s
		}
	}
	if st, err := os.Stat(gatewayPath); err == nil {
		rt.gatewayMtime = st.ModTime()
	}
	if res != nil && res.ProviderFreeTierPath != "" {
		if st, err := os.Stat(res.ProviderFreeTierPath); err == nil {
			rt.freeTierMtime = st.ModTime()
		}
	}
	return rt, nil
}

func (rt *Runtime) applyBrokerBaseURLOverride(res *config.Resolved) *config.Resolved {
	if rt.brokerBaseURLOverride == "" {
		return res
	}
	cp := config.CloneResolved(res)
	config.PatchResolvedUpstream(cp, rt.brokerBaseURLOverride)
	return cp
}

func (rt *Runtime) Sync() {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	gst, err := os.Stat(rt.gatewayPath)
	if err != nil {
		if rt.log != nil {
			rt.log.Error("gateway config missing", "msg", "gateway.config.missing", "path", rt.gatewayPath, "err", err)
		}
		return
	}
	ftPath := ""
	var ftTime time.Time
	if rt.resolved != nil {
		ftPath = rt.resolved.ProviderFreeTierPath
	}
	if ftPath != "" {
		if st, err := os.Stat(ftPath); err == nil {
			ftTime = st.ModTime()
		}
	}
	if gst.ModTime().Equal(rt.gatewayMtime) && ftTime.Equal(rt.freeTierMtime) {
		return
	}

	prev := rt.resolved
	next, err := config.LoadGatewayYAML(rt.gatewayPath, rt.log)
	if err != nil {
		if rt.log != nil {
			rt.log.Error("failed to reload gateway config", "msg", "gateway.config.reload_failed", "path", rt.gatewayPath, "config_file", naming.GatewayConfigFileTarget, "err", err)
		}
		return
	}
	pathsChanged := next.TokensPath != rt.resolved.TokensPath
	next = rt.applyBrokerBaseURLOverride(next)
	rt.resolved = next
	rt.gatewayMtime = gst.ModTime()
	if next.ProviderFreeTierPath != "" {
		if st, err := os.Stat(next.ProviderFreeTierPath); err == nil {
			rt.freeTierMtime = st.ModTime()
		}
	} else {
		rt.freeTierMtime = time.Time{}
	}
	if pathsChanged {
		rt.tokens = tokens.NewStore(next.TokensPath, rt.log)
	}
	if ragServiceConfigChanged(prev, next) {
		rt.rebuildRAGLocked()
	}
	if rt.log != nil {
		rt.log.Info("reloaded gateway config", "msg", "gateway.config.reloaded", "path", rt.gatewayPath, "config_file", naming.GatewayConfigFileTarget)
	}
}

func (rt *Runtime) Snapshot() (*config.Resolved, *tokens.Store) {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	return rt.resolved, rt.tokens
}

// GatewayPath returns the on-disk gateway.yaml path loaded by this runtime.
func (rt *Runtime) GatewayPath() string {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	return rt.gatewayPath
}

func ragServiceConfigChanged(prev, next *config.Resolved) bool {
	if next == nil {
		return false
	}
	if prev == nil {
		return next.RAG.Enabled
	}
	a, b := prev.RAG, next.RAG
	return a.Enabled != b.Enabled ||
		a.EmbeddingBaseURL != b.EmbeddingBaseURL ||
		a.EmbeddingPath != b.EmbeddingPath ||
		a.EmbeddingModel != b.EmbeddingModel ||
		a.EmbeddingDim != b.EmbeddingDim ||
		a.QdrantURL != b.QdrantURL ||
		a.QdrantAPIKey != b.QdrantAPIKey ||
		a.ChunkSize != b.ChunkSize ||
		a.ChunkOverlap != b.ChunkOverlap ||
		a.TopK != b.TopK ||
		a.ScoreThreshold != b.ScoreThreshold
}

func (rt *Runtime) rebuildRAGLocked() {
	if rt.resolved == nil || !rt.resolved.RAG.Enabled {
		rt.rag = nil
		return
	}
	s, err := buildRAGService(rt.resolved, rt.log)
	if err != nil {
		rt.rag = nil
		if rt.log != nil {
			rt.log.Warn("rag reload failed", "msg", "gateway.rag.reload_failed", "err", err)
		}
		return
	}
	rt.rag = s
	if rt.log != nil {
		rt.log.Info("rag service reloaded", "msg", "gateway.rag.reloaded",
			"embedding_model", rt.resolved.RAG.EmbeddingModel,
			"embedding_dim", rt.resolved.RAG.EmbeddingDim,
		)
	}
}

// ReloadRAG rebuilds the in-memory RAG service from the current resolved config.
// Call after mutating gateway.yaml when Sync may not yet observe a new mtime.
func (rt *Runtime) ReloadRAG() {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.rebuildRAGLocked()
}

// NextChatTurnIndex returns the next 1-based turn index for this conversation_id (in-process only).
func (rt *Runtime) NextChatTurnIndex(conversationID string) int {
	if rt == nil || conversationID == "" {
		return 1
	}
	rt.chatTurnMu.Lock()
	defer rt.chatTurnMu.Unlock()
	if rt.chatTurns == nil {
		rt.chatTurns = make(map[string]int)
	}
	rt.chatTurns[conversationID]++
	return rt.chatTurns[conversationID]
}

// Metrics returns the SQLite metrics recorder, or nil when metrics are disabled or failed to open.
func (rt *Runtime) Metrics() gatewaymetrics.Recorder {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	return rt.metrics
}

// MetricsStore returns the metrics SQLite store for admin read APIs, or nil.
func (rt *Runtime) MetricsStore() *gatewaymetrics.Store {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	return rt.metrics
}

// OperatorStore returns the operator SQLite store for workspace persistence, or nil.
func (rt *Runtime) OperatorStore() *operatorstore.Store {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	return rt.operator
}

// SetOperatorStoreForTest assigns the operator SQLite store (tests only).
func (rt *Runtime) SetOperatorStoreForTest(store *operatorstore.Store) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.operator = store
}

// VirtualModels returns the in-memory virtual model registry, or nil when operator store is unavailable.
// PrimaryVirtualModelID returns the first enabled virtual model id from operator SQLite, or "".
func (rt *Runtime) PrimaryVirtualModelID() string {
	if rt == nil {
		return ""
	}
	if reg := rt.VirtualModels(); reg != nil {
		return reg.BootstrapModelID()
	}
	return ""
}

func (rt *Runtime) VirtualModels() *virtualmodel.Registry {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	return rt.virtualModels
}

// ReloadVirtualModels refreshes the registry from operator SQLite and bumps revision.
func (rt *Runtime) ReloadVirtualModels(ctx context.Context) error {
	rt.mu.RLock()
	store := rt.operator
	reg := rt.virtualModels
	rt.mu.RUnlock()
	if store == nil {
		return nil
	}
	if reg == nil {
		reg = virtualmodel.NewRegistry()
		rt.mu.Lock()
		rt.virtualModels = reg
		rt.mu.Unlock()
	}
	return reg.Reload(ctx, store)
}

// ProviderModels returns the in-memory provider model availability registry, or nil when operator store is unavailable.
func (rt *Runtime) ProviderModels() *providermodels.Registry {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	return rt.providerModels
}

// ProviderModelAvailability returns the tenant-scoped availability snapshot (all models available when nil registry).
func (rt *Runtime) ProviderModelAvailability(tenantID string) *providermodels.TenantSnapshot {
	reg := rt.ProviderModels()
	if reg == nil {
		return &providermodels.TenantSnapshot{TenantID: tenantID}
	}
	return reg.Snapshot(tenantID)
}

// ReloadProviderModelAvailability refreshes the provider model registry from operator SQLite.
func (rt *Runtime) ReloadProviderModelAvailability(ctx context.Context) error {
	rt.mu.RLock()
	store := rt.operator
	reg := rt.providerModels
	rt.mu.RUnlock()
	if store == nil {
		return nil
	}
	if reg == nil {
		reg = providermodels.NewRegistry()
		rt.mu.Lock()
		rt.providerModels = reg
		rt.mu.Unlock()
	}
	return reg.Reload(ctx, store)
}

// NoteToolRouterAttempt records the last tool-router upstream call (for admin visibility).
func (rt *Runtime) NoteToolRouterAttempt(model string, err error) {
	rt.toolRouterMu.Lock()
	defer rt.toolRouterMu.Unlock()
	rt.toolRouterModel = strings.TrimSpace(model)
	rt.toolRouterAt = time.Now().UTC()
	rt.toolRouterLastErr = ""
	if err != nil {
		rt.toolRouterLastErr = err.Error()
		if len(rt.toolRouterLastErr) > 220 {
			rt.toolRouterLastErr = rt.toolRouterLastErr[:220]
		}
	}
}

// ToolRouterLast returns the last router attempt metadata (best-effort; in-process only).
func (rt *Runtime) ToolRouterLast() (model string, at time.Time, errMsg string) {
	rt.toolRouterMu.Lock()
	defer rt.toolRouterMu.Unlock()
	return rt.toolRouterModel, rt.toolRouterAt, rt.toolRouterLastErr
}

// SetIndexerSupervisorStatus replaces the in-process indexer supervisor status snapshot.
func (rt *Runtime) SetIndexerSupervisorStatus(st IndexerSupervisorStatus) {
	if rt == nil {
		return
	}
	if st.UpdatedAt.IsZero() {
		st.UpdatedAt = time.Now().UTC()
	}
	rt.indexerStatusMu.Lock()
	rt.indexerStatus = st
	rt.indexerStatusMu.Unlock()
}

// NoteIndexerSupervisorLog records that the supervised indexer emitted a line.
func (rt *Runtime) NoteIndexerSupervisorLog(at time.Time) {
	if rt == nil {
		return
	}
	if at.IsZero() {
		at = time.Now().UTC()
	}
	rt.indexerStatusMu.Lock()
	st := rt.indexerStatus
	if at.After(st.LastLogAt) {
		st.LastLogAt = at
	}
	if st.WorkerState == "" || st.WorkerState == "unknown" || st.WorkerState == "starting" {
		st.WorkerState = "up"
	}
	st.UpdatedAt = time.Now().UTC()
	rt.indexerStatus = st
	rt.indexerStatusMu.Unlock()
}

// NoteIndexerSupervisorHeartbeat records parsed indexer.state heartbeat details.
func (rt *Runtime) NoteIndexerSupervisorHeartbeat(at time.Time, declaredState, workerState string) {
	if rt == nil {
		return
	}
	if at.IsZero() {
		at = time.Now().UTC()
	}
	rt.indexerStatusMu.Lock()
	st := rt.indexerStatus
	st.LastHeartbeatAt = at
	st.LastState = strings.TrimSpace(declaredState)
	ws := strings.TrimSpace(workerState)
	if ws != "" {
		st.WorkerState = ws
	}
	st.UpdatedAt = time.Now().UTC()
	rt.indexerStatus = st
	rt.indexerStatusMu.Unlock()
}

// IndexerSupervisorStatus returns a copy of the latest in-process snapshot.
func (rt *Runtime) IndexerSupervisorStatus() IndexerSupervisorStatus {
	if rt == nil {
		return IndexerSupervisorStatus{}
	}
	rt.indexerStatusMu.Lock()
	defer rt.indexerStatusMu.Unlock()
	return rt.indexerStatus
}

// NoteIndexerSupervisorFromLogEntry updates supervised-indexer health from a mirrored log line.
func (rt *Runtime) NoteIndexerSupervisorFromLogEntry(ent servicelogs.Entry) {
	if rt == nil {
		return
	}
	src := strings.TrimSpace(ent.Source)
	if src != servicelogs.SourceChimeraIndexer && src != "indexer" {
		return
	}
	at := ent.Time
	if at.IsZero() {
		at = time.Now().UTC()
	}
	if declared, worker, ok := indexeradapter.ParseSupervisorHeartbeat(ent.Text); ok {
		rt.NoteIndexerSupervisorHeartbeat(at, declared, worker)
		return
	}
	rt.NoteIndexerSupervisorLog(at)
}

// LimitsGuard returns an admission guard combining the parsed limits spec with live metrics and
// optional catalog context overlay. Returns nil when no limits spec is configured. Context
// admission runs even when the metrics store is unavailable; RPM/TPM require metrics.
func (rt *Runtime) LimitsGuard() *providerlimits.Guard {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	if rt.resolved == nil || rt.resolved.ProviderLimitsSpec == nil {
		return nil
	}
	g := &providerlimits.Guard{
		Cfg: rt.resolved.ProviderLimitsSpec,
	}
	if rt.metrics != nil {
		g.Usage = metricsUsageAdapter{store: rt.metrics}
	}
	if snap := rt.catalogSnapshot.Load(); snap != nil && snap.OK && snap.IsFresh(time.Now(), catalog.CatalogSnapshotFreshness) {
		g.Catalog = snap
	}
	return g
}

// metricsUsageAdapter wraps *gatewaymetrics.Store so it satisfies providerlimits.UsageSource
// without providerlimits importing the SQLite package.
type metricsUsageAdapter struct{ store *gatewaymetrics.Store }

func (a metricsUsageAdapter) UsageForModelWindow(ctx context.Context, modelID string, start, end time.Time) (int64, int64, error) {
	if a.store == nil {
		return 0, 0, nil
	}
	u, err := a.store.UsageForModelWindow(ctx, modelID, start, end)
	if err != nil {
		return 0, 0, err
	}
	return u.Calls, u.EstTokens, nil
}

// CloseMetrics closes the SQLite metrics store if it was opened (tests and graceful shutdown).
func (rt *Runtime) CloseMetrics() {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.metrics != nil {
		_ = rt.metrics.Close()
		rt.metrics = nil
	}
}

// CloseOperator closes the operator SQLite store if it was opened (tests and graceful shutdown).
func (rt *Runtime) CloseOperator() {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.operator != nil {
		_ = rt.operator.Close()
		rt.operator = nil
	}
}

// RAG returns the RAG service when enabled, else nil.
// CorpusStaleStore returns the in-memory indexer stale-source snapshot store.
func (rt *Runtime) CorpusStaleStore() *corpusstale.Store {
	if rt == nil {
		return nil
	}
	return rt.corpusStaleStore
}

func (rt *Runtime) RAG() *rag.Service {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	return rt.rag
}

// SetRAGForTest replaces the RAG service (tests only). Production code uses
// buildRAGService via NewRuntime.
func (rt *Runtime) SetRAGForTest(s *rag.Service) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.rag = s
}

// buildRAGService constructs a vectorstore + embedding-backed Service from resolved
// config. The upstream API key (env-resolved at runtime) is used as the bearer
// for embeddings since the gateway plan colocates embed under the upstream
// LLM proxy in v0.2.
func buildRAGService(res *config.Resolved, log *slog.Logger) (*rag.Service, error) {
	apiKey := strings.TrimSpace(os.Getenv(res.UpstreamAPIKeyEnv))
	if apiKey == "" {
		apiKey = strings.TrimSpace(res.UpstreamAPIKey)
	}
	emb := ragembed.New(res.RAG.EmbeddingURL(res.UpstreamBaseURL), apiKey, res.RAG.EmbeddingModel)
	store := qdrant.New(res.RAG.QdrantURL, res.RAG.QdrantAPIKey)
	return rag.New(rag.Options{
		Store:          store,
		Embedder:       emb,
		ChunkSize:      res.RAG.ChunkSize,
		ChunkOverlap:   res.RAG.ChunkOverlap,
		TopK:           res.RAG.TopK,
		ScoreThreshold: float32(res.RAG.ScoreThreshold),
		EmbeddingDim:   res.RAG.EmbeddingDim,
		Log:            log,
	})
}

// CatalogSnapshot returns the most recent chimera-broker `/v1/models` snapshot, or nil when no poll
// has succeeded yet. The returned pointer aliases the runtime's cached value — callers MUST
// treat it as read-only.
func (rt *Runtime) CatalogSnapshot() *catalog.CatalogSnapshot {
	return rt.catalogSnapshot.Load()
}

// SetCatalogSnapshot publishes a new snapshot atomically. Used by [RefreshAvailableModels];
// tests may call it directly to seed a known-good state.
func (rt *Runtime) SetCatalogSnapshot(snap *catalog.CatalogSnapshot) {
	rt.catalogSnapshot.Store(snap)
}

func (rt *Runtime) UpstreamAPIKey() string {
	rt.mu.RLock()
	r := rt.resolved
	rt.mu.RUnlock()
	if r == nil {
		return ""
	}
	if v := strings.TrimSpace(os.Getenv(r.UpstreamAPIKeyEnv)); v != "" {
		return v
	}
	return strings.TrimSpace(r.UpstreamAPIKey)
}
