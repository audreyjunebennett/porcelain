package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Phase 8 (log-conversations.md): witness lines carry correlation + turn_index; payload sample is trace-only in fixtures.
func TestPhase8_witnessFixtureCarriesCorrelation(t *testing.T) {
	t.Parallel()
	path := filepath.Join("testdata", "correlation", "phase8-witness.example.log")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	for _, needle := range []string{
		"msg=conversation.request.witness",
		"msg=conversation.response.witness",
		"msg=conversation.payload.sample",
	} {
		if !strings.Contains(string(b), needle) {
			t.Fatalf("fixture %s missing %s", path, needle)
		}
	}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !strings.Contains(line, "msg=conversation.request.witness") &&
			!strings.Contains(line, "msg=conversation.response.witness") &&
			!strings.Contains(line, "msg=conversation.payload.sample") {
			continue
		}
		for _, key := range []string{"request_id=", "conversation_id=", "principal_id=", "turn_index="} {
			if !strings.Contains(line, key) {
				t.Fatalf("fixture %s line missing %s: %s", path, key, line)
			}
		}
	}
}
