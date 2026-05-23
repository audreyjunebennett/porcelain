package indexer

import (
	"context"
	"strings"
	"sync"
	"time"
)

type ingestSummaryWindow struct {
	ingestSucceeded int64
	chunksTotal     int64
	lastRel         string
	windowStart     time.Time
	lastEmit        time.Time
}

func (w *ingestSummaryWindow) hasActivity() bool {
	return w != nil && w.ingestSucceeded > 0
}

func (w *ingestSummaryWindow) resetAfterEmit(now time.Time) {
	if w == nil {
		return
	}
	w.ingestSucceeded = 0
	w.chunksTotal = 0
	w.lastRel = ""
	w.windowStart = now
	w.lastEmit = now
}

type ingestSummaryTracker struct {
	mu      sync.Mutex
	byScope map[string]*ingestSummaryWindow
}

func newIngestSummaryTracker() *ingestSummaryTracker {
	return &ingestSummaryTracker{byScope: map[string]*ingestSummaryWindow{}}
}

func (t *ingestSummaryTracker) window(scopeKey string, now time.Time) *ingestSummaryWindow {
	w, ok := t.byScope[scopeKey]
	if !ok {
		w = &ingestSummaryWindow{windowStart: now, lastEmit: time.Time{}}
		t.byScope[scopeKey] = w
	}
	return w
}

func (ix *Indexer) ingestSummaryEnabled() bool {
	return ix != nil && ix.ingestSummary != nil && ix.cfg.SkipSummaryMinInterval > 0
}

func (ix *Indexer) noteIngestSummarySucceeded(j Job, chunks int) {
	if !ix.ingestSummaryEnabled() {
		return
	}
	sk := ix.scopeKeyForJob(j)
	now := ix.nowForScopeLogs()
	ix.ingestSummary.mu.Lock()
	defer ix.ingestSummary.mu.Unlock()
	w := ix.ingestSummary.window(sk, now)
	w.ingestSucceeded++
	if chunks > 0 {
		w.chunksTotal += int64(chunks)
	}
	if rel := strings.TrimSpace(j.RelPath); rel != "" {
		w.lastRel = rel
	}
}

func (ix *Indexer) runIngestSummaryLoop(ctx context.Context) {
	if !ix.ingestSummaryEnabled() {
		return
	}
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			ix.emitDueIngestSummaries(false)
		}
	}
}

func (ix *Indexer) FlushIngestSummaries() {
	ix.emitDueIngestSummaries(true)
}

func (ix *Indexer) emitDueIngestSummaries(force bool) {
	if !ix.ingestSummaryEnabled() || ix.log == nil {
		return
	}
	minGap := ix.cfg.SkipSummaryMinInterval
	now := ix.nowForScopeLogs()

	type emitItem struct {
		scopeKey string
		window   ingestSummaryWindow
		windowMs int64
	}
	var pending []emitItem

	ix.ingestSummary.mu.Lock()
	for sk, w := range ix.ingestSummary.byScope {
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
			window: ingestSummaryWindow{
				ingestSucceeded: w.ingestSucceeded,
				chunksTotal:     w.chunksTotal,
				lastRel:         w.lastRel,
			},
			windowMs: windowMs,
		})
		w.resetAfterEmit(now)
	}
	ix.ingestSummary.mu.Unlock()

	for _, item := range pending {
		ix.logIngestSummary(item.scopeKey, &item.window, item.windowMs)
	}
}

func (ix *Indexer) logIngestSummary(scopeKey string, w *ingestSummaryWindow, windowMs int64) {
	if ix.log == nil || w == nil || !w.hasActivity() {
		return
	}
	proj, flav := splitScopeKey(scopeKey)
	tid := ix.tenantIDForLogs()
	ik := IndexerKey(tid, proj, flav)
	ix.log.Info("indexer job ingested summary",
		"msg", "indexer.job.ingested.summary",
		"window_ms", windowMs,
		"ingest_succeeded", w.ingestSucceeded,
		"chunks_total", w.chunksTotal,
		"last_rel", w.lastRel,
		"queue_depth", ix.queue.Len(),
		"tenant_id", tid,
		"project_id", proj,
		"ingest_project", proj,
		"flavor_id", flav,
		"indexer_target_key", ik,
	)
}
