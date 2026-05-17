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
		"msg=chat.chimera-broker.request",
		"msg=chat.chimera-broker.response",
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
