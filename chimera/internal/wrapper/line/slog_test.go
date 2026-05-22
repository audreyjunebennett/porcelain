package line

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNormalizeSlogLinePreservesShutdownAttrs(t *testing.T) {
	raw := []byte(`{"time":"2026-05-21T22:33:52Z","level":"WARN","msg":"chimera-supervisor.shutdown.child_force_kill","child":"gateway","pid":1234,"timeout":"15s"}`)
	b, ok := NormalizeSlogLine(raw, "chimera-supervisor")
	if !ok {
		t.Fatal("expected normalize")
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m["child"] != "gateway" {
		t.Fatalf("child=%v", m["child"])
	}
	if m["pid"] != "1234" {
		t.Fatalf("pid=%v", m["pid"])
	}
	if m["timeout"] != "15s" {
		t.Fatalf("timeout=%v", m["timeout"])
	}
	if m["service"] != "chimera-supervisor" {
		t.Fatalf("service=%v", m["service"])
	}
}

func TestPassthroughSlogJSONKeyOrder(t *testing.T) {
	raw := []byte(`{"time":"2026-05-16T12:00:00Z","level":"INFO","msg":"wrapper.backend.starting","component":"chimera-broker"}`)
	b, ok := PassthroughSlogJSON(raw, "broker")
	if !ok {
		t.Fatal("expected passthrough")
	}
	s := string(b)
	idxTS := strings.Index(s, `"timestamp"`)
	idxLevel := strings.Index(s, `"level"`)
	idxSvc := strings.Index(s, `"service"`)
	idxMsg := strings.Index(s, `"msg"`)
	idxNorm := strings.Index(s, `"_chimera_norm"`)
	if !(idxTS < idxLevel && idxLevel < idxSvc && idxSvc < idxMsg && idxMsg < idxNorm) {
		t.Fatalf("key order wrong: %s", s)
	}
}

func TestPassthroughSlogJSONNormalizesTimestamp(t *testing.T) {
	raw := []byte(`{"time":"2026-05-22T11:38:55.5597571-05:00","level":"INFO","msg":"wrapper.backend.starting","component":"chimera-broker"}`)
	b, ok := PassthroughSlogJSON(raw, "broker")
	if !ok {
		t.Fatal("expected passthrough")
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m["timestamp"] != "2026-05-22T16:38:55Z" {
		t.Fatalf("timestamp=%v", m["timestamp"])
	}
}

func TestPassthroughSlogJSONPreservesNormalized(t *testing.T) {
	raw := []byte(`{"_chimera_norm":1,"child":"broker","level":"WARN","msg":"gateway.shutdown.child_force_kill","service":"gateway","timestamp":"2026-05-16T00:45:21Z"}`)
	b, ok := PassthroughSlogJSON(raw, "gateway")
	if !ok {
		t.Fatal("expected passthrough")
	}
	idxTS := strings.Index(string(b), `"timestamp"`)
	idxNorm := strings.Index(string(b), `"_chimera_norm"`)
	if idxTS > idxNorm {
		t.Fatalf("expected _chimera_norm last, got %s", b)
	}
}

func TestPassthroughSlogJSONSkipsNonSlog(t *testing.T) {
	_, ok := PassthroughSlogJSON([]byte(`{"foo":"bar"}`), "broker")
	if ok {
		t.Fatal("expected no passthrough")
	}
}

func TestPassthroughSlogJSONSkipsGatewayConversationSlug(t *testing.T) {
	raw := []byte(`{"time":"2026-05-09T12:00:00Z","level":"INFO","msg":"conversation.received","conversation_id":"c1"}`)
	_, ok := PassthroughSlogJSON(raw, "chimera-gateway")
	if ok {
		t.Fatal("conversation.* must not use PassthroughSlogJSON (drops correlation attrs)")
	}
}

func TestReorderNormalizedJSON(t *testing.T) {
	raw := []byte(`{"_chimera_norm":1,"msg":"broker.ready","service":"broker","level":"INFO","timestamp":"2026-05-16T12:00:00Z"}`)
	b, ok := ReorderNormalizedJSON(raw)
	if !ok {
		t.Fatal("expected reorder")
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m["msg"] != "broker.ready" {
		t.Fatalf("msg=%v", m["msg"])
	}
}

func TestReorderNormalizedJSONNormalizesTimestamp(t *testing.T) {
	raw := []byte(`{"_chimera_norm":1,"msg":"wrapper.backend.starting","service":"chimera-broker","level":"INFO","timestamp":"2026-05-22T11:38:55.5597571-05:00"}`)
	b, ok := ReorderNormalizedJSON(raw)
	if !ok {
		t.Fatal("expected reorder")
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m["timestamp"] != "2026-05-22T16:38:55Z" {
		t.Fatalf("timestamp=%v", m["timestamp"])
	}
}
