package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Phase 6 (log-conversations.md): every chat-scoped slug for both turns must carry
// the matching turn_index, so operators can group expanded rows by turn deterministically.
func TestPhase6_multiTurnFixtureCarriesTurnIndex(t *testing.T) {
	t.Parallel()
	path := filepath.Join("testdata", "correlation", "phase6-multi-turn.example.log")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	turnSlugs := []string{
		"msg=conversation.received",
		"msg=chat.request",
		"msg=conversation.routing.resolved",
		"msg=chat.bifrost.request",
		"msg=chat.bifrost.response",
		"msg=conversation.upstream.completed",
		"msg=conversation.delivered",
	}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		var slug string
		for _, candidate := range turnSlugs {
			if strings.Contains(line, candidate) {
				slug = candidate
				break
			}
		}
		if slug == "" {
			continue
		}
		if !strings.Contains(line, "turn_index=") {
			t.Fatalf("fixture %s line missing turn_index for %s: %s", path, slug, line)
		}
	}
	if !strings.Contains(string(b), "turn_index=1") || !strings.Contains(string(b), "turn_index=2") {
		t.Fatalf("fixture %s must exercise turn_index=1 and turn_index=2", path)
	}
}
