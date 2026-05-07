package indexer

import (
	"sort"
	"strings"
	"time"
)

func (ix *Indexer) tenantIDForLogs() string {
	gw := ix.lastGW.Load()
	if gw == nil {
		return ""
	}
	return strings.TrimSpace(gw.TenantID)
}

func (ix *Indexer) replaceWorkspaceTotalsFromDiscovery(perScopeWalk map[string]*discoveryAgg) {
	ix.workspaceFilesMu.Lock()
	defer ix.workspaceFilesMu.Unlock()
	ix.workspaceFilesByScope = make(map[string]int64, len(perScopeWalk))
	for sk, d := range perScopeWalk {
		ix.workspaceFilesByScope[sk] = int64(d.Candidates)
	}
}

func (ix *Indexer) bumpWorkspaceFileCount(root Root, rel string) {
	proj, flav := ix.cfg.IngestHeaders(root, rel)
	sk := ScopeKey(proj, flav)
	ix.workspaceFilesMu.Lock()
	defer ix.workspaceFilesMu.Unlock()
	if ix.workspaceFilesByScope == nil {
		ix.workspaceFilesByScope = map[string]int64{}
	}
	ix.workspaceFilesByScope[sk]++
}

func (ix *Indexer) decrementWorkspaceFileCount(root Root, rel string) {
	proj, flav := ix.cfg.IngestHeaders(root, rel)
	sk := ScopeKey(proj, flav)
	ix.workspaceFilesMu.Lock()
	defer ix.workspaceFilesMu.Unlock()
	if ix.workspaceFilesByScope == nil {
		return
	}
	v := ix.workspaceFilesByScope[sk] - 1
	if v < 0 {
		v = 0
	}
	ix.workspaceFilesByScope[sk] = v
}

func (ix *Indexer) pendingBulkSnapshot() map[string]int64 {
	ix.pendingBulkMu.Lock()
	defer ix.pendingBulkMu.Unlock()
	if len(ix.pendingBulkByScope) == 0 {
		return nil
	}
	out := make(map[string]int64, len(ix.pendingBulkByScope))
	for k, v := range ix.pendingBulkByScope {
		out[k] = v
	}
	return out
}

func (ix *Indexer) workspaceTotalsSnapshot() map[string]int64 {
	ix.workspaceFilesMu.Lock()
	defer ix.workspaceFilesMu.Unlock()
	if len(ix.workspaceFilesByScope) == 0 {
		return nil
	}
	out := make(map[string]int64, len(ix.workspaceFilesByScope))
	for k, v := range ix.workspaceFilesByScope {
		out[k] = v
	}
	return out
}

func unionScopeKeys(groups ...map[string]int64) []string {
	seen := map[string]struct{}{}
	for _, g := range groups {
		for k := range g {
			seen[k] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// EmitScopeStatus logs one indexer.scope.status line per active scope bundle.
func (ix *Indexer) EmitScopeStatus(phase string) {
	if ix.log == nil {
		return
	}
	tid := ix.tenantIDForLogs()
	ingestTally, fanoutTally := ix.queue.TallyScopeQueues(ix.cfg)
	bulkSnap := ix.pendingBulkSnapshot()
	wsSnap := ix.workspaceTotalsSnapshot()
	scopes := unionScopeKeys(wsSnap, ingestTally, fanoutTally, bulkSnap)

	for _, sk := range scopes {
		proj, flav := splitScopeKey(sk)
		ik := IndexerKey(tid, proj, flav)
		var wsTotal int64
		if wsSnap != nil {
			wsTotal = wsSnap[sk]
		}
		qIngest := ingestTally[sk]
		qFan := fanoutTally[sk]
		var pendingBulk int64
		if bulkSnap != nil {
			pendingBulk = bulkSnap[sk]
		}
		ix.log.Info("indexer scope status",
			"msg", "indexer.scope.status",
			"phase", phase,
			"tenant_id", tid,
			"project_id", proj,
			"ingest_project", proj,
			"flavor_id", flav,
			"indexer_target_key", ik,
			"workspace_files_total", wsTotal,
			"queue_ingest_pending", qIngest,
			"queue_fanout_files_pending", qFan,
			"pending_bulk_tier1", pendingBulk,
		)
	}
}

func (ix *Indexer) emitScopeActiveFileIfDue(workerID int, j Job) {
	if ix.log == nil {
		return
	}
	proj, flav := ix.cfg.IngestHeaders(j.Root, j.RelPath)
	sk := ScopeKey(proj, flav)
	tid := ix.tenantIDForLogs()
	ik := IndexerKey(tid, proj, flav)

	minGap := ix.cfg.ScopeActiveFileLogMinInterval
	if minGap <= 0 {
		minGap = time.Duration(defaultScopeActiveFileMinMs) * time.Millisecond
	}

	now := ix.nowForScopeLogs()
	rel := j.RelPath

	ix.activeFileLogMu.Lock()
	defer ix.activeFileLogMu.Unlock()
	if ix.lastActiveFilePath == nil {
		ix.lastActiveFilePath = map[string]string{}
		ix.lastActiveFileEmit = map[string]time.Time{}
	}
	lastRel := ix.lastActiveFilePath[sk]
	lastT := ix.lastActiveFileEmit[sk]
	emit := rel != lastRel || now.Sub(lastT) >= minGap
	if !emit {
		return
	}
	ix.lastActiveFilePath[sk] = rel
	ix.lastActiveFileEmit[sk] = now

	ix.log.Info("indexer scope active file",
		"msg", "indexer.scope.active_file",
		"tenant_id", tid,
		"project_id", proj,
		"ingest_project", proj,
		"flavor_id", flav,
		"indexer_target_key", ik,
		"root", j.Root.ID,
		"rel", rel,
		"worker", workerID,
	)
}

func (ix *Indexer) nowForScopeLogs() time.Time {
	if ix.hooks.Now != nil {
		return ix.hooks.Now()
	}
	return time.Now()
}
