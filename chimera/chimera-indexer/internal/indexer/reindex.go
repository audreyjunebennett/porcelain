package indexer

import (
	"context"
	"log/slog"
	"sync"
)

// ReindexTracker remembers last applied reindex_generation per workspace id.
type ReindexTracker struct {
	mu   sync.Mutex
	last map[int64]int64
}

// NewReindexTracker constructs an empty tracker.
func NewReindexTracker() *ReindexTracker {
	return &ReindexTracker{last: map[int64]int64{}}
}

// ApplyWorkspacesReindex clears sync checkpoints for roots whose workspace generation increased.
// Returns true when any workspace was bumped (caller may trigger session reload).
func ApplyWorkspacesReindex(ctx context.Context, st *SyncState, resp *WorkspacesAPIResponse, tr *ReindexTracker, log *slog.Logger) bool {
	if st == nil || resp == nil || tr == nil {
		return false
	}
	_ = ctx
	var applied bool
	for _, ws := range resp.Workspaces {
		wid := ws.effectiveWorkspaceID()
		if wid == 0 {
			continue
		}
		gen := ws.ReindexGeneration
		tr.mu.Lock()
		prev, seen := tr.last[wid]
		if !seen {
			// Baseline generation on first poll after process start; do not clear sync.
			tr.last[wid] = gen
			tr.mu.Unlock()
			continue
		}
		if gen <= prev {
			tr.mu.Unlock()
			continue
		}
		tr.last[wid] = gen
		tr.mu.Unlock()

		roots, err := rootsForWorkspace(resp, wid)
		if err != nil {
			if log != nil {
				log.Warn("reindex: could not map workspace roots",
					"msg", "indexer.reindex.requested",
					"workspace_id", wid,
					"err", err,
				)
			}
			continue
		}
		for _, root := range roots {
			if err := st.DeleteByRoot(root.ID); err != nil && log != nil {
				log.Warn("reindex: sync state delete failed",
					"msg", "indexer.reindex.requested",
					"workspace_id", wid,
					"root_id", root.ID,
					"err", err,
				)
			}
		}
		applied = true
		if log != nil {
			log.Info("reindex generation applied; sync checkpoints cleared",
				"msg", "indexer.reindex.requested",
				"workspace_id", wid,
				"project_id", ws.ProjectID,
				"flavor_id", ws.FlavorID,
				"generation", gen,
				"roots", len(roots),
			)
		}
	}
	return applied
}

func rootsForWorkspace(resp *WorkspacesAPIResponse, workspaceID int64) ([]Root, error) {
	sub := &WorkspacesAPIResponse{Workspaces: nil}
	for _, w := range resp.Workspaces {
		if w.effectiveWorkspaceID() == workspaceID {
			sub.Workspaces = []WorkspaceAPIEntry{w}
			break
		}
	}
	if len(sub.Workspaces) == 0 {
		return nil, nil
	}
	return RootsFromWorkspacesResponse(sub)
}
