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
	ix.workspaceFilesByScope = make(map[string]int64, len(perScopeWalk))
	for sk, d := range perScopeWalk {
		ix.workspaceFilesByScope[sk] = int64(d.Candidates)
	}
	ix.workspaceFilesMu.Unlock()
	ix.MaybeEmitScopeStatusEdge("workspace_files_total")
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

// EmitScopeStatus logs heartbeat indexer.scope.status lines for every active scope.
func (ix *Indexer) EmitScopeStatus(phase string) {
	if ix.log == nil {
		return
	}
	watchMode := ix.runWatchModeForScope()
	for _, r := range ix.collectScopeStatusReadings(watchMode) {
		args := []any{
			"msg", "indexer.scope.status",
			"change_reason", "heartbeat",
			"phase", phase,
			"declarative_state", r.globals.Phase,
			"tenant_id", r.tenantID,
			"project_id", r.projectID,
			"ingest_project", r.projectID,
			"flavor_id", r.flavorID,
			"indexer_target_key", r.indexerTargetKey,
			"workspace_files_total", r.workspaceTotal,
			"queue_ingest_pending", r.queueIngest,
			"queue_fanout_files_pending", r.queueFanout,
			"pending_bulk_tier1", r.pendingBulk,
			"ingest_gate_closed", r.globals.IngestGateClosed,
			"in_recovery", r.globals.InRecovery,
			"ingest_completed", r.globals.IngestCompleted,
		}
		if r.globals.IngestGateReason != "" {
			args = append(args, "ingest_gate_reason_code", r.globals.IngestGateReason)
		}
		if r.globals.EmbedReasonCode != "" {
			args = append(args, "embed_reason_code", r.globals.EmbedReasonCode)
		}
		if r.currentRel != "" {
			args = append(args, "current_rel", r.currentRel)
		}
		ix.log.Info("indexer scope status", args...)
	}
	ix.syncScopeStatusSnapshots(watchMode)
}

func (ix *Indexer) syncScopeStatusSnapshots(watchMode bool) {
	readings := ix.collectScopeStatusReadings(watchMode)
	ix.scopeStatusEmitMu.Lock()
	defer ix.scopeStatusEmitMu.Unlock()
	if ix.lastScopeStatusEmitted == nil {
		ix.lastScopeStatusEmitted = map[string]scopeStatusEmitted{}
	}
	for _, r := range readings {
		ix.lastScopeStatusEmitted[r.scopeKey] = scopeStatusEmitted{
			globals:             r.globals,
			workspaceTotal:      r.workspaceTotal,
			queueIngest:         r.queueIngest,
			queueFanout:         r.queueFanout,
			pendingBulk:         r.pendingBulk,
			currentRel:          r.currentRel,
			lastIngestMilestone: r.globals.IngestCompleted,
		}
	}
	if len(readings) > 0 {
		ix.lastGlobalScopeStatus = readings[0].globals
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
	if ix.lastActiveFilePath == nil {
		ix.lastActiveFilePath = map[string]string{}
		ix.lastActiveFileEmit = map[string]time.Time{}
	}
	lastRel := ix.lastActiveFilePath[sk]
	lastT := ix.lastActiveFileEmit[sk]
	emit := rel != lastRel || now.Sub(lastT) >= minGap
	if !emit {
		ix.activeFileLogMu.Unlock()
		return
	}
	ix.lastActiveFilePath[sk] = rel
	ix.lastActiveFileEmit[sk] = now
	ix.activeFileLogMu.Unlock()

	ix.log.Debug("indexer scope active file",
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
