package indexerline

import (
	"encoding/json"
	"testing"
)

func TestNormalizePayloadStateJSON(t *testing.T) {
	raw := `{"msg":"indexer.state","service":"indexer","state":"watch_idle","recovery":false}`
	b := NormalizePayload(raw)
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m["msg"] != "indexer.state" {
		t.Fatalf("msg=%v", m["msg"])
	}
	if m["service"] != "chimera-indexer" {
		t.Fatalf("service=%v", m["service"])
	}
	if m["state"] != "watch_idle" {
		t.Fatalf("state=%v", m["state"])
	}
}

func TestNormalizePayloadPlain(t *testing.T) {
	b := NormalizePayload("indexer startup banner")
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m["msg"] != "indexer.log.line" {
		t.Fatalf("msg=%v", m["msg"])
	}
	if m["service"] != "chimera-indexer" {
		t.Fatalf("service=%v", m["service"])
	}
}

func TestNormalizePayloadIdempotent(t *testing.T) {
	raw := string(NormalizePayload(`{"msg":"indexer.state","service":"indexer","state":"recovery","recovery":true}`))
	b2 := NormalizePayload(raw)
	if string(b2) != raw {
		t.Fatalf("second pass changed output: %s vs %s", b2, raw)
	}
}

func TestParseSupervisorHeartbeat(t *testing.T) {
	hb, ok := ParseSupervisorHeartbeat(`{"msg":"indexer.state","service":"indexer","state":"watch_idle","recovery":false}`)
	if !ok {
		t.Fatal("expected heartbeat parse")
	}
	if hb.DeclaredState != "watch_idle" {
		t.Fatalf("declared=%q", hb.DeclaredState)
	}
	if hb.WorkerState != "up" {
		t.Fatalf("worker=%q", hb.WorkerState)
	}
}

func TestParseSupervisorHeartbeatRecovery(t *testing.T) {
	hb, ok := ParseSupervisorHeartbeat(`{"msg":"indexer.state","service":"indexer","state":"recovery","recovery":true}`)
	if !ok {
		t.Fatal("expected heartbeat parse")
	}
	if hb.WorkerState != "degraded" {
		t.Fatalf("worker=%q", hb.WorkerState)
	}
}

func TestParseSupervisorHeartbeatRejectsOtherLines(t *testing.T) {
	if _, ok := ParseSupervisorHeartbeat(`{"msg":"indexer.job.ingested","service":"indexer"}`); ok {
		t.Fatal("expected non-heartbeat line to be rejected")
	}
}
