package indexer

import (
	"sort"
	"strings"
)

// logScopeFieldsForJob returns slog-style key/value pairs so UI partitioners can
// associate an ingest-related line with tenant + project + flavor (see IndexerKey).
func (ix *Indexer) logScopeFieldsForJob(j Job) []any {
	return ix.logScopeFieldsForRootRel(j.Root, j.RelPath)
}

// logScopeFieldsForRootRel resolves ingest headers for (root, rel) and emits scope attrs.
func (ix *Indexer) logScopeFieldsForRootRel(root Root, rel string) []any {
	proj, flav := ix.cfg.IngestHeaders(root, rel)
	tid := ix.tenantIDForLogs()
	ik := IndexerKey(tid, proj, flav)
	out := []any{
		"tenant_id", tid,
		"project_id", proj,
		"ingest_project", proj,
		"flavor_id", flav,
		"indexer_target_key", ik,
		"root", root.ID,
	}
	if ws := strings.TrimSpace(root.Scope.WorkspaceID); ws != "" {
		out = append(out, "scope_workspace_id", ws)
	}
	return out
}

// logScopeFieldsForTaggedSlice annotates fan-out chunks that may contain multiple scopes.
// Single-scope chunks get the same fields as logScopeFieldsForRootRel; multi-scope chunks
// list distinct indexer_target_key values (sorted) for association without picking one workspace.
func (ix *Indexer) logScopeFieldsForTaggedSlice(cs []TaggedCandidate) []any {
	if len(cs) == 0 {
		return nil
	}
	tid := ix.tenantIDForLogs()
	type scopeRow struct {
		proj, flav, rootID string
	}
	byIK := map[string]scopeRow{}
	var keys []string
	for _, tc := range cs {
		ik := IndexerKey(tid, tc.Project, tc.Flavor)
		if _, ok := byIK[ik]; ok {
			continue
		}
		byIK[ik] = scopeRow{proj: tc.Project, flav: tc.Flavor, rootID: tc.Root.ID}
		keys = append(keys, ik)
	}
	sort.Strings(keys)
	if len(keys) == 1 {
		row := byIK[keys[0]]
		return []any{
			"tenant_id", tid,
			"project_id", row.proj,
			"ingest_project", row.proj,
			"flavor_id", row.flav,
			"indexer_target_key", keys[0],
			"root", row.rootID,
		}
	}
	return []any{
		"tenant_id", tid,
		"indexer_multi_scope_chunk", true,
		"distinct_scope_count", len(keys),
		"indexer_target_keys", strings.Join(keys, ","),
	}
}
