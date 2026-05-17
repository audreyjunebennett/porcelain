package routing

import (
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func discardLog() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError + 1}))
}

func TestStartingFallbackIndex(t *testing.T) {
	chain := []string{"a", "b", "c"}
	if StartingFallbackIndex("b", chain) != 1 {
		t.Fatal()
	}
	if StartingFallbackIndex("missing", chain) != 0 {
		t.Fatal()
	}
}

func TestPickInitialModel_RulesAndDefault(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "routing-policy.yaml")
	if err := os.WriteFile(policyPath, []byte(`
ambiguous_default_model: "gemini/default"
rules:
  - name: long
    when:
      min_message_chars: 10
    models:
      - "groq/big"
  - name: catch-all
    when: {}
    models:
      - "groq/small"
`), 0o644); err != nil {
		t.Fatal(err)
	}

	p := NewPolicy(policyPath, discardLog())
	chain := []string{"groq/small", "groq/big", "gemini/default"}
	vm := "Chimera-0.1.0"

	shortBody := map[string]json.RawMessage{
		"model":    mustRaw(t, vm),
		"messages": mustRaw(t, []map[string]string{{"role": "user", "content": "hi"}}),
	}
	m, via := p.PickInitialModel(shortBody, chain, vm)
	if m != "groq/small" || via != ViaRule {
		t.Fatalf("short: got %q %v", m, via)
	}

	longBody := map[string]json.RawMessage{
		"model":    mustRaw(t, vm),
		"messages": mustRaw(t, []map[string]string{{"role": "user", "content": "01234567890"}}),
	}
	m, via = p.PickInitialModel(longBody, chain, vm)
	if m != "groq/big" || via != ViaRule {
		t.Fatalf("long: got %q %v", m, via)
	}

	// No rules match min chars but we have catch-all — still groq/small from catch-all
}

func TestPickInitialModel_AmbiguousWhenNoRuleMatch(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "routing.yaml")
	if err := os.WriteFile(policyPath, []byte(`
ambiguous_default_model: "gemini/fallback"
rules:
  - name: never
    when:
      min_message_chars: 999999
    models:
      - "groq/x"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	p := NewPolicy(policyPath, discardLog())
	vm := "Chimera-0.1.0"
	body := map[string]json.RawMessage{
		"model":    mustRaw(t, vm),
		"messages": mustRaw(t, []map[string]string{{"role": "user", "content": "a"}}),
	}
	m, via := p.PickInitialModel(body, []string{"a", "b"}, vm)
	if m != "gemini/fallback" || via != ViaAmbiguousDefault {
		t.Fatalf("got %q %v", m, via)
	}
}

func TestPickInitialModel_FallbackChainFirst(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "routing2.yaml")
	if err := os.WriteFile(policyPath, []byte(`rules: []`), 0o644); err != nil {
		t.Fatal(err)
	}
	p := NewPolicy(policyPath, discardLog())
	vm := "Chimera-1.0.0"
	body := map[string]json.RawMessage{
		"model": mustRaw(t, vm),
	}
	m, via := p.PickInitialModel(body, []string{"first", "second"}, vm)
	if m != "first" || via != ViaChainOnly {
		t.Fatalf("got %q %v", m, via)
	}
}

func TestEvaluatePick(t *testing.T) {
	yaml := []byte(`
ambiguous_default_model: "gemini/default"
rules:
  - name: long
    when:
      min_message_chars: 10
    models:
      - "groq/big"
  - name: catch-all
    when: {}
    models:
      - "groq/small"
`)
	vm := "Chimera-0.1.0"
	chain := []string{"groq/small", "groq/big", "gemini/default"}
	shortBody := map[string]json.RawMessage{
		"model":    mustRaw(t, vm),
		"messages": mustRaw(t, []map[string]string{{"role": "user", "content": "hi"}}),
	}
	m, via, err := EvaluatePick(yaml, shortBody, chain, vm, discardLog())
	if err != nil || m != "groq/small" || via != ViaRule {
		t.Fatalf("short: %q %v %v", m, via, err)
	}
	longBody := map[string]json.RawMessage{
		"model":    mustRaw(t, vm),
		"messages": mustRaw(t, []map[string]string{{"role": "user", "content": "01234567890"}}),
	}
	m, via, err = EvaluatePick(yaml, longBody, chain, vm, discardLog())
	if err != nil || m != "groq/big" || via != ViaRule {
		t.Fatalf("long: %q %v %v", m, via, err)
	}
}

func mustRaw(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
