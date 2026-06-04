package indexer

import (
	"context"
	"strings"
)

// isCollectionMissingDetail reports Qdrant collection-not-found style errors.
func isCollectionMissingDetail(detail string) bool {
	d := strings.ToLower(strings.TrimSpace(detail))
	if d == "" {
		return false
	}
	if strings.Contains(d, "404") {
		return true
	}
	for _, phrase := range []string{
		"doesn't exist",
		"does not exist",
		"not found",
		"collection missing",
		"no collection",
	} {
		if strings.Contains(d, phrase) {
			return true
		}
	}
	return false
}

// tryAutoRepairMissingCollection clears local sync checkpoints for roots in scope when
// storage stats show a missing Qdrant collection. Runs at most once per indexer_target_key.
func (ix *Indexer) tryAutoRepairMissingCollection(ctx context.Context, sc ScopeFragment, detail, itk string) bool {
	_ = ctx
	if ix == nil || ix.syncState == nil || !isCollectionMissingDetail(detail) {
		return false
	}
	ix.obsMu.Lock()
	if ix.autoRepairDone == nil {
		ix.autoRepairDone = map[string]bool{}
	}
	if ix.autoRepairDone[itk] {
		ix.obsMu.Unlock()
		return false
	}
	ix.autoRepairDone[itk] = true
	ix.obsMu.Unlock()

	proj := strings.TrimSpace(sc.ProjectID)
	flav := strings.TrimSpace(sc.FlavorID)
	gw := ix.lastGW.Load()
	cleared := 0
	for _, root := range ix.cfg.Roots {
		_, p, f := effectiveIngestTriple(ix.cfg, root, gw)
		if p != proj || f != flav {
			continue
		}
		if err := ix.syncState.DeleteByRoot(root.ID); err != nil {
			if ix.log != nil {
				ix.log.Warn("auto-repair sync delete failed",
					"msg", "indexer.reindex.auto_collection_missing",
					"root_id", root.ID,
					"ingest_project", proj,
					"flavor_id", flav,
					"err", err,
				)
			}
			continue
		}
		cleared++
	}
	if ix.log != nil {
		ix.log.Info("cleared sync state for missing vector collection; re-ingest on next scan",
			"msg", "indexer.reindex.auto_collection_missing",
			"indexer_target_key", itk,
			"ingest_project", proj,
			"flavor_id", flav,
			"roots_cleared", cleared,
		)
	}
	if cleared > 0 && ix.initialScanCompleted.Load() {
		ix.ScheduleInitialScan()
	}
	return cleared > 0
}
