package gatewayline

import (
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
