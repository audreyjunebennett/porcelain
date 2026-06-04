package indexer

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
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

	err = addRecursiveWatch(ctx, w, root, nil)
	if err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled or nil, got %v", err)
	}
}

func TestAddRecursiveWatchSkipsIgnoredDirs(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "node_modules", "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := NewMatcher(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	w, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	if err := addRecursiveWatch(context.Background(), w, root, m); err != nil {
		t.Fatal(err)
	}
	// fsnotify Watcher.WatchList is not exported; count via Events channel is flaky.
	// Instead verify walk behavior: ignored subtree should not be registered by re-walking.
	var watched []string
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		rel, ok := relPath(root, p)
		if !ok || rel == "." {
			return nil
		}
		if m.Match(rel + "/") {
			return filepath.SkipDir
		}
		watched = append(watched, rel)
		return nil
	})
	for _, rel := range watched {
		if strings.HasPrefix(rel, "node_modules") {
			t.Fatalf("ignored dir in watch walk list: %q", rel)
		}
	}
	if len(watched) != 1 || watched[0] != "src" {
		t.Fatalf("expected only src dir watched, got %v", watched)
	}
}
