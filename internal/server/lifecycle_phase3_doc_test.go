package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Phase 3 (log-conversations.md): fixture lists each conversation.* lifecycle msg slug for UI contracts.
func TestPhase3_lifecycleFixtureContainsAllMsgs(t *testing.T) {
	t.Parallel()
	path := filepath.Join("testdata", "correlation", "lifecycle-phase3.example.log")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	text := string(b)
	want := []string{
		"msg=conversation.received",
		"msg=conversation.merged",
		"msg=conversation.dedup_hit",
		"msg=conversation.routing.resolved",
		"msg=conversation.rag.span",
		"msg=conversation.rag.skipped",
		"msg=conversation.rag.attached",
		"msg=conversation.upstream.started",
		"msg=conversation.upstream.completed",
		"msg=conversation.upstream.failed",
		"msg=conversation.fallback.attempted",
		"msg=conversation.fallback.exhausted",
		"msg=conversation.delivered",
		"msg=conversation.errored",
	}
	for _, w := range want {
		if !strings.Contains(text, w) {
			t.Fatalf("fixture %s missing %q", path, w)
		}
	}
}
