package server

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

func TestToolRelayStringFailed(t *testing.T) {
	t.Parallel()
	if !toolRelayStringFailed(`Error: something went wrong`) {
		t.Fatal("expected prefix Error:")
	}
	if !toolRelayStringFailed(`{"is_error":true}`) {
		t.Fatal("expected is_error json in string")
	}
	if toolRelayStringFailed(`all good`) {
		t.Fatal("expected false")
	}
}

func TestToolRelayContentFailed_object(t *testing.T) {
	t.Parallel()
	raw := json.RawMessage(`{"is_error":true,"content":"x"}`)
	if !toolRelayContentFailed(raw) {
		t.Fatal("expected object is_error")
	}
	raw2 := json.RawMessage(`{"error":{"type":"tool","message":"nope"}}`)
	if !toolRelayContentFailed(raw2) {
		t.Fatal("expected object error key")
	}
}

func TestLogConversationIncomingToolMessages_smoke(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	msgs := []map[string]any{
		{"role": "user", "content": "hi"},
		{"role": "tool", "tool_call_id": "call_ok", "name": "read_file", "content": "file contents"},
		{"role": "tool", "tool_call_id": "call_bad", "name": "run_cmd", "content": "Error: exit 1"},
	}
	b, err := json.Marshal(msgs)
	if err != nil {
		t.Fatal(err)
	}
	LogConversationIncomingToolMessages(log, b)
	out := buf.String()
	if !strings.Contains(out, "conversation.tool.call_started") {
		t.Fatalf("missing started: %s", out)
	}
	if !strings.Contains(out, "conversation.tool.call_completed") {
		t.Fatalf("missing completed: %s", out)
	}
	if !strings.Contains(out, "conversation.tool.call_failed") {
		t.Fatalf("missing failed: %s", out)
	}
	if !strings.Contains(out, "call_ok") || !strings.Contains(out, "call_bad") {
		t.Fatalf("expected tool_call_id in output: %s", out)
	}
}
