package indexerline

import (
	"bytes"
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
	if m["recovery"] != false {
		t.Fatalf("recovery=%v", m["recovery"])
	}
}

func TestNormalizePayloadJobIngested(t *testing.T) {
	raw := `{"time":"2026-05-18T12:00:00Z","level":"INFO","msg":"indexer.job.ingested","service":"indexer","rel":"src/main.go","chunks":8,"collection":"chimera-tenant-project-abc","mode":"whole"}`
	b := NormalizePayload(raw)
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m["msg"] != "indexer.job.ingested" {
		t.Fatalf("msg=%v", m["msg"])
	}
	if m["rel"] != "src/main.go" {
		t.Fatalf("rel=%v", m["rel"])
	}
	if int(m["chunks"].(float64)) != 8 {
		t.Fatalf("chunks=%v", m["chunks"])
	}
	if m["collection"] != "chimera-tenant-project-abc" {
		t.Fatalf("collection=%v", m["collection"])
	}
}

func TestNormalizePayloadTruncatesTimestamp(t *testing.T) {
	raw := `{"time":"2026-05-22T11:38:58.1267507-05:00","level":"INFO","msg":"indexer.run.start","service":"indexer","roots":1}`
	b := NormalizePayload(raw)
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m["timestamp"] != "2026-05-22T16:38:58Z" {
		t.Fatalf("timestamp=%v", m["timestamp"])
	}
}

func TestNormalizePayloadQueueSnapshot(t *testing.T) {
	raw := `{"level":"INFO","msg":"indexer.queue.snapshot","queue_depth":3,"queue_cap":64,"workers":4,"ingest_completed":12,"phase":"idle"}`
	b := NormalizePayload(raw)
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m["msg"] != "indexer.queue.snapshot" {
		t.Fatalf("msg=%v", m["msg"])
	}
	if int(m["queue_depth"].(float64)) != 3 {
		t.Fatalf("queue_depth=%v", m["queue_depth"])
	}
	if int(m["queue_cap"].(float64)) != 64 {
		t.Fatalf("queue_cap=%v", m["queue_cap"])
	}
	if int(m["ingest_completed"].(float64)) != 12 {
		t.Fatalf("ingest_completed=%v", m["ingest_completed"])
	}
}

func TestNormalizePayloadFanoutEnqueueFailed(t *testing.T) {
	raw := `{"level":"ERROR","msg":"indexer.fanout.enqueue_failed","candidates":40,"service":"indexer"}`
	b := NormalizePayload(raw)
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m["msg"] != "indexer.fanout.enqueue_failed" {
		t.Fatalf("msg=%v", m["msg"])
	}
	if int(m["candidates"].(float64)) != 40 {
		t.Fatalf("candidates=%v", m["candidates"])
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
	if m["progress_detail"] != "indexer startup banner" {
		t.Fatalf("progress_detail=%v", m["progress_detail"])
	}
}

func TestNormalizePayloadIdempotent(t *testing.T) {
	raw := string(NormalizePayload(`{"msg":"indexer.state","service":"indexer","state":"recovery","recovery":true}`))
	b2 := NormalizePayload(raw)
	if string(b2) != raw {
		t.Fatalf("second pass changed output: %s vs %s", b2, raw)
	}
}

func TestNormalizePayloadSupervisorSecondPass(t *testing.T) {
	raw := `{"time":"t","level":"INFO","msg":"indexer.job.ingested","rel":"pkg/foo.go","chunks":3,"collection":"coll-a"}`
	first := NormalizePayload(raw)
	var buf bytes.Buffer
	w := NewWriter(&buf)
	if _, err := w.Write(append(first, '\n')); err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &m); err != nil {
		t.Fatal(err)
	}
	if m["rel"] != "pkg/foo.go" {
		t.Fatalf("rel=%v", m)
	}
	if int(m["chunks"].(float64)) != 3 {
		t.Fatalf("chunks=%v", m)
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
