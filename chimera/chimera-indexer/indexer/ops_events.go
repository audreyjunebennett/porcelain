package indexer

import (
	"strings"
	"sync/atomic"
)

// discoveryAgg counts walk-time skips and enqueue outcomes for indexer.discovery.summary.
type discoveryAgg struct {
	Candidates          int
	Enqueued            int
	QueueFull           int
	SkippedIgnoredFiles int
	SkippedIgnoredDirs  int
	SkippedBinary       int
	SkippedOversize     int
	SkippedOther        int
}

// SkippedIgnoredByRules returns hits from ignore patterns (.gitignore/.chimeraignore/default rules)
// counting both skipped directories and skipped files discovered during Walk.
func (d *discoveryAgg) SkippedIgnoredByRules() int {
	return d.SkippedIgnoredFiles + d.SkippedIgnoredDirs
}

func classifyDiscoverySkip(reason string) string {
	r := strings.ToLower(reason)
	switch {
	case strings.Contains(r, "ignored"):
		return "ignored"
	case strings.Contains(r, "binary"):
		return "binary"
	case strings.Contains(r, "exceeds max_file_bytes"):
		return "oversize"
	default:
		return "other"
	}
}

func (d *discoveryAgg) noteSkip(reason string) {
	switch classifyDiscoverySkip(reason) {
	case "ignored":
		if strings.Contains(strings.ToLower(reason), "dir") {
			d.SkippedIgnoredDirs++
		} else {
			d.SkippedIgnoredFiles++
		}
	case "binary":
		d.SkippedBinary++
	case "oversize":
		d.SkippedOversize++
	default:
		d.SkippedOther++
	}
}

// discoveryScopeLogAttrs returns structured fields shared by per-scope discovery logs.
func (d *discoveryAgg) discoveryScopeLogAttrs() []any {
	return []any{
		"candidates_discovered", d.Candidates,
		"skipped_ignored", d.SkippedIgnoredByRules(),
		"skipped_binary", d.SkippedBinary,
		"skipped_oversize", d.SkippedOversize,
		"skipped_other", d.SkippedOther,
	}
}

// LogQueueSnapshot emits indexer.queue.snapshot for operators / UI rollup.
func (ix *Indexer) LogQueueSnapshot(phase string) {
	if ix.log == nil {
		return
	}
	cap := ix.queue.Cap()
	bulkQ, writeQ, interactQ := ix.queue.LenByTier()
	args := []any{
		"msg", "indexer.queue.snapshot",
		"phase", phase,
		"queue_depth", ix.queue.Len(),
		"queue_depth_bulk", bulkQ,
		"queue_depth_write", writeQ,
		"queue_depth_interactive", interactQ,
		"queue_cap", cap,
		"workers", ix.cfg.Workers,
		"ingest_completed", atomic.LoadInt64(&ix.opsIngestOK),
		"ingest_failed_dropped", atomic.LoadInt64(&ix.opsIngestFail),
		"retry_events", atomic.LoadInt64(&ix.opsRetry),
		"jobs_dequeued", atomic.LoadInt64(&ix.opsDequeued),
		"skip_unchanged_corpus_client_hash", atomic.LoadInt64(&ix.opsSkipCorpusClientHash),
		"skip_unchanged_corpus_sync", atomic.LoadInt64(&ix.opsSkipCorpusSyncMatch),
		"skip_unchanged_local_sync", atomic.LoadInt64(&ix.opsSkipLocalSync),
	}
	ix.log.Info("indexer queue snapshot", args...)
}

// OpsSnapshot returns operator counters for run lifecycle logs (e.g. indexer.run.done).
func (ix *Indexer) OpsSnapshot() map[string]int64 {
	return map[string]int64{
		"ingest_completed":                  atomic.LoadInt64(&ix.opsIngestOK),
		"ingest_failed_dropped":             atomic.LoadInt64(&ix.opsIngestFail),
		"retry_events":                      atomic.LoadInt64(&ix.opsRetry),
		"jobs_dequeued":                     atomic.LoadInt64(&ix.opsDequeued),
		"skip_unchanged_corpus_client_hash": atomic.LoadInt64(&ix.opsSkipCorpusClientHash),
		"skip_unchanged_corpus_sync":        atomic.LoadInt64(&ix.opsSkipCorpusSyncMatch),
		"skip_unchanged_local_sync":         atomic.LoadInt64(&ix.opsSkipLocalSync),
	}
}

func appendOpsAttrs(dst []any, snap map[string]int64) []any {
	if snap == nil {
		return dst
	}
	keys := []string{
		"ingest_completed",
		"ingest_failed_dropped",
		"retry_events",
		"jobs_dequeued",
		"skip_unchanged_corpus_client_hash",
		"skip_unchanged_corpus_sync",
		"skip_unchanged_local_sync",
	}
	for _, k := range keys {
		if v, ok := snap[k]; ok {
			dst = append(dst, k, v)
		}
	}
	return dst
}

// RunDoneAttrs are structured fields for indexer.run.done (spread into slog.Info).
func RunDoneAttrs(mode string, snap map[string]int64) []any {
	out := []any{"msg", "indexer.run.done", "mode", mode}
	return appendOpsAttrs(out, snap)
}

// recoveryPollLog emits one structured line per recovery poll (default interval is long).
func (ix *Indexer) recoveryPollLog(pollN int, storageOK bool, ragDisabled bool, storageStatus, storageDetail string, rootHealthOK *bool, errProbe error) {
	if ix.log == nil {
		return
	}
	args := []any{
		"msg", "indexer.recovery.poll",
		"poll_n", pollN,
		"interval_ms", ix.cfg.RecoveryPollInterval.Milliseconds(),
		"storage_ok", storageOK,
		"rag_disabled", ragDisabled,
	}
	if storageStatus != "" {
		args = append(args, "storage_status", storageStatus)
	}
	if storageDetail != "" {
		args = append(args, "storage_detail", storageDetail)
	}
	if rootHealthOK != nil {
		args = append(args, "root_health_ok", *rootHealthOK)
	}
	if errProbe != nil {
		args = append(args, "probe_err", errProbe.Error())
	}
	ix.log.Info("recovery poll", args...)
}
