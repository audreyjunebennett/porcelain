package indexer

import (
	"context"
	"fmt"
	"time"

	"github.com/fsnotify/fsnotify"
)

const defaultWorkspacesPollInterval = 30 * time.Second

// rootsDelta is a watch-session update applied without restarting RunWatchers.
type rootsDelta struct {
	added   []Root
	removed []Root
}

// WorkspacesPollInterval returns the supervised workspace poll cadence from
// resolved config, or defaultWorkspacesPollInterval when unset.
func WorkspacesPollInterval(cfg Resolved) time.Duration {
	if cfg.WorkspacesPollInterval > 0 {
		return cfg.WorkspacesPollInterval
	}
	return defaultWorkspacesPollInterval
}

// GetRoots returns a copy of the active watch roots.
func (ix *Indexer) GetRoots() []Root {
	return ix.getRoots()
}

func (ix *Indexer) getRoots() []Root {
	ix.rootsMu.RLock()
	defer ix.rootsMu.RUnlock()
	out := make([]Root, len(ix.cfg.Roots))
	copy(out, ix.cfg.Roots)
	return out
}

func (ix *Indexer) setRoots(roots []Root) {
	ix.rootsMu.Lock()
	ix.cfg.Roots = append([]Root(nil), roots...)
	ix.rootsMu.Unlock()
}

// ApplyRootsSnapshot replaces the active watch list and notifies the live
// watcher loop. Returns true when roots changed.
func (ix *Indexer) ApplyRootsSnapshot(ctx context.Context, newRoots []Root) (changed bool, err error) {
	if ix == nil {
		return false, fmt.Errorf("indexer is nil")
	}
	prev := ix.getRoots()
	added, removed := DiffRoots(prev, newRoots)
	if len(added) == 0 && len(removed) == 0 {
		return false, nil
	}

	for _, r := range removed {
		if ix.syncState != nil {
			if gw := ix.lastGW.Load(); gw != nil {
				PushStaleSources(ctx, ix.client, ix.cfg, gw, ix.syncState, []Root{r}, nil)
			}
			_ = ix.syncState.DeleteByRoot(r.ID)
		}
		ix.rootsMu.Lock()
		delete(ix.matchers, r.ID)
		ix.rootsMu.Unlock()
		ix.log.Info("watch root removed",
			"msg", "indexer.supervised.root_removed",
			"type", "indexer.supervised.root_removed",
			"root_id", r.ID,
			"abs_path", r.AbsPath,
		)
	}

	ix.setRoots(newRoots)

	delta := rootsDelta{added: added, removed: removed}
	if ch := ix.rootUpdates; ch != nil {
		select {
		case ch <- delta:
		default:
			select {
			case <-ch:
			default:
			}
			ch <- delta
		}
	}

	if len(added) > 0 {
		_ = ix.ScheduleScanForRoots(added, "workspace-add")
	}
	return true, nil
}

// ApplyTuning copies non-root fields from next into the live indexer config
// without changing watch roots or index_run_id.
func (ix *Indexer) ApplyTuning(next Resolved) {
	if ix == nil {
		return
	}
	roots := ix.getRoots()
	ix.cfgMu.Lock()
	defer ix.cfgMu.Unlock()
	ix.cfg.GatewayURL = next.GatewayURL
	ix.cfg.Token = next.Token
	ix.cfg.IgnoreExtra = append([]string(nil), next.IgnoreExtra...)
	ix.cfg.DefaultScope = next.DefaultScope
	ix.cfg.GlobOverrides = append([]GlobOverride(nil), next.GlobOverrides...)
	ix.cfg.RetryMaxAttempts = next.RetryMaxAttempts
	ix.cfg.RetryBaseDelay = next.RetryBaseDelay
	ix.cfg.RetryMaxDelay = next.RetryMaxDelay
	ix.cfg.RecoveryPollInterval = next.RecoveryPollInterval
	ix.cfg.Debounce = next.Debounce
	ix.cfg.Workers = next.Workers
	ix.cfg.QueueDepth = next.QueueDepth
	ix.cfg.MaxFileBytes = next.MaxFileBytes
	ix.cfg.RequestTimeout = next.RequestTimeout
	ix.cfg.BinaryNullByteSample = next.BinaryNullByteSample
	ix.cfg.BinaryNullByteRatio = next.BinaryNullByteRatio
	ix.cfg.SyncStatePath = next.SyncStatePath
	ix.cfg.MaxWholeFileBytes = next.MaxWholeFileBytes
	ix.cfg.RecoveryIncludeRootHealth = next.RecoveryIncludeRootHealth
	ix.cfg.RetryShortCircuitOnEmbed = next.RetryShortCircuitOnEmbed
	ix.cfg.LogLevel = next.LogLevel
	ix.cfg.JobSkipLog = next.JobSkipLog
	ix.cfg.JobIngestLog = next.JobIngestLog
	ix.cfg.StorageStatsPoll = next.StorageStatsPoll
	ix.cfg.QueueFanoutHWMPercent = next.QueueFanoutHWMPercent
	ix.cfg.ScopeStatusPoll = next.ScopeStatusPoll
	ix.cfg.ScopeActiveFileLogMinInterval = next.ScopeActiveFileLogMinInterval
	ix.cfg.SkipSummaryMinInterval = next.SkipSummaryMinInterval
	ix.cfg.ScopeStatusIngestMilestone = next.ScopeStatusIngestMilestone
	ix.cfg.ScopeStatusEdgeMinInterval = next.ScopeStatusEdgeMinInterval
	ix.cfg.QueueSnapshotIdleInfoInterval = next.QueueSnapshotIdleInfoInterval
	ix.cfg.WorkspacesPollInterval = next.WorkspacesPollInterval
	ix.cfg.SupervisedLayer = next.SupervisedLayer
	ix.cfg.Roots = roots
}

func (ix *Indexer) applyRootsDelta(ctx context.Context, w *fsnotify.Watcher, delta rootsDelta) error {
	ix.watchMu.Lock()
	defer ix.watchMu.Unlock()
	if ix.watchedPaths == nil {
		ix.watchedPaths = map[string][]string{}
	}
	for _, r := range delta.removed {
		paths := ix.watchedPaths[r.ID]
		removeWatchPaths(w, paths)
		delete(ix.watchedPaths, r.ID)
	}
	for _, r := range delta.added {
		m, err := NewMatcher(r.AbsPath, ix.cfg.IgnoreExtra)
		if err != nil {
			return fmt.Errorf("ignore matcher for %s: %w", r.AbsPath, err)
		}
		ix.rootsMu.Lock()
		ix.matchers[r.ID] = m
		ix.rootsMu.Unlock()
		paths, err := registerRecursiveWatchPaths(ctx, w, r.AbsPath, m)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("watch %s: %w", r.AbsPath, err)
		}
		ix.watchedPaths[r.ID] = paths
		ix.log.Info("watch root added",
			"msg", "indexer.supervised.root_added",
			"type", "indexer.supervised.root_added",
			"root_id", r.ID,
			"abs_path", r.AbsPath,
			"watch_dirs", len(paths),
		)
	}
	return nil
}

func removeWatchPaths(w *fsnotify.Watcher, paths []string) {
	if w == nil {
		return
	}
	for _, p := range paths {
		_ = w.Remove(p)
	}
}
