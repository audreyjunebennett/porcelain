package vectorstoreline

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/lynn/porcelain/internal/naming"
)

func TestNormalizePayloadVersionPlain(t *testing.T) {
	b := NormalizePayload("Version: 1.14.1, build: abc")
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m["msg"] != "vectorstore.version" {
		t.Fatalf("msg=%v", m["msg"])
	}
	if !strings.Contains(m["qdrant_version"].(string), "1.14.1") {
		t.Fatalf("version=%v", m["qdrant_version"])
	}
	if m["service"] != naming.ProductVectorstoreName {
		t.Fatalf("service=%v", m["service"])
	}
}

func TestNormalizePayloadLoadingCollection(t *testing.T) {
	raw := `{"timestamp":"t","level":"INFO","fields":{"message":"Loading collection: chimera-default-x"},"target":"storage::content_manager::toc"}`
	b := NormalizePayload(raw)
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m["msg"] != "vectorstore.collection.loading" {
		t.Fatal(m["msg"])
	}
	if m["collection"] != "chimera-default-x" {
		t.Fatal(m["collection"])
	}
	if m["service"] != naming.ProductVectorstoreName {
		t.Fatal(m["service"])
	}
}

func TestNormalizePayloadHTTPReadinessProbeDebug(t *testing.T) {
	raw := `{"timestamp":"t","level":"INFO","fields":{"message":"127.0.0.1 \"GET /collections HTTP/1.1\" 200 42 \"-\" \"Go-http-client/1.1\" 0.001"},"target":"actix_web::middleware::logger"}`
	b := NormalizePayload(raw)
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m["msg"] != "vectorstore.http.access_other" {
		t.Fatalf("msg=%v", m["msg"])
	}
	if m["level"] != "DEBUG" {
		t.Fatalf("level=%v want DEBUG", m["level"])
	}
	if int(m["http_status"].(float64)) != 200 {
		t.Fatal(m["http_status"])
	}
}

func TestNormalizePayloadHTTPReadinessProbeFailureStaysInfo(t *testing.T) {
	raw := `{"timestamp":"t","level":"INFO","fields":{"message":"127.0.0.1 \"GET /collections HTTP/1.1\" 503 12 \"-\" \"Go-http-client/1.1\" 0.001"},"target":"actix_web::middleware::logger"}`
	b := NormalizePayload(raw)
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m["level"] != "INFO" {
		t.Fatalf("level=%v want INFO", m["level"])
	}
}

func TestNormalizePayloadHTTPUpsertOK(t *testing.T) {
	raw := `{"timestamp":"t","level":"INFO","fields":{"message":"127.0.0.1 \"PUT /collections/coll-a/points?wait=true HTTP/1.1\" 200 92 \"-\" \"Go-http-client/1.1\" 0.001"},"target":"actix_web::middleware::logger"}`
	b := NormalizePayload(raw)
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m["msg"] != "vectorstore.http.points_upsert_ok" {
		t.Fatal(m["msg"])
	}
	if m["collection"] != "coll-a" {
		t.Fatal(m["collection"])
	}
	if int(m["http_status"].(float64)) != 200 {
		t.Fatal(m["http_status"])
	}
	if m["level"] != "INFO" {
		t.Fatalf("level=%v want INFO", m["level"])
	}
}

func TestNormalizePayloadHTTPCollectionMetaDemotedViaWriter(t *testing.T) {
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

func TestNormalizePayloadHTTPCollectionMetaNotFoundDemotedViaWriter(t *testing.T) {
	raw := `{"timestamp":"t","level":"INFO","fields":{"message":"127.0.0.1 \"GET /collections/coll-a HTTP/1.1\" 404 42 \"-\" \"Go-http-client/1.1\" 0.001"},"target":"actix_web::middleware::logger"}`
	b := postProcessNormalizedLine(NormalizePayload(raw))
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m["level"] != "DEBUG" {
		t.Fatalf("level=%v want DEBUG for 404 probe", m["level"])
	}
}

func TestNormalizePayloadHTTPCollectionCreate(t *testing.T) {
	raw := `{"timestamp":"t","level":"INFO","fields":{"message":"127.0.0.1 \"PUT /collections/coll-a HTTP/1.1\" 200 92 \"-\" \"Go-http-client/1.1\" 0.001"},"target":"actix_web::middleware::logger"}`
	b := NormalizePayload(raw)
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m["msg"] != "vectorstore.http.collection_create" {
		t.Fatalf("msg=%v", m["msg"])
	}
	if m["collection"] != "coll-a" {
		t.Fatal(m["collection"])
	}
	if m["level"] != "INFO" {
		t.Fatalf("level=%v want INFO", m["level"])
	}
}

func TestNormalizePayloadHTTPCollectionCreateConflict(t *testing.T) {
	raw := `{"timestamp":"t","level":"INFO","fields":{"message":"127.0.0.1 \"PUT /collections/coll-a HTTP/1.1\" 409 92 \"-\" \"Go-http-client/1.1\" 0.001"},"target":"actix_web::middleware::logger"}`
	b := NormalizePayload(raw)
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m["msg"] != "vectorstore.http.collection_create_rejected" {
		t.Fatalf("msg=%v", m["msg"])
	}
	if m["level"] != "INFO" {
		t.Fatalf("level=%v want INFO", m["level"])
	}
}

func TestNormalizePayloadHTTPCollectionIndex(t *testing.T) {
	raw := `{"timestamp":"t","level":"INFO","fields":{"message":"127.0.0.1 \"PUT /collections/coll-a/index HTTP/1.1\" 200 92 \"-\" \"Go-http-client/1.1\" 0.001"},"target":"actix_web::middleware::logger"}`
	b := NormalizePayload(raw)
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m["msg"] != "vectorstore.http.collection_index" {
		t.Fatalf("msg=%v", m["msg"])
	}
}

func TestNormalizePayloadCreatingCollection(t *testing.T) {
	raw := `{"timestamp":"t","level":"INFO","fields":{"message":"Creating collection chimera-default-x"},"target":"storage::content_manager::toc::collection_meta_ops"}`
	b := NormalizePayload(raw)
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m["msg"] != "vectorstore.collection.creating" {
		t.Fatal(m["msg"])
	}
	if m["collection"] != "chimera-default-x" {
		t.Fatal(m["collection"])
	}
}

func TestNormalizePayloadIdempotent(t *testing.T) {
	raw := `{"timestamp":"t","level":"INFO","service":"` + naming.ProductVectorstoreName + `","msg":"vectorstore.version","_chimera_norm":1}`
	b2 := NormalizePayload(raw)
	if string(b2) != raw {
		t.Fatalf("second pass changed output: %s vs %s", b2, raw)
	}
}

func TestNormalizePayloadPlainBanner(t *testing.T) {
	resetVectorstoreStartupChatterForTest()
	b := postProcessNormalizedLine(NormalizePayload("   ____  Qdrant banner line"))
	assertNormalizedFieldOrder(t, b)
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m["msg"] != "vectorstore.startup.banner" {
		t.Fatalf("msg=%v", m["msg"])
	}
	if m["level"] != "INFO" {
		t.Fatalf("first banner level=%v want INFO", m["level"])
	}
	if pd, ok := m["progress_detail"]; ok && pd != nil && pd != "" {
		t.Fatalf("first banner should omit decorative detail: %v", m)
	}
	if _, ok := m["timestamp"]; !ok {
		t.Fatalf("missing timestamp: %v", m)
	}

	b2 := postProcessNormalizedLine(NormalizePayload("   ║ more qdrant art ║"))
	var m2 map[string]any
	if err := json.Unmarshal(b2, &m2); err != nil {
		t.Fatal(err)
	}
	if m2["level"] != "DEBUG" {
		t.Fatalf("second banner level=%v want DEBUG", m2["level"])
	}
}

func TestNormalizePayloadStartupChatterDemotesRoutine(t *testing.T) {
	resetVectorstoreStartupChatterForTest()
	raw := `{"timestamp":"t","level":"INFO","fields":{"message":"Actix runtime found; 8 workers"},"target":"actix_server::builder"}`
	b := postProcessNormalizedLine(NormalizePayload(raw))
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m["msg"] != "vectorstore.actix.workers" {
		t.Fatalf("msg=%v", m["msg"])
	}
	if m["level"] != "DEBUG" {
		t.Fatalf("level=%v want DEBUG", m["level"])
	}

	rawRecover := `{"timestamp":"t","level":"INFO","fields":{"message":"Recovering shard ./storage/collections/foo/0: 0/1 (0%)"},"target":"collection::shards::local_shard"}`
	bRecover := postProcessNormalizedLine(NormalizePayload(rawRecover))
	var mRecover map[string]any
	if err := json.Unmarshal(bRecover, &mRecover); err != nil {
		t.Fatal(err)
	}
	if mRecover["level"] != "DEBUG" {
		t.Fatalf("zero progress recover level=%v want DEBUG", mRecover["level"])
	}

	rawDone := `{"timestamp":"t","level":"INFO","fields":{"message":"Recovered collection foo: 1/1 (100%)"},"target":"collection::shards::local_shard"}`
	bDone := postProcessNormalizedLine(NormalizePayload(rawDone))
	var mDone map[string]any
	if err := json.Unmarshal(bDone, &mDone); err != nil {
		t.Fatal(err)
	}
	if mDone["msg"] != "vectorstore.shard.recovered" {
		t.Fatalf("msg=%v", mDone["msg"])
	}
	if mDone["level"] != "INFO" {
		t.Fatalf("recovered level=%v want INFO", mDone["level"])
	}
}

func TestNormalizePayloadTraceOtherDetail(t *testing.T) {
	raw := `{"timestamp":"2026-05-19T02:19:38Z","level":"INFO","target":"foo","fields":{"message":"shard init ok"}}`
	b := NormalizePayload(raw)
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m["msg"] != "vectorstore.trace.other" {
		t.Fatalf("msg=%v", m["msg"])
	}
	if m["progress_detail"] != "shard init ok" {
		t.Fatalf("progress_detail=%v", m["progress_detail"])
	}
}

func TestNormalizePayloadVectorstoreUpstreamLine(t *testing.T) {
	raw := `{"time":"t","level":"INFO","msg":"vectorstore.upstream.line","upstream_raw":"plain upstream"}`
	b := NormalizePayload(raw)
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m["progress_detail"] != "plain upstream" {
		t.Fatalf("progress_detail=%v", m)
	}
}

func TestNormalizePayloadSupervisorSecondPass(t *testing.T) {
	raw := `{"timestamp":"t","level":"INFO","fields":{"message":"unclassified line"},"target":"qdrant::foo"}`
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
	if m["msg"] != "vectorstore.trace.other" {
		t.Fatalf("msg=%v", m["msg"])
	}
	if m["progress_detail"] != "unclassified line" {
		t.Fatalf("progress_detail=%v", m["progress_detail"])
	}
}
