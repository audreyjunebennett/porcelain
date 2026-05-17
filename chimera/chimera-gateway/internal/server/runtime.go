package server

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
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/rag"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/rag/embed"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/routing"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/vectorstore/qdrant"
	"github.com/lynn/porcelain/chimera/internal/config"
	"github.com/lynn/porcelain/chimera/internal/providerlimits"
	"github.com/lynn/porcelain/chimera/internal/tokens"
)

// Runtime mirrors src/runtime.ts RuntimeState.
type Runtime struct {
	log              *slog.Logger
	gatewayPath      string
	mu               sync.RWMutex
	gatewayMtime     time.Time
	freeTierMtime    time.Time
	resolved         *config.Resolved
	tokens           *tokens.Store
	routing          *routing.Policy
	metrics          *gatewaymetrics.Store // optional; nil when disabled or init failed
	operator         *operatorstore.Store  // optional; nil when init failed
	upstreamOverride string                // non-empty: after each yaml load, patch upstream base + health (supervised BiFrost)

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

	// catalogSnapshot is the most recent BiFrost `/v1/models` view, refreshed by the periodic
	// poller in cmd/chimera/serve.go. nil until the first refresh completes. See
	// availablemodels.go for the snapshot type and consumers (provider-health classifier and
	// future routing/embedding/router-model auditors).
	catalogSnapshot atomic.Pointer[CatalogSnapshot]

	indexerStatusMu sync.Mutex
	indexerStatus   IndexerSupervisorStatus
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
	return NewRuntimeWithUpstreamOverride(gatewayPath, log, "")
}

// NewRuntimeWithUpstreamOverride loads gateway.yaml; if upstreamOverride is set (e.g. http://127.0.0.1:8080),
// it replaces upstream.base_url and health probe URL on every reload (supervised BiFrost).
func NewRuntimeWithUpstreamOverride(gatewayPath string, log *slog.Logger, upstreamOverride string) (*Runtime, error) {
	res, err := config.LoadGatewayYAML(gatewayPath, log)
	if err != nil {
		return nil, err
	}
	res, err = config.EnsureGeneratedUpstreamAPIKey(gatewayPath, res, log)
	if err != nil {
		return nil, err
	}
	if upstreamOverride != "" {
		res = config.CloneResolved(res)
		config.PatchResolvedUpstream(res, upstreamOverride)
	}
	rt := &Runtime{
		log:              log,
		gatewayPath:      gatewayPath,
		upstreamOverride: upstreamOverride,
		resolved:         res,
		tokens:           tokens.NewStore(res.TokensPath, log),
		routing:          routing.NewPolicy(res.RoutingPolicyPath, log),
		ingestSessions:   newIngestSessionStore(),
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

func (rt *Runtime) applyUpstreamOverride(res *config.Resolved) *config.Resolved {
	if rt.upstreamOverride == "" {
		return res
	}
	cp := config.CloneResolved(res)
	config.PatchResolvedUpstream(cp, rt.upstreamOverride)
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

	next, err := config.LoadGatewayYAML(rt.gatewayPath, rt.log)
	if err != nil {
		if rt.log != nil {
			rt.log.Error("failed to reload gateway.yaml", "msg", "gateway.config.reload_failed", "path", rt.gatewayPath, "err", err)
		}
		return
	}
	pathsChanged := next.TokensPath != rt.resolved.TokensPath ||
		next.RoutingPolicyPath != rt.resolved.RoutingPolicyPath
	rt.resolved = rt.applyUpstreamOverride(next)
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
		rt.routing = routing.NewPolicy(next.RoutingPolicyPath, rt.log)
	}
	if rt.log != nil {
		rt.log.Info("reloaded gateway.yaml", "msg", "gateway.config.reloaded", "path", rt.gatewayPath)
	}
}

func (rt *Runtime) Snapshot() (*config.Resolved, *tokens.Store, *routing.Policy) {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	return rt.resolved, rt.tokens, rt.routing
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

// LimitsGuard returns an admission guard combining the parsed limits spec with live metrics.
// Returns nil when no limits spec is configured or the metrics store is unavailable; callers
// treat nil as "no enforcement".
func (rt *Runtime) LimitsGuard() *providerlimits.Guard {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	if rt.resolved == nil || rt.resolved.ProviderLimitsSpec == nil || rt.metrics == nil {
		return nil
	}
	return &providerlimits.Guard{
		Cfg:   rt.resolved.ProviderLimitsSpec,
		Usage: metricsUsageAdapter{store: rt.metrics},
	}
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

// buildRAGService constructs a Qdrant + embedding-backed Service from resolved
// config. The upstream API key (env-resolved at runtime) is used as the bearer
// for embeddings since the gateway plan colocates embed under the upstream
// LLM proxy in v0.2.
func buildRAGService(res *config.Resolved, log *slog.Logger) (*rag.Service, error) {
	apiKey := strings.TrimSpace(os.Getenv(res.UpstreamAPIKeyEnv))
	if apiKey == "" {
		apiKey = strings.TrimSpace(res.UpstreamAPIKey)
	}
	emb := embed.New(res.RAG.EmbeddingURL(res.UpstreamBaseURL), apiKey, res.RAG.EmbeddingModel)
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

// CatalogSnapshot returns the most recent BiFrost `/v1/models` snapshot, or nil when no poll
// has succeeded yet. The returned pointer aliases the runtime's cached value — callers MUST
// treat it as read-only.
func (rt *Runtime) CatalogSnapshot() *CatalogSnapshot {
	return rt.catalogSnapshot.Load()
}

// SetCatalogSnapshot publishes a new snapshot atomically. Used by [RefreshAvailableModels];
// tests may call it directly to seed a known-good state.
func (rt *Runtime) SetCatalogSnapshot(snap *CatalogSnapshot) {
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
