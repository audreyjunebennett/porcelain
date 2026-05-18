package brokerline

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/lynn/porcelain/internal/naming"
)

func TestNormalizePayloadHTTPAccessAndRateLimit(t *testing.T) {
	raw := `{"level":"info","http.method":"POST","http.target":"/v1/chat/completions","http.status_code":200,"http.request_duration_ms":348,"time":"2026-05-08T14:29:53-05:00","message":"request completed"}`
	b := NormalizePayload(raw)
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m["msg"] != "broker.http.access" {
		t.Fatalf("msg=%v", m["msg"])
	}
	if m["service"] != naming.ProductBrokerName {
		t.Fatalf("service=%v", m["service"])
	}
	if int(m["http_status"].(float64)) != 200 {
		t.Fatal(m["http_status"])
	}

	raw429 := `{"level":"warn","http.method":"POST","http.target":"/v1/chat/completions","http.status_code":429,"http.request_duration_ms":126,"time":"2026-05-08T14:30:06-05:00","message":"request completed"}`
	b429 := NormalizePayload(raw429)
	var m429 map[string]any
	if err := json.Unmarshal(b429, &m429); err != nil {
		t.Fatal(err)
	}
	if m429["msg"] != "broker.rate_limit" {
		t.Fatalf("msg=%v", m429["msg"])
	}
}

func TestNormalizePayloadReadyAndBootstrap(t *testing.T) {
	raw := `{"level":"info","time":"2026-05-08T14:15:51-05:00","message":"successfully started chimera-broker, serving UI on http://127.0.0.1:8080"}`
	b := NormalizePayload(raw)
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m["msg"] != "broker.ready" {
		t.Fatalf("msg=%v", m["msg"])
	}
	if int(m["listen_port"].(float64)) != 8080 {
		t.Fatal(m["listen_port"])
	}

	rawB := `{"level":"info","time":"t","message":"Time spent in chimera-broker server bootstrap 2232 ms"}`
	bb := NormalizePayload(rawB)
	var mb map[string]any
	if err := json.Unmarshal(bb, &mb); err != nil {
		t.Fatal(err)
	}
	if mb["msg"] != "broker.bootstrap.complete" {
		t.Fatal(mb["msg"])
	}
	if int(mb["bootstrap_ms"].(float64)) != 2232 {
		t.Fatal(mb["bootstrap_ms"])
	}
}

func TestNormalizePayloadIdempotent(t *testing.T) {
	raw := string(NormalizePayload(`{"level":"info","time":"t","message":"chimera-broker client initialized"}`))
	b2 := NormalizePayload(raw)
	if string(b2) != raw {
		t.Fatalf("second pass changed output: %s vs %s", b2, raw)
	}
	if !strings.Contains(raw, `"service":"`+naming.ProductBrokerName+`"`) {
		t.Fatalf("expected broker service in normalized output: %s", raw)
	}
}
