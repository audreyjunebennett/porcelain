package indexer

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"sync/atomic"
	"time"
)

func (ix *Indexer) loadRemoteCorpusInventory(ctx context.Context) error {
	gw := ix.lastGW.Load()
	if gw == nil {
		return nil
	}
	p := strings.TrimSpace(gw.CorpusInventoryPath)
	if p == "" {
		p = apiPathIndexerCorpusInventory
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
		pol := RetryPolicyFromResolved(ix.cfg)
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
