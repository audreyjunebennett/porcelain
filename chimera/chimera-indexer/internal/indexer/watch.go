package indexer

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/fsnotify/fsnotify"
)

// RunWatchers wires fsnotify watchers onto every configured root and
// translates create/write events into queued jobs (debounced per path).
// Root add/remove is applied in-process via ApplyRootsSnapshot without
// restarting this loop. Returns when ctx is cancelled.
func (ix *Indexer) RunWatchers(ctx context.Context) error {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("fsnotify: %w", err)
	}
	defer w.Close()

	ix.rootUpdates = make(chan rootsDelta, 1)
	ix.watchMu.Lock()
	ix.watchedPaths = map[string][]string{}
	ix.watchMu.Unlock()

	if err := ix.ensureMatchers(); err != nil {
		return err
	}

	roots := ix.getRoots()
	if err := ix.registerInitialWatches(ctx, w, roots); err != nil {
		if ctx.Err() != nil || errors.Is(err, context.Canceled) {
			return nil
		}
		return err
	}

	debouncer := newDebouncer(ix.cfg.Debounce, func(absPath string, tier PriorityTier) {
		root, rel, ok := ix.matchAbs(absPath)
		if !ok {
			return
		}
		ix.rootsMu.RLock()
		m := ix.matchers[root.ID]
		ix.rootsMu.RUnlock()
		if m != nil && m.Match(rel) {
			return
		}
		st, err := os.Stat(absPath)
		if err != nil || !st.Mode().IsRegular() {
			return
		}
		ix.cfgMu.RLock()
		maxBytes := ix.cfg.MaxFileBytes
		sample := ix.cfg.BinaryNullByteSample
		ratio := ix.cfg.BinaryNullByteRatio
		ix.cfgMu.RUnlock()
		if maxBytes > 0 && st.Size() > maxBytes {
			return
		}
		bin, err := IsBinaryFile(absPath, sample, ratio)
		if err != nil || bin {
			return
		}
		job := Job{Root: root, RelPath: rel, AbsPath: absPath}
		wasPending := ix.queue.HasPendingKey(job.Key())
		wi := IngestEnqueue(job, tier, false, "")
		if ix.queue.Enqueue(wi) {
			if tier == TierInteractive && !wasPending {
				ix.bumpWorkspaceFileCount(root, rel)
			}
		}
	})
	defer debouncer.Close()

	for {
		select {
		case <-ctx.Done():
			_ = w.Close()
			return nil
		case delta, ok := <-ix.rootUpdates:
			if !ok {
				return nil
			}
			if err := ix.applyRootsDelta(ctx, w, delta); err != nil {
				if ctx.Err() != nil {
					return nil
				}
				ix.log.Warn("dynamic root update failed", "err", err)
			}
		case ev, ok := <-w.Events:
			if !ok {
				return nil
			}
			if ctx.Err() != nil {
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
					root, _, ok := ix.matchAbs(ev.Name)
					var m *Matcher
					if ok {
						ix.rootsMu.RLock()
						m = ix.matchers[root.ID]
						ix.rootsMu.RUnlock()
					}
					_ = registerRecursiveWatches(ctx, w, []Root{{ID: root.ID, AbsPath: ev.Name}}, map[string]*Matcher{root.ID: m})
				}
			}
		case err, ok := <-w.Errors:
			if !ok {
				return nil
			}
			if ctx.Err() != nil {
				return nil
			}
			ix.log.Warn("fsnotify error", "err", err)
		}
	}
}

func (ix *Indexer) registerInitialWatches(ctx context.Context, w *fsnotify.Watcher, roots []Root) error {
	if err := ix.ensureMatchers(); err != nil {
		return err
	}
	ix.watchMu.Lock()
	defer ix.watchMu.Unlock()
	if ix.watchedPaths == nil {
		ix.watchedPaths = map[string][]string{}
	}
	for _, r := range roots {
		m := ix.matchers[r.ID]
		if m == nil {
			var err error
			m, err = NewMatcher(r.AbsPath, ix.cfg.IgnoreExtra)
			if err != nil {
				return fmt.Errorf("ignore matcher for %s: %w", r.AbsPath, err)
			}
			ix.matchers[r.ID] = m
		}
		paths, err := registerRecursiveWatchPaths(ctx, w, r.AbsPath, m)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("watch %s: %w", r.AbsPath, err)
		}
		ix.watchedPaths[r.ID] = paths
	}
	return nil
}

func (ix *Indexer) ensureMatchers() error {
	ix.rootsMu.Lock()
	defer ix.rootsMu.Unlock()
	if ix.matchers == nil {
		ix.matchers = map[string]*Matcher{}
	}
	for _, r := range ix.cfg.Roots {
		if ix.matchers[r.ID] != nil {
			continue
		}
		m, err := NewMatcher(r.AbsPath, ix.cfg.IgnoreExtra)
		if err != nil {
			return fmt.Errorf("ignore matcher for %s: %w", r.AbsPath, err)
		}
		ix.matchers[r.ID] = m
	}
	return nil
}

// registerRecursiveWatches adds fsnotify watches per root in parallel so session
// cancel during supervised reload returns without waiting for a full tree walk.
func registerRecursiveWatches(ctx context.Context, w *fsnotify.Watcher, roots []Root, matchers map[string]*Matcher) error {
	if len(roots) == 0 {
		return nil
	}
	type result struct {
		root Root
		err  error
	}
	ch := make(chan result, len(roots))
	for _, r := range roots {
		r := r
		m := matchers[r.ID]
		go func() {
			_, err := registerRecursiveWatchPaths(ctx, w, r.AbsPath, m)
			ch <- result{r, err}
		}()
	}
	var firstErr error
	for range roots {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case res := <-ch:
			if res.err != nil && firstErr == nil && ctx.Err() == nil {
				firstErr = fmt.Errorf("watch %s: %w", res.root.AbsPath, res.err)
			}
		}
	}
	return firstErr
}

func registerRecursiveWatchPaths(ctx context.Context, w *fsnotify.Watcher, root string, matcher *Matcher) ([]string, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	root = filepath.Clean(root)
	var paths []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if d.IsDir() {
			if matcher != nil {
				rel, ok := relPath(root, p)
				if ok && rel != "." && matcher.Match(rel+"/") {
					return filepath.SkipDir
				}
			}
			if addErr := w.Add(p); addErr != nil {
				return addErr
			}
			paths = append(paths, p)
		}
		return nil
	})
	return paths, err
}

func addRecursiveWatch(ctx context.Context, w *fsnotify.Watcher, root string, matcher *Matcher) error {
	_, err := registerRecursiveWatchPaths(ctx, w, root, matcher)
	return err
}

func (ix *Indexer) matchAbs(abs string) (Root, string, bool) {
	for _, r := range ix.getRoots() {
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
