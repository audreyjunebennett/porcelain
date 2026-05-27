package indexer

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"
)

const defaultQueueSnapshotIdleInfoIntervalMs = 300000 // 5 minutes

// LogQueueSnapshot emits indexer.queue.snapshot for operators / UI rollup.
// Unchanged idle drains log at DEBUG except for a periodic INFO heartbeat.
func (ix *Indexer) LogQueueSnapshot(phase string) {
	if ix.log == nil {
		return
	}
	fp := ix.queueSnapshotFingerprint()
	level := ix.queueSnapshotLevel(phase, fp)
	args := ix.queueSnapshotArgs(phase)
	ix.log.Log(context.Background(), level, "indexer queue snapshot", args...)
}

func (ix *Indexer) queueSnapshotArgs(phase string) []any {
	cap := ix.queue.Cap()
	bulkQ, writeQ, interactQ := ix.queue.LenByTier()
	return []any{
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
}

func (ix *Indexer) queueSnapshotFingerprint() string {
	bulkQ, writeQ, interactQ := ix.queue.LenByTier()
	return fmt.Sprintf("%d/%d/%d/%d/%d/%d/%d/%d/%d/%d/%d",
		ix.queue.Len(), bulkQ, writeQ, interactQ,
		atomic.LoadInt64(&ix.opsIngestOK),
		atomic.LoadInt64(&ix.opsIngestFail),
		atomic.LoadInt64(&ix.opsRetry),
		atomic.LoadInt64(&ix.opsDequeued),
		atomic.LoadInt64(&ix.opsSkipCorpusClientHash),
		atomic.LoadInt64(&ix.opsSkipCorpusSyncMatch),
		atomic.LoadInt64(&ix.opsSkipLocalSync),
	)
}

func queueSnapshotAlwaysInfoPhase(phase string) bool {
	switch phase {
	case "run_workers_start", "run_workers_exit", "after_initial_scan",
		"worker_paused_before_recovery", "worker_resumed_after_recovery":
		return true
	default:
		return false
	}
}

func (ix *Indexer) queueSnapshotLevel(phase, fp string) slog.Level {
	now := ix.nowForScopeLogs()
	if queueSnapshotAlwaysInfoPhase(phase) {
		ix.markQueueSnapshotInfo(fp, now)
		return slog.LevelInfo
	}
	if ix.queue.Len() > 0 || ix.ingestInflight.Load() > 0 {
		ix.markQueueSnapshotInfo(fp, now)
		return slog.LevelInfo
	}

	ix.queueSnapMu.Lock()
	defer ix.queueSnapMu.Unlock()

	if fp != ix.lastQueueSnapFP {
		ix.lastQueueSnapFP = fp
		ix.lastQueueSnapInfoAt = now
		return slog.LevelInfo
	}

	interval := ix.cfg.QueueSnapshotIdleInfoInterval
	if interval <= 0 {
		return slog.LevelDebug
	}
	if ix.lastQueueSnapInfoAt.IsZero() || now.Sub(ix.lastQueueSnapInfoAt) >= interval {
		ix.lastQueueSnapInfoAt = now
		return slog.LevelInfo
	}
	return slog.LevelDebug
}

func (ix *Indexer) markQueueSnapshotInfo(fp string, now time.Time) {
	ix.queueSnapMu.Lock()
	defer ix.queueSnapMu.Unlock()
	ix.lastQueueSnapFP = fp
	ix.lastQueueSnapInfoAt = now
}
