package indexer

import (
	"context"
	"encoding/json"
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
	scopes := DistinctEffectiveStorageStatsScopes(ix.cfg, gw)
	if len(scopes) == 0 {
		return nil
	}
	merged := make(map[string]CorpusInventoryRow)
	var loaded, failed int
	for _, sc := range scopes {
		proj := IngestProject(sc)
		flav := strings.TrimSpace(sc.FlavorID)
		hdrs := ScopeHTTPHeaders(proj, flav)
		page, err := ix.client.FetchCorpusInventoryAll(ctx, p, hdrs)
		if err != nil {
			failed++
			if ix.log != nil {
				ix.log.Warn("corpus inventory fetch failed for scope",
					"msg", "indexer.reconcile.inventory_scope_failed",
					"ingest_project", proj,
					"flavor_id", flav,
					"indexer_target_key", IndexerKey(ix.tenantIDForLogs(), proj, flav),
					"err", err,
				)
			}
			continue
		}
		for src, row := range page {
			merged[CorpusInventoryKey(proj, flav, src)] = row
		}
		loaded++
	}
	if len(merged) == 0 {
		if failed > 0 {
			return fmt.Errorf("corpus inventory: %d scope(s) failed, 0 entries loaded", failed)
		}
		return nil
	}
	ix.remoteInv = merged
	if ix.log != nil {
		ix.log.Info("corpus inventory loaded",
			"msg", "indexer.reconcile.summary",
			"phase", "inventory_loaded",
			"remote_source_paths", len(merged),
			"inventory_scopes_loaded", loaded,
			"inventory_scopes_failed", failed,
		)
	}
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
// retries are exhausted while errors remain retryable, or when the ingest gate
// is closed / embed short-circuit fires.
func (ix *Indexer) processIngestWithRetries(ctx context.Context, wi WorkItem, rng *rand.Rand, workerID int) error {
	j := wi.Job
	for attempt := 0; attempt < ix.cfg.RetryMaxAttempts; attempt++ {
		if ix.ingestGateClosed() {
			return ErrPaused
		}
		ix.ingestInflight.Add(1)
		err := ix.ingestOne(ctx, j, workerID)
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
		if ix.cfg.RetryShortCircuitOnEmbed && IsEmbedClassifiedHTTPError(err) {
			ix.closeIngestGateFromHealth(ctx)
			return ErrPaused
		}
		if ix.ingestGateClosed() {
			return ErrPaused
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

func (ix *Indexer) ingestOne(ctx context.Context, j Job, workerID int) error {
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
		atomic.AddInt64(&ix.opsSkipEmpty, 1)
		ix.noteSkipSummaryEmpty(j)
		ix.emitSkippedFile(j, "empty_or_whitespace", "indexer.skip.empty_or_whitespace", "skip ingest: empty or whitespace-only document")
		return nil
	}
	normalized, hash, err := ReadNormalizeFile(j.AbsPath)
	if err != nil {
		return fmt.Errorf("hash %s: %w", j.RelPath, err)
	}
	proj, flav := ix.cfg.IngestHeaders(j.Root, j.RelPath)
	if ix.remoteInv != nil {
		invKey := CorpusInventoryKey(proj, flav, j.RelPath)
		if row, ok := ix.remoteInv[invKey]; ok {
			if row.ClientContentHash != "" && row.ClientContentHash == hash {
				atomic.AddInt64(&ix.opsSkipCorpusClientHash, 1)
				ix.noteSkipSummaryUnchanged(j, "unchanged_corpus_client_hash")
				ix.emitSkippedFile(j, "unchanged_corpus_client_hash", "indexer.skip.unchanged_corpus_client_hash", "skip unchanged (corpus inventory)")
				return nil
			}
			if row.ClientContentHash == "" && ix.syncState != nil {
				if ent, ok := ix.syncState.Get(j.Key()); ok && ent.ServerSHA == row.ContentSHA256 && ent.ClientSHA == hash {
					atomic.AddInt64(&ix.opsSkipCorpusSyncMatch, 1)
					ix.noteSkipSummaryUnchanged(j, "unchanged_corpus_sync")
					ix.emitSkippedFile(j, "unchanged_corpus_sync", "indexer.skip.unchanged_corpus_sync", "skip unchanged (corpus inventory + sync state)")
					return nil
				}
			}
		}
	}
	if ix.syncState != nil {
		if ent, ok := ix.syncState.Get(j.Key()); ok && ent.ClientSHA == hash && ent.ChunkSchema == manifestChunkSchema {
			atomic.AddInt64(&ix.opsSkipLocalSync, 1)
			ix.noteSkipSummaryUnchanged(j, "unchanged_local_sync")
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

	chunkSize, chunkOverlap := 512, 128
	if gw != nil {
		if gw.ChunkSize > 0 {
			chunkSize = gw.ChunkSize
		}
		if gw.ChunkOverlap > 0 {
			chunkOverlap = gw.ChunkOverlap
		}
	}
	manifest, err := BuildManifest(j.RelPath, normalized, hash, chunkSize, chunkOverlap)
	if err != nil {
		return err
	}
	if ix.log != nil {
		ix.log.Debug("manifest built",
			"msg", "indexer.job.manifest_built",
			"rel", j.RelPath,
			"chunks", len(manifest.Chunks),
			"line_count", manifest.LineCount,
			"file_bytes", manifest.FileBytes,
		)
	}

	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("marshal manifest %s: %w", j.RelPath, err)
	}
	chunkCount := len(manifest.Chunks)
	wholeLimit := ix.effectiveWholeFileLimit(gw)
	useSession := gw != nil && strings.TrimSpace(gw.IngestSessionPath) != "" &&
		wholeLimit > 0 && int64(len(manifestJSON)) > wholeLimit

	ix.emitScopeActiveFileIfDue(workerID, j)
	ix.noteSkipSummaryIngestStarted(j)

	if ix.log != nil && ix.cfg.JobSkipLog != JobSkipLogOff {
		transport := "manifest"
		if useSession {
			transport = "manifest_session"
		}
		args := []any{
			"msg", "indexer.job.upload",
			"rel", j.RelPath,
			"bytes", st.Size(),
			"transport", transport,
			"chunks", len(manifest.Chunks),
		}
		args = append(args, ix.logScopeFieldsForJob(j)...)
		switch ix.cfg.JobSkipLog {
		case JobSkipLogInfo:
			ix.log.Info("job upload", args...)
		case JobSkipLogDebug:
			ix.log.Debug("job upload", args...)
		}
	}

	var res *IngestResponse
	pol := RetryPolicyFromResolved(ix.cfg)
	if useSession {
		res, err = ix.client.IngestManifestSession(ctx, manifest, IngestRequest{
			Source:      j.RelPath,
			ContentHash: hash,
			Project:     proj,
			Flavor:      flav,
		}, gw, pol)
	} else {
		res, err = ix.client.IngestManifestBody(ctx, manifestJSON, IngestRequest{
			Source:      j.RelPath,
			ContentHash: hash,
			Project:     proj,
			Flavor:      flav,
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
		ent := SyncEntry{
			ClientSHA:   hash,
			ServerSHA:   serverSHA,
			ChunkCount:  chunkCount,
			ChunkSchema: manifestChunkSchema,
		}
		if err := ix.syncState.Put(j.Key(), ent); err != nil {
			args := []any{
				"msg", "indexer.sync_state.write_failed",
				"rel", j.RelPath, "err", err,
			}
			args = append(args, ix.logScopeFieldsForJob(j)...)
			ix.log.Warn("sync state write failed", args...)
		}
	}
	mode := "manifest"
	if useSession {
		mode = "manifest_session"
	}
	atomic.AddInt64(&ix.opsIngestOK, 1)
	ix.noteSkipSummaryIngestSucceeded(j)
	ix.noteIngestSummarySucceeded(j, res.Chunks)
	ix.logIngestedSuccess(j, mode, res.Chunks, res.Collection, serverSHA)
	if ix.hooks.AfterIngest != nil {
		ix.hooks.AfterIngest(j, res)
	}
	return nil
}

func (ix *Indexer) logIngestedSuccess(j Job, mode string, chunks int, collection, contentSHA string) {
	if ix.log == nil {
		return
	}
	args := []any{
		"msg", "indexer.job.ingested",
		"rel", j.RelPath,
		"mode", mode,
		"chunks", chunks,
		"collection", collection,
		"content_sha256", contentSHA,
	}
	args = append(args, ix.logScopeFieldsForJob(j)...)
	switch ix.cfg.JobIngestLog {
	case JobIngestLogDebug:
		ix.log.Debug("ingested", args...)
	default:
		ix.log.Info("ingested", args...)
	}
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
