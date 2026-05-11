package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Phase 7 (log-conversations.md): tool router and relay-side tool call lines carry
// the correlation triple plus turn_index; fixture exercises success and failure slugs.
func TestPhase7_toolsFixtureCarriesCorrelation(t *testing.T) {
	t.Parallel()
	path := filepath.Join("testdata", "correlation", "phase7-tools.example.log")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	requiredMsgs := []string{
		"msg=conversation.tool.router",
		"msg=conversation.tool.call_started",
		"msg=conversation.tool.call_completed",
		"msg=conversation.tool.call_failed",
	}
	for _, needle := range requiredMsgs {
		if !strings.Contains(string(b), needle) {
			t.Fatalf("fixture %s missing %s", path, needle)
		}
	}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !strings.Contains(line, "msg=conversation.tool.") {
			continue
		}
		for _, key := range []string{"request_id=", "conversation_id=", "principal_id=", "turn_index="} {
			if !strings.Contains(line, key) {
				t.Fatalf("fixture %s line missing %s: %s", path, key, line)
			}
		}
	}
}
