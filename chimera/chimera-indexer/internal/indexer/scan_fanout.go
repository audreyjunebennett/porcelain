package indexer

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
)

// maxDiscoveryPathsLogged caps rel_paths per scope in INFO logs (full list at DEBUG).
const maxDiscoveryPathsLogged = 200

func (ix *Indexer) computePerScopeBudget(nScopes int) int {
	cap := ix.queue.Cap()
	if cap <= 0 {
		return 1 << 28 // effectively unlimited chunk size for unbounded queues
	}
	pct := effectiveQueueFanoutHWMPercent(ix.cfg.QueueFanoutHWMPercent)
	p := float64(pct) / 100.0
	n := nScopes
	if n < 1 {
		n = 1
	}
	return int(math.Floor(float64(cap) * p / float64(n)))
}

func (ix *Indexer) incPendingBulk(scopeKey string) {
	ix.pendingBulkMu.Lock()
	defer ix.pendingBulkMu.Unlock()
	if ix.pendingBulkByScope == nil {
		ix.pendingBulkByScope = map[string]int64{}
	}
	ix.pendingBulkByScope[scopeKey]++
}

func (ix *Indexer) decPendingBulk(scopeKey string) {
	if scopeKey == "" {
		return
	}
	ix.pendingBulkMu.Lock()
	defer ix.pendingBulkMu.Unlock()
	if ix.pendingBulkByScope == nil {
		return
	}
	v := ix.pendingBulkByScope[scopeKey] - 1
	if v <= 0 {
		delete(ix.pendingBulkByScope, scopeKey)
		return
	}
	ix.pendingBulkByScope[scopeKey] = v
}

func (ix *Indexer) pendingBulk(scopeKey string) int64 {
	ix.pendingBulkMu.Lock()
	defer ix.pendingBulkMu.Unlock()
	if ix.pendingBulkByScope == nil {
		return 0
	}
	return ix.pendingBulkByScope[scopeKey]
}

// runScanJob performs corpus inventory load, walk per root, scoped discovery logs,
// and enqueues FanoutListJob shards (tier 1).
func (ix *Indexer) runScanJob(ctx context.Context, scanID string) error {
	if err := ix.loadRemoteCorpusInventory(ctx); err != nil {
		ix.log.Warn("corpus inventory fetch skipped", "err", err)
	}

	var all []TaggedCandidate
	perScopeWalk := map[string]*discoveryAgg{}

	noteSkip := func(root Root, rel, reason string) {
		sk := ix.scopeKeyFor(root, rel)
		if perScopeWalk[sk] == nil {
			perScopeWalk[sk] = &discoveryAgg{}
		}
		perScopeWalk[sk].noteSkip(reason)
		if ix.hooks.OnSkip != nil {
			ix.hooks.OnSkip(rel, reason)
		}
		args := []any{"rel", rel, "reason", reason}
		args = append(args, ix.logScopeFieldsForRootRel(root, rel)...)
		ix.log.Debug("skip", args...)
	}

	for _, r := range ix.cfg.Roots {
		m, err := NewMatcher(r.AbsPath, ix.cfg.IgnoreExtra)
		if err != nil {
			return fmt.Errorf("ignore matcher for %s: %w", r.AbsPath, err)
		}
		ix.matchers[r.ID] = m
		cands, err := Walk(r, WalkOptions{
			Matcher:              m,
			MaxFileBytes:         ix.cfg.MaxFileBytes,
			BinaryNullByteSample: ix.cfg.BinaryNullByteSample,
			BinaryNullByteRatio:  ix.cfg.BinaryNullByteRatio,
			OnSkip: func(rel, reason string) {
				noteSkip(r, rel, reason)
			},
		})
		if err != nil {
			return fmt.Errorf("walk %s: %w", r.AbsPath, err)
		}
		for _, c := range cands {
			proj, flav := ix.cfg.IngestHeaders(c.Root, c.RelPath)
			sk := ScopeKey(proj, flav)
			all = append(all, TaggedCandidate{
				Candidate: c,
				Project:   proj,
				Flavor:    flav,
			})
			if perScopeWalk[sk] == nil {
				perScopeWalk[sk] = &discoveryAgg{}
			}
			perScopeWalk[sk].Candidates++
		}
	}

	ix.replaceWorkspaceTotalsFromDiscovery(perScopeWalk)

	// Round-robin interleave by (project_id, flavor_id) so bulk fan-out and
	// ingest ordering mix workspaces instead of processing entire roots serially.
	all = interleaveTaggedCandidatesByScope(all)

	scopeSet := map[string]struct{}{}
	for sk := range perScopeWalk {
		scopeSet[sk] = struct{}{}
	}
	for _, tc := range all {
		scopeSet[ScopeKey(tc.Project, tc.Flavor)] = struct{}{}
	}
	nScopes := len(scopeSet)
	budget := ix.computePerScopeBudget(nScopes)
	cap := ix.queue.Cap()
	pct := effectiveQueueFanoutHWMPercent(ix.cfg.QueueFanoutHWMPercent)

	ix.log.Info("scan fan-out budget",
		"msg", "indexer.scan.complete",
		"scan_id", scanID,
		"n_scopes", nScopes,
		"per_scope_fanout_budget", budget,
		"queue_cap", cap,
		"queue_fanout_high_water_mark_percent", pct,
		"candidates_total", len(all),
	)

	tid := ix.tenantIDForLogs()
	for sk, d := range perScopeWalk {
		proj, flav := splitScopeKey(sk)
		ik := IndexerKey(tid, proj, flav)
		var paths []string
		for _, tc := range all {
			if ScopeKey(tc.Project, tc.Flavor) != sk {
				continue
			}
			paths = append(paths, tc.RelPath)
		}
		sort.Strings(paths)
		truncated := false
		if len(paths) > maxDiscoveryPathsLogged {
			truncated = true
			paths = paths[:maxDiscoveryPathsLogged]
		}
		args := []any{
			"msg", "indexer.discovery.summary.scope",
			"scan_id", scanID,
			"tenant_id", tid,
			"project_id", proj,
			"ingest_project", proj,
			"flavor_id", flav,
			"indexer_target_key", ik,
			"rel_paths", paths,
			"paths_truncated", truncated,
			"path_sample_count", len(paths),
		}
		args = append(args, d.discoveryScopeLogAttrs()...)
		ix.log.Info("discovery summary (scope)", args...)
	}

	meta := FanoutMeta{
		NScopes:               nScopes,
		PerScopeFanoutBudget:  budget,
		QueueFanoutHWMPercent: pct,
	}

	if len(all) == 0 {
		ix.log.Info("initial scan complete (no candidates)",
			"msg", "indexer.run.progress",
			"phase", "initial_scan",
			"scan_id", scanID,
			"candidates_enqueued", 0,
		)
		ix.initialScanCompleted.Store(true)
		ix.LogQueueSnapshot("after_initial_scan")
		return nil
	}

	if !ix.enqueueFanoutWork(all, meta) {
		return fmt.Errorf("could not enqueue fan-out jobs for scan %q", scanID)
	}

	ix.log.Info("initial scan discovery complete",
		"msg", "indexer.run.progress",
		"phase", "initial_scan",
		"scan_id", scanID,
		"candidates_total", len(all),
	)
	ix.initialScanCompleted.Store(true)
	ix.LogQueueSnapshot("after_initial_scan")
	return nil
}

func splitScopeKey(sk string) (project, flavor string) {
	i := strings.Index(sk, scopeKeySep)
	if i < 0 {
		return sk, ""
	}
	return sk[:i], sk[i+1:]
}

func (ix *Indexer) scopeKeyFor(root Root, rel string) string {
	p, f := ix.cfg.IngestHeaders(root, rel)
	return ScopeKey(p, f)
}

// interleaveTaggedCandidatesByScope reorders candidates in round-robin order
// across ScopeKey(project, flavor). Within each scope, walk order is preserved.
// Scope keys are visited in sorted order each round so behavior is deterministic.
func interleaveTaggedCandidatesByScope(cands []TaggedCandidate) []TaggedCandidate {
	if len(cands) <= 1 {
		return cands
	}
	buckets := make(map[string][]TaggedCandidate)
	for _, tc := range cands {
		sk := ScopeKey(tc.Project, tc.Flavor)
		buckets[sk] = append(buckets[sk], tc)
	}
	if len(buckets) <= 1 {
		return cands
	}
	keys := make([]string, 0, len(buckets))
	for k := range buckets {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]TaggedCandidate, 0, len(cands))
	for {
		var took int
		for _, k := range keys {
			q := buckets[k]
			if len(q) == 0 {
				continue
			}
			out = append(out, q[0])
			buckets[k] = q[1:]
			took++
		}
		if took == 0 {
			break
		}
	}
	return out
}

// enqueueFanoutWork shards candidates into one or more FanoutListJob entries.
func (ix *Indexer) enqueueFanoutWork(cands []TaggedCandidate, meta FanoutMeta) bool {
	if len(cands) == 0 {
		return true
	}
	const chunkSize = 4096
	for i := 0; i < len(cands); i += chunkSize {
		end := i + chunkSize
		if end > len(cands) {
			end = len(cands)
		}
		slice := append([]TaggedCandidate(nil), cands[i:end]...)
		w := WorkItem{
			Kind:       WorkFanoutList,
			Tier:       TierBulk,
			FanoutID:   nextFanoutID(),
			Candidates: slice,
			Meta:       meta,
		}
		if !ix.queue.Enqueue(w) {
			args := []any{
				"msg", "indexer.fanout.enqueue_failed",
				"candidates", len(slice),
			}
			args = append(args, ix.logScopeFieldsForTaggedSlice(slice)...)
			ix.log.Error("failed to enqueue fan-out list job", args...)
			return false
		}
	}
	return true
}

// runFanoutList drains candidates with fair-share limits and queue backpressure.
func (ix *Indexer) runFanoutList(ctx context.Context, wi WorkItem) error {
	meta := wi.Meta
	budget := meta.PerScopeFanoutBudget
	if budget <= 0 {
		budget = ix.computePerScopeBudget(meta.NScopes)
	}

	remaining := wi.Candidates
	for len(remaining) > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		tc := remaining[0]
		sk := ScopeKey(tc.Project, tc.Flavor)

		if ix.pendingBulk(sk)+1 > int64(budget) {
			return ix.splitFanoutRemainder(remaining, meta)
		}

		j := IngestEnqueue(Job{
			Root:    tc.Root,
			RelPath: tc.RelPath,
			AbsPath: tc.AbsPath,
		}, TierBulk, true, sk)

		if !ix.queue.Enqueue(j) {
			return ix.splitFanoutRemainder(remaining, meta)
		}
		ix.incPendingBulk(sk)
		remaining = remaining[1:]
	}
	return nil
}

func (ix *Indexer) splitFanoutRemainder(remaining []TaggedCandidate, meta FanoutMeta) error {
	if len(remaining) == 0 {
		return nil
	}
	w := WorkItem{
		Kind:       WorkFanoutList,
		Tier:       TierBulk,
		FanoutID:   nextFanoutID(),
		Candidates: append([]TaggedCandidate(nil), remaining...),
		Meta:       meta,
	}
	if ix.queue.Enqueue(w) {
		return nil
	}
	args := []any{
		"msg", "indexer.fanout.remainder_blocked",
		"remainder_size", len(remaining),
	}
	args = append(args, ix.logScopeFieldsForTaggedSlice(remaining)...)
	ix.log.Warn("queue full while retaining fan-out remainder; paths may be dropped until rescan", args...)
	return nil
}
