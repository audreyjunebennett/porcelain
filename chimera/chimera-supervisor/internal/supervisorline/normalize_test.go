package supervisorline

import (
	"encoding/json"
	"testing"
)

func TestNormalizePayloadPreservesShutdownChildFields(t *testing.T) {
	raw := `{"time":"2026-05-21T22:33:37.6486723Z","level":"INFO","msg":"chimera-supervisor.shutdown.child_signaling","child":"vectorstore","pid":4242}`
	b := NormalizePayload(raw)
	if len(b) == 0 {
		t.Fatal("expected normalized output")
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m["msg"] != "chimera-supervisor.shutdown.child_signaling" {
		t.Fatalf("msg=%v", m["msg"])
	}
	if m["child"] != "vectorstore" {
		t.Fatalf("child=%v", m["child"])
	}
	if m["pid"] != "4242" {
		t.Fatalf("pid=%v", m["pid"])
	}
	if m["service"] != "chimera-supervisor" {
		t.Fatalf("service=%v", m["service"])
	}
}

func TestNormalizePayloadPreservesForceKillFields(t *testing.T) {
	raw := `{"time":"2026-05-21T22:33:52.6489788Z","level":"WARN","msg":"chimera-supervisor.shutdown.child_force_kill","child":"broker","pid":5150,"timeout":"10s"}`
	b := NormalizePayload(raw)
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m["child"] != "broker" {
		t.Fatalf("child=%v", m["child"])
	}
	if m["timeout"] != "10s" {
		t.Fatalf("timeout=%v", m["timeout"])
	}
}

func TestNormalizePayloadPreservesChildExitFields(t *testing.T) {
	raw := `{"time":"2026-05-21T22:33:52.848589Z","level":"INFO","msg":"chimera-supervisor.child.exited","child":"indexer","forced":false}`
	b := NormalizePayload(raw)
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m["child"] != "indexer" {
		t.Fatalf("child=%v", m["child"])
	}
	if forced, ok := m["forced"].(bool); !ok || forced {
		t.Fatalf("forced=%v", m["forced"])
	}
}
