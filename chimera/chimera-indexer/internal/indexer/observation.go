package indexer

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// RunObservationLoop periodically pulls GET /v1/indexer/storage/stats once per
// distinct effective ingest scope (deduped watched roots) and emits indexer.state
// snapshots for operators and /ui/settings.
func (ix *Indexer) RunObservationLoop(ctx context.Context, watchMode bool) {
	if ix.cfg.StorageStatsPoll <= 0 {
		return
	}
	t := time.NewTicker(ix.cfg.StorageStatsPoll)
	defer t.Stop()
	ix.EmitStorageStatsAndState(ctx, watchMode)
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			ix.EmitStorageStatsAndState(ctx, watchMode)
		}
	}
}

// EmitStorageStatsAndState fetches live storage stats (best effort) and logs
// structured indexer.storage.stats and indexer.state lines.
func (ix *Indexer) EmitStorageStatsAndState(ctx context.Context, watchMode bool) {
	if ix.log == nil {
		return
	}
	gw := ix.lastGW.Load()
	scopes := DistinctEffectiveStorageStatsScopes(ix.cfg, gw)
	if len(scopes) == 0 {
		ix.qdrantPoints.Store(0)
	} else {
		var total int64
		tenant := ""
		if gw != nil {
			tenant = strings.TrimSpace(gw.TenantID)
		}
		for _, sc := range scopes {
			hdrs := StorageStatsRequestHeaders(sc)
			stats, err := ix.client.FetchStorageStats(ctx, hdrs)
			proj := strings.TrimSpace(sc.ProjectID)
			flav := strings.TrimSpace(sc.FlavorID)
			itk := IndexerKey(tenant, proj, flav)
			if err != nil {
				ix.log.Warn("storage stats fetch failed",
					"msg", "indexer.storage.stats",
					"indexer_target_key", itk,
					"ingest_project", proj,
					"flavor_id", flav,
					"available", false,
					"err", err.Error())
				continue
			}
			total += stats.Points
			fp := storageStatsFingerprint(stats.Available, stats.Points, stats.VectorDim, stats.Detail, stats.Collection)
			if !stats.Available {
				ix.tryAutoRepairMissingCollection(ctx, sc, stats.Detail, itk)
				level := slog.LevelWarn
				if isCollectionMissingDetail(stats.Detail) {
					level = ix.storageStatsObsLevel(itk, fp)
					if level == slog.LevelWarn {
						level = slog.LevelInfo
					}
				}
				ix.log.Log(context.Background(), level, "indexer storage stats unavailable",
					"msg", "indexer.storage.stats",
					"indexer_target_key", itk,
					"ingest_project", proj,
					"flavor_id", flav,
					"collection", stats.Collection,
					"qdrant_points", stats.Points,
					"vector_dim", stats.VectorDim,
					"available", false,
					"detail", stats.Detail,
				)
				ix.obsMu.Lock()
				if ix.lastStorageStatsFP == nil {
					ix.lastStorageStatsFP = map[string]string{}
				}
				ix.lastStorageStatsFP[itk] = fp
				ix.obsMu.Unlock()
				continue
			}
			level := ix.storageStatsObsLevel(itk, fp)
			ix.log.Log(context.Background(), level, "indexer storage stats sync",
				"msg", "indexer.storage.stats",
				"indexer_target_key", itk,
				"ingest_project", proj,
				"flavor_id", flav,
				"collection", stats.Collection,
				"qdrant_points", stats.Points,
				"vector_dim", stats.VectorDim,
				"available", stats.Available,
				"detail", stats.Detail,
			)
		}
		ix.qdrantPoints.Store(total)
	}

	state := ix.computeDeclarativeState(watchMode)
	stateFP := stateObservationFingerprint(
		state,
		ix.queue.Len(),
		ix.ingestInflight.Load(),
		ix.initialScanCompleted.Load(),
		watchMode,
		ix.inRecovery.Load(),
		ix.qdrantPoints.Load(),
	)
	stateLevel := ix.stateObsLevel(stateFP)
	ix.log.Log(context.Background(), stateLevel, "indexer state",
		"msg", "indexer.state",
		"state", state,
		"queue_depth", ix.queue.Len(),
		"ingest_inflight", ix.ingestInflight.Load(),
		"initial_scan_complete", ix.initialScanCompleted.Load(),
		"watch_mode", watchMode,
		"recovery", ix.inRecovery.Load(),
		"qdrant_points_reported", ix.qdrantPoints.Load(),
	)
}

func storageStatsFingerprint(available bool, points int64, vectorDim int, detail, collection string) string {
	return fmt.Sprintf("%t/%d/%d/%s/%s", available, points, vectorDim, detail, collection)
}

func stateObservationFingerprint(state string, queueDepth int, ingestInflight int32, initialScanComplete, watchMode, recovery bool, qdrantPoints int64) string {
	return fmt.Sprintf("%s/%d/%d/%t/%t/%t/%d", state, queueDepth, ingestInflight, initialScanComplete, watchMode, recovery, qdrantPoints)
}

func (ix *Indexer) storageStatsObsLevel(scopeKey, fp string) slog.Level {
	ix.obsMu.Lock()
	defer ix.obsMu.Unlock()
	if ix.lastStorageStatsFP == nil {
		ix.lastStorageStatsFP = map[string]string{}
	}
	prev, seen := ix.lastStorageStatsFP[scopeKey]
	if seen && prev == fp {
		return slog.LevelDebug
	}
	ix.lastStorageStatsFP[scopeKey] = fp
	return slog.LevelInfo
}

func (ix *Indexer) stateObsLevel(fp string) slog.Level {
	ix.obsMu.Lock()
	defer ix.obsMu.Unlock()
	if ix.lastStateFP == fp {
		return slog.LevelDebug
	}
	ix.lastStateFP = fp
	return slog.LevelInfo
}

func (ix *Indexer) computeDeclarativeState(watchMode bool) string {
	if ix.inRecovery.Load() {
		return "recovery"
	}
	if !ix.initialScanCompleted.Load() {
		return "initial_scanning"
	}
	if ix.ingestInflight.Load() > 0 {
		return "uploading"
	}
	if ix.queue.Len() > 0 {
		return "backlog"
	}
	if watchMode {
		return "watch_idle"
	}
	return "idle"
}

// LogIndexerRunStart logs indexer.run.start with absolute watched paths (never
// sent to the gateway; local operator logs only).
func (ix *Indexer) LogIndexerRunStart() {
	if ix.log == nil {
		return
	}
	paths := make([]string, len(ix.cfg.Roots))
	for i, r := range ix.cfg.Roots {
		paths[i] = r.AbsPath
	}
	ds := ix.cfg.DefaultScope
	payload := RootScopesPayload(ix.cfg, ix.lastGW.Load())
	ix.log.Info("indexer run start", "msg", "indexer.run.start",
		"roots", len(ix.cfg.Roots),
		"root_ids", RootIDsCSV(ix.cfg.Roots),
		"watch_root_paths", paths,
		"root_scopes", string(payload),
		"ingest_project", IngestProject(ds),
		"flavor_id", strings.TrimSpace(ds.FlavorID),
		"scope_project_id", strings.TrimSpace(ds.ProjectID),
		"scope_workspace_id", strings.TrimSpace(ds.WorkspaceID),
	)
}
