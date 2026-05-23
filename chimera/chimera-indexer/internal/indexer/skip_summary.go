package indexer

import (
	"context"
	"sync"
	"time"
)

const defaultSkipSummaryMinIntervalMs = 5000

type skipSummaryWindow struct {
	filesEvaluated       int64
	skipLocalSync        int64
	skipCorpusClientHash int64
	skipCorpusSync       int64
	skipEmpty            int64
	ingestStarted        int64
	ingestSucceeded      int64
	ingestFailed         int64
	windowStart          time.Time
	lastEmit             time.Time
}

func (w *skipSummaryWindow) hasActivity() bool {
	return w != nil && w.filesEvaluated > 0
}

func (w *skipSummaryWindow) resetAfterEmit(now time.Time) {
	if w == nil {
		return
	}
	w.filesEvaluated = 0
	w.skipLocalSync = 0
	w.skipCorpusClientHash = 0
	w.skipCorpusSync = 0
	w.skipEmpty = 0
	w.ingestStarted = 0
	w.ingestSucceeded = 0
	w.ingestFailed = 0
	w.windowStart = now
	w.lastEmit = now
}

type skipSummaryTracker struct {
	mu      sync.Mutex
	byScope map[string]*skipSummaryWindow
}

func newSkipSummaryTracker() *skipSummaryTracker {
	return &skipSummaryTracker{byScope: map[string]*skipSummaryWindow{}}
}

func (t *skipSummaryTracker) window(scopeKey string, now time.Time) *skipSummaryWindow {
	w, ok := t.byScope[scopeKey]
	if !ok {
		w = &skipSummaryWindow{windowStart: now, lastEmit: time.Time{}}
		t.byScope[scopeKey] = w
	}
	return w
}

func (ix *Indexer) skipSummaryEnabled() bool {
	return ix != nil && ix.skipSummary != nil && ix.cfg.SkipSummaryMinInterval > 0
}

func (ix *Indexer) scopeKeyForJob(j Job) string {
	proj, flav := ix.cfg.IngestHeaders(j.Root, j.RelPath)
	return ScopeKey(proj, flav)
}

func (ix *Indexer) noteSkipSummaryEmpty(j Job) {
	if !ix.skipSummaryEnabled() {
		return
	}
	sk := ix.scopeKeyForJob(j)
	now := ix.nowForScopeLogs()
	ix.skipSummary.mu.Lock()
	defer ix.skipSummary.mu.Unlock()
	w := ix.skipSummary.window(sk, now)
	w.filesEvaluated++
	w.skipEmpty++
}

func (ix *Indexer) noteSkipSummaryUnchanged(j Job, kind string) {
	if !ix.skipSummaryEnabled() {
		return
	}
	sk := ix.scopeKeyForJob(j)
	now := ix.nowForScopeLogs()
	ix.skipSummary.mu.Lock()
	defer ix.skipSummary.mu.Unlock()
	w := ix.skipSummary.window(sk, now)
	w.filesEvaluated++
	switch kind {
	case "unchanged_local_sync":
		w.skipLocalSync++
	case "unchanged_corpus_client_hash":
		w.skipCorpusClientHash++
	case "unchanged_corpus_sync":
		w.skipCorpusSync++
	}
}

func (ix *Indexer) noteSkipSummaryIngestStarted(j Job) {
	if !ix.skipSummaryEnabled() {
		return
	}
	sk := ix.scopeKeyForJob(j)
	now := ix.nowForScopeLogs()
	ix.skipSummary.mu.Lock()
	defer ix.skipSummary.mu.Unlock()
	w := ix.skipSummary.window(sk, now)
	w.ingestStarted++
}

func (ix *Indexer) noteSkipSummaryIngestSucceeded(j Job) {
	if !ix.skipSummaryEnabled() {
		return
	}
	sk := ix.scopeKeyForJob(j)
	now := ix.nowForScopeLogs()
	ix.skipSummary.mu.Lock()
	defer ix.skipSummary.mu.Unlock()
	w := ix.skipSummary.window(sk, now)
	w.filesEvaluated++
	w.ingestSucceeded++
}

func (ix *Indexer) noteSkipSummaryIngestFailed(j Job) {
	if !ix.skipSummaryEnabled() {
		return
	}
	sk := ix.scopeKeyForJob(j)
	now := ix.nowForScopeLogs()
	ix.skipSummary.mu.Lock()
	defer ix.skipSummary.mu.Unlock()
	w := ix.skipSummary.window(sk, now)
	w.filesEvaluated++
	w.ingestFailed++
}

func (ix *Indexer) runSkipSummaryLoop(ctx context.Context) {
	if !ix.skipSummaryEnabled() {
		return
	}
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			ix.emitDueSkipSummaries(false)
		}
	}
}

func (ix *Indexer) FlushSkipSummaries() {
	ix.emitDueSkipSummaries(true)
}

func (ix *Indexer) emitDueSkipSummaries(force bool) {
	if !ix.skipSummaryEnabled() || ix.log == nil {
		return
	}
	minGap := ix.cfg.SkipSummaryMinInterval
	now := ix.nowForScopeLogs()

	type emitItem struct {
		scopeKey string
		window   skipSummaryWindow
		windowMs int64
	}
	var pending []emitItem

	ix.skipSummary.mu.Lock()
	for sk, w := range ix.skipSummary.byScope {
		if !w.hasActivity() {
			continue
		}
		if !force && now.Sub(w.windowStart) < minGap {
			continue
		}
		windowMs := int64(0)
		if !w.windowStart.IsZero() {
			windowMs = now.Sub(w.windowStart).Milliseconds()
		}
		pending = append(pending, emitItem{
			scopeKey: sk,
			window: skipSummaryWindow{
				filesEvaluated:       w.filesEvaluated,
				skipLocalSync:        w.skipLocalSync,
				skipCorpusClientHash: w.skipCorpusClientHash,
				skipCorpusSync:       w.skipCorpusSync,
				skipEmpty:            w.skipEmpty,
				ingestStarted:        w.ingestStarted,
				ingestSucceeded:      w.ingestSucceeded,
				ingestFailed:         w.ingestFailed,
			},
			windowMs: windowMs,
		})
		w.resetAfterEmit(now)
	}
	ix.skipSummary.mu.Unlock()

	for _, item := range pending {
		ix.logSkipSummary(item.scopeKey, &item.window, item.windowMs)
	}
}

func (ix *Indexer) logSkipSummary(scopeKey string, w *skipSummaryWindow, windowMs int64) {
	if ix.log == nil || w == nil || !w.hasActivity() {
		return
	}
	proj, flav := splitScopeKey(scopeKey)
	tid := ix.tenantIDForLogs()
	ik := IndexerKey(tid, proj, flav)
	ix.log.Info("indexer job skipped summary",
		"msg", "indexer.job.skipped.summary",
		"window_ms", windowMs,
		"files_evaluated", w.filesEvaluated,
		"skip_unchanged_local_sync", w.skipLocalSync,
		"skip_unchanged_corpus_client_hash", w.skipCorpusClientHash,
		"skip_unchanged_corpus_sync", w.skipCorpusSync,
		"skip_empty_or_whitespace", w.skipEmpty,
		"ingest_started", w.ingestStarted,
		"ingest_succeeded", w.ingestSucceeded,
		"ingest_failed", w.ingestFailed,
		"queue_depth", ix.queue.Len(),
		"tenant_id", tid,
		"project_id", proj,
		"ingest_project", proj,
		"flavor_id", flav,
		"indexer_target_key", ik,
	)
}
