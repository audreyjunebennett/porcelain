package indexer

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestWatchConfigPathForReload_TriggersOnWrite(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "cfg.yaml")
	if err := os.WriteFile(cfgPath, []byte("roots: []\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()

	var n atomic.Int32
	errCh := make(chan error, 1)
	go func() {
		errCh <- WatchConfigPathForReload(ctx, cfgPath, 150*time.Millisecond, func() {
			n.Add(1)
		}, nil)
	}()

	time.Sleep(300 * time.Millisecond)
	if err := os.WriteFile(cfgPath, []byte("roots: []\nmodified: true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(3 * time.Second)
	for n.Load() < 1 {
		select {
		case <-deadline:
			t.Fatalf("reload callback not fired, n=%d", n.Load())
		case <-time.After(50 * time.Millisecond):
		}
	}

	cancel()
	err := <-errCh
	if err != nil && err != context.Canceled {
		t.Fatalf("watch return: %v", err)
	}
}
