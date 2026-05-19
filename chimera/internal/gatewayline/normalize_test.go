package gatewayline

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestNormalizePayloadGatewayAccess(t *testing.T) {
	raw := `{"time":"2026-05-14T12:34:56Z","level":"INFO","msg":"gateway.http.access","method":"GET","path":"/health","statusCode":200,"responseTimeMs":12,"timeline_kind":"web","service":"gateway"}`
	b := NormalizePayload(raw)
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m["msg"] != "gateway.http.access" {
		t.Fatalf("msg=%v", m["msg"])
	}
	if m["service"] != "chimera-gateway" {
		t.Fatalf("service=%v", m["service"])
	}
	if m["method"] != "GET" || m["path"] != "/health" {
		t.Fatalf("method/path=%v/%v", m["method"], m["path"])
	}
	if int(m["statusCode"].(float64)) != 200 {
		t.Fatalf("status=%v", m["statusCode"])
	}
}

func TestNormalizePayloadPlainLine(t *testing.T) {
	raw := `gateway startup seed`
	out := string(NormalizePayload(raw))
	if !strings.Contains(out, `"service":"chimera-gateway"`) {
		t.Fatalf("missing gateway service: %s", out)
	}
	if !strings.Contains(out, `"msg":"gateway.log.text"`) {
		t.Fatalf("missing gateway text msg: %s", out)
	}
}

func TestNormalizePayloadIdempotent(t *testing.T) {
	raw := string(NormalizePayload(`{"msg":"gateway.startup.seed","service":"gateway","_chimera_norm":1}`))
	got := string(NormalizePayload(raw))
	if got != raw {
		t.Fatalf("expected idempotent normalize, got %s want %s", got, raw)
	}
}

// TestNormalizePayloadSupervisorSecondPass simulates chimera-supervisor LogSink re-normalizing
// wrapper stdout that was already normalized once.
func TestNormalizePayloadUpstreamLineDetail(t *testing.T) {
	raw := `{"time":"2026-05-14T12:00:00Z","level":"INFO","msg":"gateway.upstream.line","upstream_raw":"{\"level\":\"INFO\",\"msg\":\"nested\"}"}`
	b := NormalizePayload(raw)
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m["progress_detail"] == nil || m["progress_detail"] == "" {
		t.Fatalf("missing progress_detail: %v", m)
	}
	if !strings.Contains(m["progress_detail"].(string), "nested") {
		t.Fatalf("progress_detail=%v", m["progress_detail"])
	}
}

func TestNormalizePayloadPlainHasTimestamp(t *testing.T) {
	b := NormalizePayload("gateway startup banner")
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if _, ok := m["timestamp"]; !ok {
		t.Fatalf("missing timestamp: %v", m)
	}
}

func TestNormalizePayloadStartupListeningPreservesKV(t *testing.T) {
	raw := `{"time":"2026-05-14T12:00:00Z","level":"INFO","msg":"gateway.startup.listening","addr":":8080","broker":"http://127.0.0.1:8081","config":"/cfg/gateway.yaml","vectorstore_supervised":true,"indexer_supervised":false,"timeline_kind":"gateway"}`
	b := NormalizePayload(raw)
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m["addr"] != ":8080" {
		t.Fatalf("addr=%v", m["addr"])
	}
	if m["broker"] != "http://127.0.0.1:8081" {
		t.Fatalf("broker=%v", m["broker"])
	}
	if m["config"] != "/cfg/gateway.yaml" {
		t.Fatalf("config=%v", m["config"])
	}
	if m["timeline_kind"] != "gateway" {
		t.Fatalf("timeline_kind=%v", m["timeline_kind"])
	}
}

func TestNormalizePayloadSupervisorSecondPass(t *testing.T) {
	raw := `{"time":"2026-05-14T12:34:56Z","level":"INFO","msg":"gateway.http.access","method":"GET","path":"/health","statusCode":200,"responseTimeMs":12,"service":"gateway"}`
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
	if m["method"] != "GET" || m["path"] != "/health" {
		t.Fatalf("supervisor second pass stripped fields: %v", m)
	}
}
