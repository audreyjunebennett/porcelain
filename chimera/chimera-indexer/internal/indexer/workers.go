package indexer

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"
)

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
		// immediately so operators (and /ui/settings) prove the drain loop is live.
		ix.LogQueueSnapshot("worker_drain_tick")
		t := time.NewTicker(workerDrainHeartbeatEvery)
		defer t.Stop()
		for {
			select {
			case <-tickCtx.Done():
				return
			case <-t.C:
				ix.LogQueueSnapshot("worker_drain_tick")
				ix.MaybeEmitScopeStatusEdge("")
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
	go ix.runSkipSummaryLoop(tickCtx)
	go ix.runIngestSummaryLoop(tickCtx)
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
				if wi.Kind == WorkIngest {
					if err := ix.waitIngestGateOpen(ctx); err != nil {
						return
					}
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
						ix.closeIngestGateFromHealth(ctx)
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
						ix.noteSkipSummaryIngestFailed(wi.Job)
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
	ix.FlushSkipSummaries()
	ix.FlushIngestSummaries()
	ix.LogQueueSnapshot("run_workers_exit")
}

// processWorkItem dispatches scan, fan-out, or ingest work.
func (ix *Indexer) processWorkItem(ctx context.Context, wi WorkItem, rng *rand.Rand, workerID int) error {
	switch wi.Kind {
	case WorkScan:
		return ix.runScanJob(ctx, wi.ScanID, wi.ScanRootIDs)
	case WorkFanoutList:
		return ix.runFanoutList(ctx, wi)
	case WorkIngest:
		return ix.processIngestWithRetries(ctx, wi, rng, workerID)
	default:
		return nil
	}
}

func (ix *Indexer) waitForRecovery(ctx context.Context) error {
	ix.inRecovery.Store(true)
	defer func() {
		ix.inRecovery.Store(false)
		ix.MaybeEmitScopeStatusEdge("recovery_exited")
	}()
	ix.MaybeEmitScopeStatusEdge("recovery_entered")

	pollOnce := func(pollN int) (bool, error) {
		h, errProbe := ix.client.CheckHealth(ctx)
		vectorstoreOK := errProbe == nil && h != nil && h.VectorstoreOK()
		ragDisabled := h != nil && h.RAGDisabled
		if errProbe != nil {
			ix.log.Warn("storage health probe failed", "err", errProbe)
		}
		if ragDisabled {
			ix.recoveryPollLog(pollN, false, true, h, nil, errProbe)
			ix.log.Error("gateway has RAG disabled; nothing to recover",
				"detail", h.Message, "type", h.ErrorType,
				"hint", "set rag.enabled=true in config/gateway.yaml and restart the chimera gateway")
			return false, fmt.Errorf("gateway rejects ingest: %s (%s)", h.Message, h.ErrorType)
		}
		if h != nil && !h.IngestReady() {
			ix.log.Warn("storage health degraded",
				"status", h.Status, "detail", h.HealthDetail(), "http_status", h.HTTPStatus,
				"embed_ok", h.EmbedOK(), "embed_reason_code", h.ReasonCode())
			ix.syncIngestGateFromHealth(ctx, h)
		}

		recovered := errProbe == nil && h != nil && h.IngestReady()
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

		ix.recoveryPollLog(pollN, vectorstoreOK, false, h, rootHealthOK, errProbe)
		if recovered {
			ix.openIngestGate(embedModelFromHealth(h))
			ix.log.Info("health recovered; resuming", "msg", "indexer.recovery.resumed")
			return true, nil
		}
		return false, nil
	}

	pollN := 0
	pollN++
	if recovered, err := pollOnce(pollN); err != nil {
		return err
	} else if recovered {
		return nil
	}

	t := time.NewTicker(ix.cfg.RecoveryPollInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			pollN++
			recovered, err := pollOnce(pollN)
			if err != nil {
				return err
			}
			if recovered {
				return nil
			}
		}
	}
}
