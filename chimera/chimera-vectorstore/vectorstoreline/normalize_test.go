package vectorstoreline

import (
	"encoding/json"
	"strings"
	"testing"
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
	if m["service"] != "chimera-vectorstore" {
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
	if m["service"] != "chimera-vectorstore" {
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
	raw := `{"timestamp":"t","level":"INFO","service":"chimera-vectorstore","msg":"vectorstore.version","_chimera_norm":1}`
	b2 := NormalizePayload(raw)
	if string(b2) != raw {
		t.Fatalf("second pass changed output: %s vs %s", b2, raw)
	}
}
