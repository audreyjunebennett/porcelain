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
}

func TestNormalizePayloadIdempotent(t *testing.T) {
	raw := `{"timestamp":"t","level":"INFO","service":"` + naming.ProductVectorstoreName + `","msg":"vectorstore.version","_chimera_norm":1}`
	b2 := NormalizePayload(raw)
	if string(b2) != raw {
		t.Fatalf("second pass changed output: %s vs %s", b2, raw)
	}
}

func TestNormalizePayloadPlainBanner(t *testing.T) {
	b := NormalizePayload("   ____  Qdrant banner line")
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m["msg"] != "vectorstore.startup.banner" {
		t.Fatalf("msg=%v", m["msg"])
	}
	if m["progress_detail"] != "____  Qdrant banner line" {
		t.Fatalf("progress_detail=%q", m["progress_detail"])
	}
	if _, ok := m["timestamp"]; !ok {
		t.Fatalf("missing timestamp: %v", m)
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
