package indexer

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lynn/porcelain/internal/naming"
)

// Indexer ties together discovery, watching, the worker pool, and the
// gateway client into a single supervised process. Build one with New, then
// call Run.
type Indexer struct {
	cfg    Resolved
	client *GatewayClient
	log    *slog.Logger

	queue    *Queue
	matchers map[string]*Matcher

	hooks     Hooks
	syncState *SyncState
	lastGW    atomic.Pointer[IndexerConfig]
	// remoteInv is populated from GET /v1/indexer/corpus/inventory during the
	// initial scan (nil when unavailable). Keys are CorpusInventoryKey(project, flavor, rel).
	remoteInv map[string]CorpusInventoryRow

	// Operator-facing counters (indexer.* structured events / run.done rollup).
	opsSkipCorpusClientHash int64
	opsSkipCorpusSyncMatch  int64
	opsSkipLocalSync        int64
	opsSkipEmpty            int64
	skipSummary             *skipSummaryTracker
	ingestSummary           *ingestSummaryTracker
	opsIngestOK             int64
	opsIngestFail           int64
	opsRetry                int64
	opsDequeued             int64

	ingestInflight       atomic.Int32
	inRecovery           atomic.Bool
	ingestGate           *ingestGate
	initialScanCompleted atomic.Bool
	qdrantPoints         atomic.Int64

	// pendingBulkByScope counts tier-1 bulk ingest jobs queued from fan-out (fair-share).
	pendingBulkMu      sync.Mutex
	pendingBulkByScope map[string]int64

	// workspaceFilesByScope approximates tracked files per (project, flavor) after scans (+/- watchers).
	workspaceFilesMu      sync.Mutex
	workspaceFilesByScope map[string]int64
	activeFileLogMu       sync.Mutex
	lastActiveFilePath    map[string]string
	lastActiveFileEmit    map[string]time.Time

	runWatchMode           atomic.Bool
	scopeStatusEmitMu      sync.Mutex
	lastScopeStatusEmitted map[string]scopeStatusEmitted
	lastGlobalScopeStatus  scopeStatusGlobals
	embedReasonMu          sync.Mutex
	lastEmbedReasonCode    string

	queueSnapMu         sync.Mutex
	lastQueueSnapFP     string
	lastQueueSnapInfoAt time.Time

	obsMu              sync.Mutex
	lastStorageStatsFP map[string]string
	lastStateFP        string
	autoRepairDone     map[string]bool

	// Dynamic supervised roots (Phase 3): updated without restarting RunWatchers.
	rootsMu      sync.RWMutex
	cfgMu        sync.RWMutex
	rootUpdates  chan rootsDelta
	watchMu      sync.Mutex
	watchedPaths map[string][]string // root id -> fsnotify-registered directory paths
}

// Hooks is an optional set of callbacks tests can install to observe and
// influence the Indexer without wiring real fsnotify or gateway calls.
type Hooks struct {
	// AfterIngest fires once a Job successfully ingests, with the gateway's
	// response.
	AfterIngest func(Job, *IngestResponse)
	// OnSkip fires once per file the walker rejects (binary, ignored,
	// oversize, unreadable).
	OnSkip func(rel, reason string)
	// Now overrides time.Now (sleep timing still uses real clock).
	Now func() time.Time
}

// New constructs an Indexer. The provided log may be nil; a discard logger
// is installed in that case.
func New(cfg Resolved, client *GatewayClient, log *slog.Logger) *Indexer {
	if log == nil {
		log = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	}
	st, err := OpenSyncState(cfg.SyncStatePath)
	if err != nil {
		log.Warn("could not open sync state; continuing without skip cache",
			"path", cfg.SyncStatePath, "store", ResolveSyncStateSQLitePath(cfg.SyncStatePath), "err", err)
		st = nil
	}
	return &Indexer{
		cfg:           cfg,
		client:        client,
		log:           log,
		queue:         NewQueue(cfg.QueueDepth),
		matchers:      map[string]*Matcher{},
		syncState:     st,
		ingestGate:    newIngestGate(),
		skipSummary:   newSkipSummaryTracker(),
		ingestSummary: newIngestSummaryTracker(),
	}
}

// Close releases resources held by the indexer (sync-state SQLite).
func (ix *Indexer) Close() error {
	if ix == nil || ix.syncState == nil {
		return nil
	}
	return ix.syncState.Close()
}

// SetHooks installs test hooks. Must be called before Run.
func (ix *Indexer) SetHooks(h Hooks) { ix.hooks = h }

// Queue exposes the internal queue (read-only intent; tests inspect Len).
func (ix *Indexer) Queue() *Queue { return ix.queue }

// FetchAndLogConfig calls GET /v1/indexer/config and logs version-skew info.
// Transient failures are logged but do not abort the indexer; a 503 caused by
// the gateway having RAG turned off is surfaced as a fatal error so operators
// see the actionable message instead of watching workers retry forever.
func (ix *Indexer) FetchAndLogConfig(ctx context.Context) (*IndexerConfig, error) {
	cfg, err := ix.client.FetchConfig(ctx, ix.cfg.DefaultIndexerHeaders())
	if err != nil {
		var he *HTTPError
		if errors.As(err, &he) && he.Status == 503 && strings.Contains(strings.ToLower(he.Body), "rag is not enabled") {
			ix.log.Error("gateway has RAG disabled; nothing for the indexer to do",
				"hint", "set rag.enabled=true in config/gateway.yaml and restart the chimera gateway",
				"body", he.Body)
			return nil, err
		}
		ix.log.Warn("fetch indexer config failed", "err", err)
		return nil, err
	}
	ix.lastGW.Store(cfg)
	targetKeys := DistinctIndexerTargetKeys(ix.cfg, cfg)
	var withArgs []any
	if tid := strings.TrimSpace(cfg.TenantID); tid != "" {
		withArgs = append(withArgs, "tenant_id", tid, "principal_id", tid)
	}
	if ul := strings.TrimSpace(cfg.UserLabel); ul != "" {
		withArgs = append(withArgs, "user_label", ul)
	}
	// Single-ingest-scope processes get a stable log.With indexer_key. Multi-scope
	// configs (distinct project/flavor pairs across roots) omit it so /ui/settings can
	// partition by indexer_target_key from indexer.run.start root_scopes and job rows.
	if len(targetKeys) == 1 {
		withArgs = append(withArgs, "indexer_key", targetKeys[0])
	} else if len(targetKeys) > 1 {
		withArgs = append(withArgs, "indexer_multi_target", true)
	}
	ix.log = ix.log.With(withArgs...)

	logArgs := []any{
		"msg", "gateway.indexer.config",
		"gateway_version", cfg.GatewayVersion,
		"embedding_model", cfg.EmbeddingModel,
		"embedding_dim", cfg.EmbeddingDim,
		"chunk_size", cfg.ChunkSize,
		"chunk_overlap", cfg.ChunkOverlap,
		"max_ingest_bytes", cfg.MaxIngestBytes,
		"max_whole_file_bytes", cfg.MaxWholeFileBytes,
		"ingest_session_path", cfg.IngestSessionPath,
		"corpus_inventory_path", cfg.CorpusInventoryPath,
	}
	if hdr := ix.cfg.DefaultIndexerHeaders(); hdr != nil {
		if v := strings.TrimSpace(hdr[naming.HeaderProjectTarget]); v != "" {
			logArgs = append(logArgs, "ingest_project", v)
		}
		if v := strings.TrimSpace(hdr[naming.HeaderFlavorTarget]); v != "" {
			logArgs = append(logArgs, "flavor_id", v)
		}
	}
	if v := strings.TrimSpace(ix.cfg.DefaultScope.ProjectID); v != "" {
		logArgs = append(logArgs, "scope_project_id", v)
	}
	if v := strings.TrimSpace(ix.cfg.DefaultScope.WorkspaceID); v != "" {
		logArgs = append(logArgs, "scope_workspace_id", v)
	}
	if v := strings.TrimSpace(cfg.Defaults.ProjectID); v != "" {
		logArgs = append(logArgs, "defaults_project_id", v)
	}
	if v := strings.TrimSpace(cfg.Defaults.FlavorID); v != "" {
		logArgs = append(logArgs, "defaults_flavor_id", v)
	}
	ix.log.Info("gateway indexer config", logArgs...)
	return cfg, nil
}

// ScheduleInitialScan enqueues a single tier-1 ScanJob ("initial"). Discovery,
// corpus inventory load, and fan-out happen asynchronously inside worker
// handlers (queue-safe). Returns false if the queue is closed or full.
func (ix *Indexer) ScheduleInitialScan() bool {
	return ix.ScheduleScanForRoots(nil, "initial")
}

// ScheduleScanForRoots enqueues a scan job. When roots is nil or empty, all
// configured roots are walked; otherwise only the given roots are scanned.
func (ix *Indexer) ScheduleScanForRoots(roots []Root, scanID string) bool {
	wi := WorkItem{
		Kind:   WorkScan,
		Tier:   TierBulk,
		ScanID: scanID,
	}
	if len(roots) > 0 {
		wi.ScanRootIDs = make([]string, len(roots))
		for i, r := range roots {
			wi.ScanRootIDs[i] = r.ID
		}
	}
	ok := ix.queue.Enqueue(wi)
	if ok {
		n := len(ix.getRoots())
		if len(roots) > 0 {
			n = len(roots)
		}
		ix.log.Info("scheduled scan job",
			"msg", "indexer.run.progress",
			"phase", "scan_scheduled",
			"scan_id", scanID,
			"roots", n,
		)
	}
	return ok
}
