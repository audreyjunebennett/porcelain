package vectorstoreline

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

func assertNormalizedFieldOrder(t *testing.T, b []byte) {
	t.Helper()
	s := string(b)
	if !strings.HasPrefix(s, `{"timestamp":`) {
		t.Fatalf("expected timestamp-first JSON, got %s", s)
	}
	if strings.HasPrefix(s, `{"_chimera_norm":`) {
		t.Fatalf("expected _chimera_norm last, got %s", s)
	}
}

func TestPostProcessDemotesSuccessfulUpsert(t *testing.T) {
	raw := `{"timestamp":"t","level":"INFO","fields":{"message":"127.0.0.1 \"PUT /collections/coll-a/points?wait=true HTTP/1.1\" 200 92 \"-\" \"Go-http-client/1.1\" 0.001"},"target":"actix_web::middleware::logger"}`
	b := postProcessNormalizedLine(NormalizePayload(raw))
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m["msg"] != "vectorstore.http.points_upsert_ok" {
		t.Fatalf("msg=%v", m["msg"])
	}
	if m["level"] != "DEBUG" {
		t.Fatalf("level=%v want DEBUG", m["level"])
	}
}

func TestPostProcessDemotesCollectionMeta(t *testing.T) {
	raw := `{"timestamp":"t","level":"INFO","fields":{"message":"127.0.0.1 \"GET /collections/coll-a HTTP/1.1\" 200 42 \"-\" \"Go-http-client/1.1\" 0.001"},"target":"actix_web::middleware::logger"}`
	b := postProcessNormalizedLine(NormalizePayload(raw))
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m["msg"] != "vectorstore.http.collection_meta" {
		t.Fatalf("msg=%v", m["msg"])
	}
	if m["level"] != "DEBUG" {
		t.Fatalf("level=%v want DEBUG", m["level"])
	}
}

func TestPostProcessKeepsRejectedUpsertAtInfo(t *testing.T) {
	raw := `{"timestamp":"t","level":"INFO","fields":{"message":"127.0.0.1 \"PUT /collections/coll-a/points?wait=true HTTP/1.1\" 400 92 \"-\" \"Go-http-client/1.1\" 0.001"},"target":"actix_web::middleware::logger"}`
	b := postProcessNormalizedLine(NormalizePayload(raw))
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m["msg"] != "vectorstore.http.points_upsert_rejected" {
		t.Fatalf("msg=%v", m["msg"])
	}
	if m["level"] != "INFO" {
		t.Fatalf("level=%v want INFO", m["level"])
	}
}

func TestHTTPSummaryEmitsAfterWindow(t *testing.T) {
	tracker := &httpSummaryTracker{
		window:      &httpSummaryWindow{collections: map[string]struct{}{}, windowStart: time.Now().UTC().Add(-6 * time.Second)},
		minInterval: 5 * time.Second,
	}
	var buf bytes.Buffer
	tracker.dsts = []ioWriterRef{{w: &buf}}

	tracker.note("coll-a", "vectorstore.http.points_upsert_ok", 200)
	tracker.note("coll-a", "vectorstore.http.points_upsert_ok", 200)
	tracker.note("coll-b", "vectorstore.http.vector_search", 200)

	tracker.emitDue(true)

	out := buf.String()
	if !strings.Contains(out, "vectorstore.http.upsert.summary") {
		t.Fatalf("missing summary line: %q", out)
	}
	assertNormalizedFieldOrder(t, []byte(strings.TrimSpace(out)))
	var m map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &m); err != nil {
		t.Fatal(err)
	}
	if int(m["upserts_ok"].(float64)) != 2 {
		t.Fatalf("upserts_ok=%v", m["upserts_ok"])
	}
	if int(m["searches_ok"].(float64)) != 1 {
		t.Fatalf("searches_ok=%v", m["searches_ok"])
	}
	if m["collections"] != "coll-a,coll-b" {
		t.Fatalf("collections=%v", m["collections"])
	}
}

func TestDualWriterSameCaptureEmitsOneSummary(t *testing.T) {
	httpSummaryOnce = sync.Once{}
	httpSummary = nil

	var capture bytes.Buffer
	RegisterHTTPSummaryDestination(&capture)
	stdoutW := NewWriter(io.MultiWriter(&capture, io.Discard))
	stderrW := NewWriter(io.MultiWriter(&capture, io.Discard))

	upsert := `{"timestamp":"t","level":"INFO","fields":{"message":"127.0.0.1 \"PUT /collections/coll-a/points?wait=true HTTP/1.1\" 200 92 \"-\" \"Go-http-client/1.1\" 0.001"},"target":"actix_web::middleware::logger"}` + "\n"
	if _, err := stdoutW.Write([]byte(upsert)); err != nil {
		t.Fatal(err)
	}
	if _, err := stderrW.Write([]byte(upsert)); err != nil {
		t.Fatal(err)
	}
	globalHTTPSummary().emitDue(true)

	lines := strings.Split(strings.TrimSpace(capture.String()), "\n")
	var summaries int
	for _, line := range lines {
		if strings.Contains(line, "vectorstore.http.upsert.summary") {
			summaries++
		}
	}
	if summaries != 1 {
		t.Fatalf("want 1 summary line in capture, got %d: %q", summaries, capture.String())
	}
}

func TestWriterEmitsSummaryLine(t *testing.T) {
	httpSummaryOnce = sync.Once{}
	httpSummary = nil

	var buf bytes.Buffer
	RegisterHTTPSummaryDestination(&buf)
	w := NewWriter(&buf)

	upsert := `{"timestamp":"t","level":"INFO","fields":{"message":"127.0.0.1 \"PUT /collections/coll-a/points?wait=true HTTP/1.1\" 200 92 \"-\" \"Go-http-client/1.1\" 0.001"},"target":"actix_web::middleware::logger"}` + "\n"
	if _, err := w.Write([]byte(upsert)); err != nil {
		t.Fatal(err)
	}

	globalHTTPSummary().emitDue(true)

	all := buf.String()
	if !strings.Contains(all, "vectorstore.http.points_upsert_ok") {
		t.Fatalf("missing upsert line: %q", all)
	}
	if !strings.Contains(all, "vectorstore.http.upsert.summary") {
		t.Fatalf("missing summary line: %q", all)
	}
}
