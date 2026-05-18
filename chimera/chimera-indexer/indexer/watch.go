package indexer

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/fsnotify/fsnotify"
)

// RunWatchers wires fsnotify watchers onto every configured root and
// translates create/write events into queued jobs (debounced per path).
// Returns when ctx is cancelled.
func (ix *Indexer) RunWatchers(ctx context.Context) error {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("fsnotify: %w", err)
	}
	defer w.Close()

	for _, r := range ix.cfg.Roots {
		if err := addRecursive(w, r.AbsPath); err != nil {
			return fmt.Errorf("watch %s: %w", r.AbsPath, err)
		}
	}

	debouncer := newDebouncer(ix.cfg.Debounce, func(absPath string, tier PriorityTier) {
		root, rel, ok := ix.matchAbs(absPath)
		if !ok {
			return
		}
		m := ix.matchers[root.ID]
		if m != nil && m.Match(rel) {
			return
		}
		st, err := os.Stat(absPath)
		if err != nil || !st.Mode().IsRegular() {
			return
		}
		if ix.cfg.MaxFileBytes > 0 && st.Size() > ix.cfg.MaxFileBytes {
			return
		}
		bin, err := IsBinaryFile(absPath, ix.cfg.BinaryNullByteSample, ix.cfg.BinaryNullByteRatio)
		if err != nil || bin {
			return
		}
		job := Job{Root: root, RelPath: rel, AbsPath: absPath}
		wasPending := ix.queue.HasPendingKey(job.Key())
		w := IngestEnqueue(job, tier, false, "")
		if ix.queue.Enqueue(w) {
			if tier == TierInteractive && !wasPending {
				ix.bumpWorkspaceFileCount(root, rel)
			}
		}
	})
	defer debouncer.Close()

	for {
		select {
		case <-ctx.Done():
			return nil
		case ev, ok := <-w.Events:
			if !ok {
				return nil
			}
			if ev.Op&(fsnotify.Create|fsnotify.Write) != 0 {
				tier := TierWrite
				if ev.Op&fsnotify.Create != 0 {
					tier = TierInteractive
				}
				debouncer.Trigger(ev.Name, tier)
			}
			if ev.Op&fsnotify.Remove != 0 {
				ix.onWatchedPathRemoved(ev.Name)
			}
			if ev.Op&fsnotify.Create != 0 {
				if st, err := os.Stat(ev.Name); err == nil && st.IsDir() {
					_ = addRecursive(w, ev.Name)
				}
			}
		case err, ok := <-w.Errors:
			if !ok {
				return nil
			}
			ix.log.Warn("fsnotify error", "err", err)
		}
	}
}

func addRecursive(w *fsnotify.Watcher, root string) error {
	return filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if d.IsDir() {
			return w.Add(p)
		}
		return nil
	})
}

func (ix *Indexer) matchAbs(abs string) (Root, string, bool) {
	for _, r := range ix.cfg.Roots {
		if rel, ok := relPath(r.AbsPath, abs); ok {
			return r, rel, true
		}
	}
	return Root{}, "", false
}

func (ix *Indexer) onWatchedPathRemoved(absPath string) {
	root, rel, ok := ix.matchAbs(absPath)
	if !ok {
		return
	}
	if st, err := os.Stat(absPath); err == nil && st.IsDir() {
		return
	}
	ix.decrementWorkspaceFileCount(root, rel)
}
