package indexer

import (
	"sync/atomic"
	"time"
)

const (
	defaultScopeStatusIngestMilestone   = 25
	defaultScopeStatusEdgeMinIntervalMs = 2000
	queuePendingDeltaThreshold          = 100
)

type scopeStatusGlobals struct {
	Phase            string
	PhaseEdgeBucket  string
	IngestGateClosed bool
	IngestGateReason string
	EmbedReasonCode  string
	InRecovery       bool
	IngestCompleted  int64
}

type scopeStatusReading struct {
	scopeKey         string
	tenantID         string
	projectID        string
	flavorID         string
	indexerTargetKey string
	workspaceTotal   int64
	queueIngest      int64
	queueFanout      int64
	pendingBulk      int64
	currentRel       string
	globals          scopeStatusGlobals
}

type scopeStatusEmitted struct {
	globals             scopeStatusGlobals
	workspaceTotal      int64
	queueIngest         int64
	queueFanout         int64
	pendingBulk         int64
	currentRel          string
	lastIngestMilestone int64
	lastEdgeEmit        time.Time
}

func (ix *Indexer) SetRunWatchMode(watchMode bool) {
	ix.runWatchMode.Store(watchMode)
}

func (ix *Indexer) runWatchModeForScope() bool {
	return ix.runWatchMode.Load()
}

func (ix *Indexer) setLastEmbedReasonCode(code string) {
	ix.embedReasonMu.Lock()
	ix.lastEmbedReasonCode = code
	ix.embedReasonMu.Unlock()
}

func (ix *Indexer) getEmbedReasonCode() string {
	ix.embedReasonMu.Lock()
	defer ix.embedReasonMu.Unlock()
	return ix.lastEmbedReasonCode
}

func (ix *Indexer) ingestGateSnapshot() (closed bool, reason string) {
	if ix.ingestGate == nil {
		return false, ""
	}
	ix.ingestGate.mu.Lock()
	defer ix.ingestGate.mu.Unlock()
	return ix.ingestGate.closed, ix.ingestGate.reasonCode
}

func scopePhaseEdgeBucket(phase string, queueDepth int64, ingestInflight int32, watchMode bool) string {
	switch phase {
	case "recovery", "initial_scanning":
		return phase
	}
	if queueDepth > 0 || ingestInflight > 0 {
		return "draining"
	}
	if watchMode {
		return "watch_idle"
	}
	return "idle"
}

func (ix *Indexer) currentScopeStatusGlobals(watchMode bool) scopeStatusGlobals {
	closed, reason := ix.ingestGateSnapshot()
	phase := ix.computeDeclarativeState(watchMode)
	qd := int64(ix.queue.Len())
	infl := ix.ingestInflight.Load()
	return scopeStatusGlobals{
		Phase:            phase,
		PhaseEdgeBucket:  scopePhaseEdgeBucket(phase, qd, infl, watchMode),
		IngestGateClosed: closed,
		IngestGateReason: reason,
		EmbedReasonCode:  ix.getEmbedReasonCode(),
		InRecovery:       ix.inRecovery.Load(),
		IngestCompleted:  atomic.LoadInt64(&ix.opsIngestOK),
	}
}

func (ix *Indexer) collectScopeStatusReadings(watchMode bool) []scopeStatusReading {
	tid := ix.tenantIDForLogs()
	globals := ix.currentScopeStatusGlobals(watchMode)
	ingestTally, fanoutTally := ix.queue.TallyScopeQueues(ix.cfg)
	bulkSnap := ix.pendingBulkSnapshot()
	wsSnap := ix.workspaceTotalsSnapshot()
	scopes := unionScopeKeys(wsSnap, ingestTally, fanoutTally, bulkSnap)

	ix.activeFileLogMu.Lock()
	activeRelByScope := make(map[string]string, len(ix.lastActiveFilePath))
	for sk, rel := range ix.lastActiveFilePath {
		activeRelByScope[sk] = rel
	}
	ix.activeFileLogMu.Unlock()

	out := make([]scopeStatusReading, 0, len(scopes))
	for _, sk := range scopes {
		proj, flav := splitScopeKey(sk)
		var wsTotal int64
		if wsSnap != nil {
			wsTotal = wsSnap[sk]
		}
		var pendingBulk int64
		if bulkSnap != nil {
			pendingBulk = bulkSnap[sk]
		}
		out = append(out, scopeStatusReading{
			scopeKey:         sk,
			tenantID:         tid,
			projectID:        proj,
			flavorID:         flav,
			indexerTargetKey: IndexerKey(tid, proj, flav),
			workspaceTotal:   wsTotal,
			queueIngest:      ingestTally[sk],
			queueFanout:      fanoutTally[sk],
			pendingBulk:      pendingBulk,
			currentRel:       activeRelByScope[sk],
			globals:          globals,
		})
	}
	return out
}

func globalScopeStatusChangeReason(last, cur scopeStatusGlobals, milestone int) string {
	if last.PhaseEdgeBucket != cur.PhaseEdgeBucket {
		return "phase"
	}
	if last.IngestGateClosed != cur.IngestGateClosed || last.IngestGateReason != cur.IngestGateReason {
		return "ingest_gate"
	}
	if last.EmbedReasonCode != cur.EmbedReasonCode {
		return "embed_reason_code"
	}
	if last.InRecovery != cur.InRecovery {
		if cur.InRecovery {
			return "recovery_entered"
		}
		return "recovery_exited"
	}
	if milestone > 0 && cur.IngestCompleted > 0 {
		lastBucket := last.IngestCompleted / int64(milestone)
		curBucket := cur.IngestCompleted / int64(milestone)
		if curBucket > lastBucket {
			return "ingest_completed"
		}
	}
	return ""
}

func queuePendingChangeReason(field string, last, cur, workspaceTotal int64) string {
	if last == cur {
		return ""
	}
	if last == 0 && cur > 0 {
		return field
	}
	if last > 0 && cur == 0 {
		return field
	}
	delta := cur - last
	if delta < 0 {
		delta = -delta
	}
	if delta >= queuePendingDeltaThreshold {
		return field
	}
	if workspaceTotal > 0 {
		lastBucket := (last * 10) / workspaceTotal
		curBucket := (cur * 10) / workspaceTotal
		if lastBucket != curBucket {
			return field
		}
	}
	return ""
}

func perScopeStatusChangeReason(last scopeStatusEmitted, cur scopeStatusReading) string {
	if last.workspaceTotal != cur.workspaceTotal {
		return "workspace_files_total"
	}
	if r := queuePendingChangeReason("queue_ingest_pending", last.queueIngest, cur.queueIngest, cur.workspaceTotal); r != "" {
		return r
	}
	if r := queuePendingChangeReason("queue_fanout_files_pending", last.queueFanout, cur.queueFanout, cur.workspaceTotal); r != "" {
		return r
	}
	if r := queuePendingChangeReason("pending_bulk_tier1", last.pendingBulk, cur.pendingBulk, cur.workspaceTotal); r != "" {
		return r
	}
	if last.currentRel != cur.currentRel && cur.currentRel != "" {
		return "current_rel"
	}
	return ""
}

func queueZeroCrossing(last, cur int64) bool {
	return (last == 0 && cur > 0) || (last > 0 && cur == 0)
}

func edgeReasonHighPriority(reason string) bool {
	switch reason {
	case "ingest_gate", "recovery_entered", "recovery_exited", "embed_reason_code", "initial", "workspace_files_total":
		return true
	default:
		return false
	}
}

func (ix *Indexer) scopeStatusEdgeMinInterval() time.Duration {
	d := ix.cfg.ScopeStatusEdgeMinInterval
	if d < 0 {
		return 0
	}
	if d == 0 {
		return time.Duration(defaultScopeStatusEdgeMinIntervalMs) * time.Millisecond
	}
	return d
}

func allowScopeStatusEdgeEmit(reason string, last scopeStatusEmitted, cur scopeStatusReading, minGap time.Duration, now time.Time) bool {
	if edgeReasonHighPriority(reason) {
		return true
	}
	if reason == "queue_ingest_pending" && queueZeroCrossing(last.queueIngest, cur.queueIngest) {
		return true
	}
	if reason == "queue_fanout_files_pending" && queueZeroCrossing(last.queueFanout, cur.queueFanout) {
		return true
	}
	if reason == "pending_bulk_tier1" && queueZeroCrossing(last.pendingBulk, cur.pendingBulk) {
		return true
	}
	if reason == "phase" && (last.globals.PhaseEdgeBucket == "watch_idle" || cur.globals.PhaseEdgeBucket == "watch_idle" ||
		last.globals.PhaseEdgeBucket == "initial_scanning" || cur.globals.PhaseEdgeBucket == "initial_scanning" ||
		last.globals.PhaseEdgeBucket == "recovery" || cur.globals.PhaseEdgeBucket == "recovery") {
		return true
	}
	if minGap <= 0 {
		return true
	}
	if !last.lastEdgeEmit.IsZero() && now.Sub(last.lastEdgeEmit) < minGap {
		return false
	}
	return true
}

func (ix *Indexer) scopeStatusIngestMilestone() int {
	n := ix.cfg.ScopeStatusIngestMilestone
	if n < 0 {
		return 0
	}
	if n == 0 {
		return defaultScopeStatusIngestMilestone
	}
	return n
}

// MaybeEmitScopeStatusEdge logs indexer.scope.status for scopes whose tracked
// dimensions changed since the last emission. explicitReason forces emission
// for all active scopes (e.g. ingest_gate).
func (ix *Indexer) MaybeEmitScopeStatusEdge(explicitReason string) {
	if ix.log == nil {
		return
	}
	watchMode := ix.runWatchModeForScope()
	readings := ix.collectScopeStatusReadings(watchMode)
	if len(readings) == 0 {
		return
	}

	milestone := ix.scopeStatusIngestMilestone()
	minGap := ix.scopeStatusEdgeMinInterval()
	now := ix.nowForScopeLogs()
	ix.scopeStatusEmitMu.Lock()
	defer ix.scopeStatusEmitMu.Unlock()
	if ix.lastScopeStatusEmitted == nil {
		ix.lastScopeStatusEmitted = map[string]scopeStatusEmitted{}
	}

	globals := readings[0].globals
	globalReason := explicitReason
	if globalReason == "" {
		globalReason = globalScopeStatusChangeReason(ix.lastGlobalScopeStatus, globals, milestone)
	}

	for _, r := range readings {
		last, ok := ix.lastScopeStatusEmitted[r.scopeKey]
		reason := globalReason
		if reason == "" && ok {
			reason = perScopeStatusChangeReason(last, r)
		} else if reason == "" && !ok {
			reason = "initial"
		}
		if reason == "" {
			continue
		}
		if explicitReason == "" && ok && !allowScopeStatusEdgeEmit(reason, last, r, minGap, now) {
			continue
		}
		ix.logScopeStatusLine(r, reason)
		ix.lastScopeStatusEmitted[r.scopeKey] = scopeStatusEmitted{
			globals:             globals,
			workspaceTotal:      r.workspaceTotal,
			queueIngest:         r.queueIngest,
			queueFanout:         r.queueFanout,
			pendingBulk:         r.pendingBulk,
			currentRel:          r.currentRel,
			lastIngestMilestone: globals.IngestCompleted,
			lastEdgeEmit:        now,
		}
	}
	ix.lastGlobalScopeStatus = globals
}

func (ix *Indexer) logScopeStatusLine(r scopeStatusReading, changeReason string) {
	args := []any{
		"msg", "indexer.scope.status",
		"change_reason", changeReason,
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
