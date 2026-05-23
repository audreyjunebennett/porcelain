package indexerapi

import (
	"testing"
	"time"

	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/catalog"
)

func TestProviderFromModelID(t *testing.T) {
	t.Parallel()
	if got := providerFromModelID("ollama/nomic-embed-text:latest"); got != "ollama" {
		t.Fatalf("got %q", got)
	}
	if got := providerFromModelID("test-embed"); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestProviderStateFromSnapshot(t *testing.T) {
	snap := catalog.NewTestSnapshotWithModels(time.Now(), []string{"ollama/nomic-embed-text:latest"})
	if state := providerStateFromSnapshot(snap, "ollama"); state != "up" {
		t.Fatalf("state=%q", state)
	}
	if state := providerStateFromSnapshot(snap, "groq"); state != "down" {
		t.Fatalf("state=%q", state)
	}
}

func TestCatalogStaleDetail(t *testing.T) {
	if got := catalogStaleDetail(nil); got == "" {
		t.Fatal("expected detail for nil snapshot")
	}
	failed := catalog.NewTestFailedSnapshot(time.Now(), "fetch failed")
	if got := catalogStaleDetail(failed); got != "fetch failed" {
		t.Fatalf("got %q", got)
	}
}
