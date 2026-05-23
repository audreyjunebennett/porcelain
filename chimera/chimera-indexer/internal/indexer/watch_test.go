package indexer

import (
	"context"
	"errors"
	"testing"

	"github.com/fsnotify/fsnotify"
)

func TestAddRecursiveWatchRespectsCancel(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	w, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = addRecursiveWatch(ctx, w, root)
	if err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled or nil, got %v", err)
	}
}
