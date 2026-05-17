package indexer

import (
	"context"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
)

// DefaultConfigReloadDebounce is the delay after the last filesystem event
// before treating a supervised --config file as changed (coalesces rapid writes).
const DefaultConfigReloadDebounce = 800 * time.Millisecond

// WatchConfigPathForReload watches the parent directory of absConfigPath and
// invokes onReload (debounced) when that file is written or replaced.
// It returns when ctx is cancelled or the watcher ends unexpectedly.
func WatchConfigPathForReload(ctx context.Context, absConfigPath string, debounce time.Duration, onReload func(), log *slog.Logger) error {
	if debounce <= 0 {
		debounce = DefaultConfigReloadDebounce
	}
	absConfigPath, err := filepath.Abs(absConfigPath)
	if err != nil {
		return err
	}
	dir := filepath.Dir(absConfigPath)
	wantBase := filepath.Base(absConfigPath)

	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer w.Close()
	if err := w.Add(dir); err != nil {
		return err
	}

	match := func(name string) bool {
		b := filepath.Base(name)
		return strings.EqualFold(b, wantBase)
	}

	deb := newDebouncer(debounce, func(string, PriorityTier) {
		if onReload != nil {
			onReload()
		}
	})
	defer deb.Close()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err, ok := <-w.Errors:
			if !ok {
				return nil
			}
			if err != nil && log != nil {
				log.Warn("indexer config watch: fsnotify error", "err", err)
			}
		case ev, ok := <-w.Events:
			if !ok {
				return nil
			}
			if !match(ev.Name) {
				continue
			}
			if ev.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename|fsnotify.Remove) != 0 {
				deb.Trigger("__config_reload__", TierBulk)
			}
		}
	}
}
