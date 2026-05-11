package indexer

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fsnotify/fsnotify"
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
	// initial scan (nil when unavailable). Keys are root-relative source paths.
	remoteInv map[string]CorpusInventoryRow

	// Operator-facing counters (indexer.* structured events / run.done rollup).
	opsSkipCorpusClientHash int64
	opsSkipCorpusSyncMatch  int64
	opsSkipLocalSync        int64
	opsIngestOK             int64
	opsIngestFail           int64
	opsRetry                int64
	opsDequeued             int64

	ingestInflight       atomic.Int32
	inRecovery           atomic.Bool
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
			"path", cfg.SyncStatePath, "err", err)
		st = nil
	}
	return &Indexer{
		cfg:       cfg,
		client:    client,
		log:       log,
		queue:     NewQueue(cfg.QueueDepth),
		matchers:  map[string]*Matcher{},
		syncState: st,
	}
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
				"hint", "set rag.enabled=true in config/gateway.yaml and restart the claudia gateway",
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
	// configs (distinct project/flavor pairs across roots) omit it so /ui/logs can
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
		if v := strings.TrimSpace(hdr["X-Claudia-Project"]); v != "" {
			logArgs = append(logArgs, "ingest_project", v)
		}
		if v := strings.TrimSpace(hdr["X-Claudia-Flavor-Id"]); v != "" {
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
	ok := ix.queue.Enqueue(WorkItem{
		Kind:   WorkScan,
		Tier:   TierBulk,
		ScanID: "initial",
	})
	if ok {
		ix.log.Info("scheduled initial scan job",
			"msg", "indexer.run.progress",
			"phase", "scan_scheduled",
			"scan_id", "initial",
		)
	}
	return ok
}

func (ix *Indexer) loadRemoteCorpusInventory(ctx context.Context) error {
	gw := ix.lastGW.Load()
	if gw == nil {
		return nil
	}
	p := strings.TrimSpace(gw.CorpusInventoryPath)
	if p == "" {
		p = "/v1/indexer/corpus/inventory"
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	m, err := ix.client.FetchCorpusInventoryAll(ctx, p, ix.cfg.DefaultIndexerHeaders())
	if err != nil {
		return err
	}
	ix.remoteInv = m
	ix.log.Info("corpus inventory loaded",
		"msg", "indexer.reconcile.summary",
		"phase", "inventory_loaded",
		"remote_source_paths", len(m),
	)
	return nil
}

// workerDrainHeartbeatEvery is how often we emit indexer.queue.snapshot while
// workers are draining. Skips and hashing log at DEBUG; a slow first ingest can
// otherwise leave operators with no INFO lines for minutes.
const workerDrainHeartbeatEvery = 30 * time.Second

// RunWorkers spawns cfg.Workers goroutines that drain the queue. It returns
// when ctx is cancelled or the queue is closed. Workers loop on retryable
// errors per the failure-handling contract; on a fatal error they log and
// drop the job.
func (ix *Indexer) RunWorkers(ctx context.Context) {
	ix.LogQueueSnapshot("run_workers_start")
	tickCtx, tickCancel := context.WithCancel(ctx)
	defer tickCancel()
	go func() {
		// time.Ticker does not fire until the first interval elapses; emit once
		// immediately so operators (and /ui/logs) prove the drain loop is live.
		ix.LogQueueSnapshot("worker_drain_tick")
		t := time.NewTicker(workerDrainHeartbeatEvery)
		defer t.Stop()
		for {
			select {
			case <-tickCtx.Done():
				return
			case <-t.C:
				ix.LogQueueSnapshot("worker_drain_tick")
			}
		}
	}()
	if ix.cfg.ScopeStatusPoll > 0 {
		go func() {
			t := time.NewTicker(ix.cfg.ScopeStatusPoll)
			defer t.Stop()
			ix.EmitScopeStatus("startup")
			for {
				select {
				case <-tickCtx.Done():
					return
				case <-t.C:
					ix.EmitScopeStatus("heartbeat")
				}
			}
		}()
	}
	var wg sync.WaitGroup
	for i := 0; i < ix.cfg.Workers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			rng := rand.New(rand.NewSource(time.Now().UnixNano() + int64(id)))
			for {
				wi, ok := ix.queue.Dequeue(ctx)
				if !ok {
					return
				}
				atomic.AddInt64(&ix.opsDequeued, 1)
				if wi.Kind == WorkIngest && wi.FromFanout && wi.BulkScopeKey != "" {
					ix.decPendingBulk(wi.BulkScopeKey)
				}
				if err := ix.processWorkItem(ctx, wi, rng, id); err != nil {
					if errors.Is(err, ErrPaused) {
						rel := ""
						if wi.Kind == WorkIngest {
							rel = wi.Job.RelPath
						}
						args := []any{
							"msg", "indexer.worker.paused",
							"worker", id, "rel", rel,
							"work_kind", wi.Kind,
						}
						if wi.Kind == WorkIngest {
							args = append(args, ix.logScopeFieldsForJob(wi.Job)...)
						}
						ix.log.Warn("worker paused; awaiting health recovery", args...)
						ix.LogQueueSnapshot("worker_paused_before_recovery")
						if perr := ix.waitForRecovery(ctx); perr != nil {
							return
						}
						if wi.Kind == WorkIngest && wi.FromFanout && wi.BulkScopeKey != "" {
							ix.incPendingBulk(wi.BulkScopeKey)
						}
						_ = ix.queue.Enqueue(wi)
						ix.LogQueueSnapshot("worker_resumed_after_recovery")
						continue
					}
					// When the session context was cancelled (controlled reload or shutdown),
					// dropped jobs are expected — the new session will re-process them via
					// initial scan. Log at WARN, not ERROR, to avoid alarming noise.
					if ctx.Err() != nil {
						if wi.Kind == WorkIngest {
							atomic.AddInt64(&ix.opsIngestFail, 1)
							args := []any{
								"msg", "indexer.job.cancelled",
								"type", "indexer.job.cancelled",
								"worker", id, "rel", wi.Job.RelPath, "err", err,
								"reload_drop", true,
							}
							args = append(args, ix.logScopeFieldsForJob(wi.Job)...)
							ix.log.Warn("ingest dropped (session restarting; will re-process)", args...)
						}
						return
					}
					if wi.Kind == WorkIngest {
						atomic.AddInt64(&ix.opsIngestFail, 1)
						args := []any{
							"msg", "indexer.job.failed",
							"worker", id, "rel", wi.Job.RelPath, "err", err,
						}
						args = append(args, ix.logScopeFieldsForJob(wi.Job)...)
						ix.log.Error("ingest failed (dropped)", args...)
					} else {
						args := []any{
							"msg", "indexer.work.failed",
							"worker", id, "kind", wi.Kind, "err", err,
						}
						if wi.Kind == WorkFanoutList && len(wi.Candidates) > 0 {
							args = append(args, ix.logScopeFieldsForTaggedSlice(wi.Candidates)...)
						}
						ix.log.Error("work item failed (dropped)", args...)
					}
				}
			}
		}(i)
	}
	wg.Wait()
	ix.LogQueueSnapshot("run_workers_exit")
}

// emitSkippedFile logs a per-file skip using JobSkipLog (info / debug / off).
func (ix *Indexer) emitSkippedFile(j Job, skipReason, debugSlug, debugHuman string) {
	if ix.log == nil || ix.cfg.JobSkipLog == JobSkipLogOff {
		return
	}
	switch ix.cfg.JobSkipLog {
	case JobSkipLogInfo:
		args := []any{
			"msg", "indexer.job.skipped",
			"rel", j.RelPath,
			"skip_reason", skipReason,
		}
		args = append(args, ix.logScopeFieldsForJob(j)...)
		ix.log.Info("job skipped", args...)
	case JobSkipLogDebug:
		args := []any{"msg", debugSlug, "rel", j.RelPath}
		args = append(args, ix.logScopeFieldsForJob(j)...)
		ix.log.Debug(debugHuman, args...)
	}
}

// processWorkItem dispatches scan, fan-out, or ingest work.
func (ix *Indexer) processWorkItem(ctx context.Context, wi WorkItem, rng *rand.Rand, workerID int) error {
	switch wi.Kind {
	case WorkScan:
		return ix.runScanJob(ctx, wi.ScanID)
	case WorkFanoutList:
		return ix.runFanoutList(ctx, wi)
	case WorkIngest:
		return ix.processIngestWithRetries(ctx, wi, rng, workerID)
	default:
		return nil
	}
}

// processIngestWithRetries runs ingestOne with backoff. Returns ErrPaused if
// retries are exhausted while errors remain retryable.
func (ix *Indexer) processIngestWithRetries(ctx context.Context, wi WorkItem, rng *rand.Rand, workerID int) error {
	j := wi.Job
	for attempt := 0; attempt < ix.cfg.RetryMaxAttempts; attempt++ {
		ix.emitScopeActiveFileIfDue(workerID, j)
		ix.ingestInflight.Add(1)
		err := ix.ingestOne(ctx, j)
		ix.ingestInflight.Add(-1)
		if err == nil {
			return nil
		}
		if IsFatal(err) {
			return err
		}
		if !IsRetryable(err) {
			return err
		}
		d := Backoff(attempt, ix.cfg.RetryBaseDelay, ix.cfg.RetryMaxDelay, rng)
		atomic.AddInt64(&ix.opsRetry, 1)
		args := []any{
			"msg", "indexer.retry.scheduled",
			"rel", j.RelPath,
			"attempt", attempt + 1,
			"max_attempts", ix.cfg.RetryMaxAttempts,
			"delay_ms", d.Milliseconds(),
			"err", err,
		}
		args = append(args, ix.logScopeFieldsForJob(j)...)
		ix.log.Warn("ingest retry", args...)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(d):
		}
	}
	return ErrPaused
}

func (ix *Indexer) ingestOne(ctx context.Context, j Job) error {
	st, err := os.Stat(j.AbsPath)
	if err != nil {
		return fmt.Errorf("stat %s: %w", j.RelPath, err)
	}
	if ix.cfg.MaxFileBytes > 0 && st.Size() > ix.cfg.MaxFileBytes {
		return fmt.Errorf("file exceeds max_file_bytes: %s", j.RelPath)
	}
	noText, err := fileHasNoIngestableText(j.AbsPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", j.RelPath, err)
	}
	if noText {
		ix.emitSkippedFile(j, "empty_or_whitespace", "indexer.skip.empty_or_whitespace", "skip ingest: empty or whitespace-only document")
		return nil
	}
	hash, _, err := HashFile(j.AbsPath)
	if err != nil {
		return fmt.Errorf("hash %s: %w", j.RelPath, err)
	}
	if ix.remoteInv != nil {
		if row, ok := ix.remoteInv[j.RelPath]; ok {
			if row.ClientContentHash != "" && row.ClientContentHash == hash {
				atomic.AddInt64(&ix.opsSkipCorpusClientHash, 1)
				ix.emitSkippedFile(j, "unchanged_corpus_client_hash", "indexer.skip.unchanged_corpus_client_hash", "skip unchanged (corpus inventory)")
				return nil
			}
			if row.ClientContentHash == "" && ix.syncState != nil {
				if ent, ok := ix.syncState.Get(j.Key()); ok && ent.ServerSHA == row.ContentSHA256 && ent.ClientSHA == hash {
					atomic.AddInt64(&ix.opsSkipCorpusSyncMatch, 1)
					ix.emitSkippedFile(j, "unchanged_corpus_sync", "indexer.skip.unchanged_corpus_sync", "skip unchanged (corpus inventory + sync state)")
					return nil
				}
			}
		}
	}
	if ix.syncState != nil {
		if ent, ok := ix.syncState.Get(j.Key()); ok && ent.ClientSHA == hash {
			atomic.AddInt64(&ix.opsSkipLocalSync, 1)
			ix.emitSkippedFile(j, "unchanged_local_sync", "indexer.skip.unchanged_local_sync", "skip unchanged (sync state)")
			return nil
		}
	}

	gw := ix.lastGW.Load()
	maxIngest := int64(1<<62 - 1)
	if gw != nil && gw.MaxIngestBytes > 0 {
		maxIngest = gw.MaxIngestBytes
	}
	if st.Size() > maxIngest {
		return fmt.Errorf("file larger than gateway max_ingest_bytes (%d): %s", maxIngest, j.RelPath)
	}

	proj, flav := ix.cfg.IngestHeaders(j.Root, j.RelPath)
	wholeLimit := ix.effectiveWholeFileLimit(gw)
	useChunked := gw != nil && strings.TrimSpace(gw.IngestSessionPath) != "" &&
		wholeLimit < maxIngest && st.Size() > wholeLimit

	if ix.log != nil && ix.cfg.JobSkipLog == JobSkipLogInfo {
		transport := "whole"
		if useChunked {
			transport = "chunked"
		}
		args := []any{
			"msg", "indexer.job.upload",
			"rel", j.RelPath,
			"bytes", st.Size(),
			"transport", transport,
		}
		args = append(args, ix.logScopeFieldsForJob(j)...)
		ix.log.Info("job upload", args...)
	}

	var res *IngestResponse
	if useChunked {
		pol := SessionRetryPolicy{
			MaxAttempts: ix.cfg.RetryMaxAttempts,
			BaseDelay:   ix.cfg.RetryBaseDelay,
			MaxDelay:    ix.cfg.RetryMaxDelay,
		}
		res, err = ix.client.IngestChunked(ctx, j.AbsPath, IngestRequest{
			Source:      j.RelPath,
			ContentHash: hash,
			Project:     proj,
			Flavor:      flav,
		}, gw, pol)
	} else {
		var f *os.File
		f, err = os.Open(j.AbsPath)
		if err != nil {
			return fmt.Errorf("open %s: %w", j.RelPath, err)
		}
		defer f.Close()
		res, err = ix.client.Ingest(ctx, IngestRequest{
			Source:      j.RelPath,
			ContentHash: hash,
			Project:     proj,
			Flavor:      flav,
			Body:        f,
		})
	}
	if err != nil {
		return err
	}
	serverSHA := strings.TrimSpace(res.ContentSHA256)
	if serverSHA == "" {
		serverSHA = strings.TrimSpace(res.ContentHash)
	}
	if ix.syncState != nil && serverSHA != "" {
		if err := ix.syncState.Put(j.Key(), SyncEntry{ClientSHA: hash, ServerSHA: serverSHA}); err != nil {
			args := []any{
				"msg", "indexer.sync_state.write_failed",
				"rel", j.RelPath, "err", err,
			}
			args = append(args, ix.logScopeFieldsForJob(j)...)
			ix.log.Warn("sync state write failed", args...)
		}
	}
	mode := "whole"
	if useChunked {
		mode = "chunked"
	}
	atomic.AddInt64(&ix.opsIngestOK, 1)
	args := []any{
		"msg", "indexer.job.ingested",
		"rel", j.RelPath,
		"mode", mode,
		"chunks", res.Chunks,
		"collection", res.Collection,
		"content_sha256", serverSHA,
	}
	args = append(args, ix.logScopeFieldsForJob(j)...)
	ix.log.Info("ingested", args...)
	if ix.hooks.AfterIngest != nil {
		ix.hooks.AfterIngest(j, res)
	}
	return nil
}

func (ix *Indexer) effectiveWholeFileLimit(gw *IndexerConfig) int64 {
	var gwWhole int64
	if gw != nil {
		gwWhole = gw.MaxWholeFileBytes
		if gwWhole <= 0 {
			gwWhole = gw.MaxIngestBytes
		}
	}
	if gwWhole <= 0 {
		gwWhole = ix.cfg.MaxFileBytes
	}
	out := gwWhole
	if ix.cfg.MaxWholeFileBytes > 0 {
		out = min(out, ix.cfg.MaxWholeFileBytes)
	}
	if ix.cfg.MaxFileBytes > 0 {
		out = min(out, ix.cfg.MaxFileBytes)
	}
	return out
}

func (ix *Indexer) waitForRecovery(ctx context.Context) error {
	ix.inRecovery.Store(true)
	defer ix.inRecovery.Store(false)
	t := time.NewTicker(ix.cfg.RecoveryPollInterval)
	defer t.Stop()
	pollN := 0
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			pollN++
			h, errProbe := ix.client.CheckHealth(ctx)
			storageOK := errProbe == nil && h != nil && h.OK
			ragDisabled := h != nil && h.RAGDisabled
			status, detail := "", ""
			if h != nil {
				status = h.Status
				detail = h.Detail
			}
			if errProbe != nil {
				ix.log.Warn("storage health probe failed", "err", errProbe)
			}
			if ragDisabled {
				ix.recoveryPollLog(pollN, false, true, status, detail, nil, errProbe)
				ix.log.Error("gateway has RAG disabled; nothing to recover",
					"detail", h.Message, "type", h.ErrorType,
					"hint", "set rag.enabled=true in config/gateway.yaml and restart the claudia gateway")
				return fmt.Errorf("gateway rejects ingest: %s (%s)", h.Message, h.ErrorType)
			}
			if h != nil && !storageOK {
				ix.log.Warn("storage health degraded",
					"status", h.Status, "detail", h.Detail, "http_status", h.HTTPStatus)
			}

			recovered := storageOK
			var rootHealthOK *bool
			if recovered && ix.cfg.RecoveryIncludeRootHealth {
				rh, rerr := ix.client.CheckGatewayRootHealth(ctx)
				if rerr != nil {
					ix.log.Warn("gateway /health probe failed", "err", rerr)
					recovered = false
				} else if rh == nil || !rh.OK {
					if rh != nil {
						ix.log.Warn("gateway /health not ready", "status", rh.Status, "degraded", rh.Degraded)
						b := rh.OK
						rootHealthOK = &b
					}
					recovered = false
				} else {
					b := true
					rootHealthOK = &b
				}
			} else if recovered {
				b := true
				rootHealthOK = &b
			}

			ix.recoveryPollLog(pollN, recovered, false, status, detail, rootHealthOK, errProbe)
			if recovered {
				ix.log.Info("health recovered; resuming", "msg", "indexer.recovery.resumed")
				return nil
			}
		}
	}
}

// RunWatchers wires fsnotify watchers onto every configured root and
// translates create/write events into queued jobs (debounced per path).
// Returns when ctx is cancelled.
func (ix *Indexer) RunWatchers(ctx context.Context) error {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("fsnotify: %w", err)
	}
	defer w.Close()

	for _, r := range ix.cfg.Roots {
		if err := addRecursive(w, r.AbsPath); err != nil {
			return fmt.Errorf("watch %s: %w", r.AbsPath, err)
		}
	}

	debouncer := newDebouncer(ix.cfg.Debounce, func(absPath string, tier PriorityTier) {
		root, rel, ok := ix.matchAbs(absPath)
		if !ok {
			return
		}
		m := ix.matchers[root.ID]
		if m != nil && m.Match(rel) {
			return
		}
		st, err := os.Stat(absPath)
		if err != nil || !st.Mode().IsRegular() {
			return
		}
		if ix.cfg.MaxFileBytes > 0 && st.Size() > ix.cfg.MaxFileBytes {
			return
		}
		bin, err := IsBinaryFile(absPath, ix.cfg.BinaryNullByteSample, ix.cfg.BinaryNullByteRatio)
		if err != nil || bin {
			return
		}
		job := Job{Root: root, RelPath: rel, AbsPath: absPath}
		wasPending := ix.queue.HasPendingKey(job.Key())
		w := IngestEnqueue(job, tier, false, "")
		if ix.queue.Enqueue(w) {
			if tier == TierInteractive && !wasPending {
				ix.bumpWorkspaceFileCount(root, rel)
			}
		}
	})
	defer debouncer.Close()

	for {
		select {
		case <-ctx.Done():
			return nil
		case ev, ok := <-w.Events:
			if !ok {
				return nil
			}
			if ev.Op&(fsnotify.Create|fsnotify.Write) != 0 {
				tier := TierWrite
				if ev.Op&fsnotify.Create != 0 {
					tier = TierInteractive
				}
				debouncer.Trigger(ev.Name, tier)
			}
			if ev.Op&fsnotify.Remove != 0 {
				ix.onWatchedPathRemoved(ev.Name)
			}
			if ev.Op&fsnotify.Create != 0 {
				if st, err := os.Stat(ev.Name); err == nil && st.IsDir() {
					_ = addRecursive(w, ev.Name)
				}
			}
		case err, ok := <-w.Errors:
			if !ok {
				return nil
			}
			ix.log.Warn("fsnotify error", "err", err)
		}
	}
}

func addRecursive(w *fsnotify.Watcher, root string) error {
	return filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if d.IsDir() {
			return w.Add(p)
		}
		return nil
	})
}

func (ix *Indexer) matchAbs(abs string) (Root, string, bool) {
	for _, r := range ix.cfg.Roots {
		if rel, ok := relPath(r.AbsPath, abs); ok {
			return r, rel, true
		}
	}
	return Root{}, "", false
}

func (ix *Indexer) onWatchedPathRemoved(absPath string) {
	root, rel, ok := ix.matchAbs(absPath)
	if !ok {
		return
	}
	if st, err := os.Stat(absPath); err == nil && st.IsDir() {
		return
	}
	ix.decrementWorkspaceFileCount(root, rel)
}
