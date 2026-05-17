package line

import (
	"encoding/json"
	"strings"
	"testing"
)

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
