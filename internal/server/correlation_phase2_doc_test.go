package server

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPhase2_correlationDocFixturesPresent(t *testing.T) {
	t.Parallel()
	readme := filepath.Join("testdata", "correlation", "README.txt")
	if b, err := os.ReadFile(readme); err != nil || len(b) < 20 {
		t.Fatalf("missing or short %s: %v", readme, err)
	}
	example := filepath.Join("testdata", "correlation", "ingest-complete-with-conversation.example.log")
	if b, err := os.ReadFile(example); err != nil || len(b) < 20 {
		t.Fatalf("missing or short %s: %v", example, err)
	}
	lc := filepath.Join("testdata", "correlation", "lifecycle-phase3.example.log")
	if b, err := os.ReadFile(lc); err != nil || len(b) < 20 {
		t.Fatalf("missing or short %s: %v", lc, err)
	}
	p5 := filepath.Join("testdata", "correlation", "phase5-qdrant-tier4b.example.log")
	if b, err := os.ReadFile(p5); err != nil || len(b) < 20 {
		t.Fatalf("missing or short %s: %v", p5, err)
	}
	p6 := filepath.Join("testdata", "correlation", "phase6-multi-turn.example.log")
	if b, err := os.ReadFile(p6); err != nil || len(b) < 20 {
		t.Fatalf("missing or short %s: %v", p6, err)
	}
}
