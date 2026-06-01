package brokerline

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/lynn/porcelain/internal/naming"
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

func TestNormalizePayloadBrokerUpstreamLine(t *testing.T) {
	raw := `{"level":"INFO","time":"2026-05-08T14:00:00-05:00","msg":"broker.upstream.line","upstream_raw":"Version: 1.14.1"}`
	b := NormalizePayload(raw)
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m["msg"] != "broker.upstream.line" {
		t.Fatalf("msg=%v", m["msg"])
	}
	if m["progress_detail"] != "Version: 1.14.1" {
		t.Fatalf("progress_detail=%v", m["progress_detail"])
	}
	if m["msg"] == m["progress_detail"] {
		t.Fatal("progress_detail must not echo slug")
	}
}

func TestNormalizePayloadPlainBannerTimestamp(t *testing.T) {
	resetBrokerStartupChatterForTest()
	b := NormalizePayload("╔════ BiFrost ════╗")
	assertNormalizedFieldOrder(t, b)
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
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

	b2 := NormalizePayload("║  more ascii art ║")
	var m2 map[string]any
	if err := json.Unmarshal(b2, &m2); err != nil {
		t.Fatal(err)
	}
	if m2["level"] != "DEBUG" {
		t.Fatalf("second banner level=%v want DEBUG", m2["level"])
	}
}

func TestNormalizePayloadStartupChatterDemotesRoutine(t *testing.T) {
	resetBrokerStartupChatterForTest()
	cases := []struct {
		raw       string
		wantLevel string
	}{
		{`{"level":"info","time":"t","message":"config store initialized (sqlite)"}`, "DEBUG"},
		{`{"level":"info","time":"t","message":"Token refresh worker started"}`, "DEBUG"},
		{`{"level":"info","time":"t","message":"successfully started chimera-broker, serving UI on http://127.0.0.1:8080"}`, "INFO"},
		{`{"level":"info","time":"t","message":"42 models added to catalog"}`, "INFO"},
		{`{"level":"info","time":"t","message":"initializing model catalog"}`, "DEBUG"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.wantLevel, func(t *testing.T) {
			b := NormalizePayload(tc.raw)
			var m map[string]any
			if err := json.Unmarshal(b, &m); err != nil {
				t.Fatal(err)
			}
			if m["level"] != tc.wantLevel {
				t.Fatalf("level=%v want %s msg=%v", m["level"], tc.wantLevel, m["msg"])
			}
		})
	}
}

func TestNormalizePayloadSchemaWarnBoxDemoted(t *testing.T) {
	resetBrokerStartupChatterForTest()
	box := NormalizePayload("╔══ schema warning box ══╗")
	var boxM map[string]any
	if err := json.Unmarshal(box, &boxM); err != nil {
		t.Fatal(err)
	}
	if boxM["msg"] != "broker.config.schema_warn" {
		t.Fatalf("msg=%v", boxM["msg"])
	}
	if boxM["level"] != "DEBUG" {
		t.Fatalf("box level=%v want DEBUG", boxM["level"])
	}

	plain := NormalizePayload(`{"level":"warn","time":"t","message":"config file does not include all schema fields"}`)
	var plainM map[string]any
	if err := json.Unmarshal(plain, &plainM); err != nil {
		t.Fatal(err)
	}
	if plainM["level"] != "WARN" {
		t.Fatalf("plain warn level=%v want WARN", plainM["level"])
	}
}

func TestNormalizePayloadProviderHealthAndDiscovery(t *testing.T) {
	cases := []struct {
		raw     string
		wantMsg string
		wantPID string
	}{
		{
			raw:     `{"level":"warn","time":"t","message":"Model discovery failed for provider ollama"}`,
			wantMsg: "broker.provider.model_discovery.fail",
			wantPID: "ollama",
		},
		{
			raw:     `{"level":"warn","time":"t","message":"failed to list models for provider ollama: network error occurred while connecting to provider API (DNS lookup, connection refused, etc.)"}`,
			wantMsg: "broker.provider.model_discovery.fail",
			wantPID: "ollama",
		},
		{
			raw:     `{"level":"info","time":"t","message":"key loaded for provider groq"}`,
			wantMsg: "broker.provider.key_loaded",
			wantPID: "groq",
		},
		{
			raw:     `{"level":"warn","time":"t","message":"no API key for provider gemini"}`,
			wantMsg: "broker.provider.key_missing",
			wantPID: "gemini",
		},
		{
			raw:     `{"level":"info","time":"t","message":"provider ollama health check passed"}`,
			wantMsg: "broker.provider.health.ok",
			wantPID: "ollama",
		},
		{
			raw:     `{"level":"warn","time":"t","message":"health check failed for provider openai"}`,
			wantMsg: "broker.provider.health.fail",
			wantPID: "openai",
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.wantMsg, func(t *testing.T) {
			t.Parallel()
			b := NormalizePayload(tc.raw)
			var m map[string]any
			if err := json.Unmarshal(b, &m); err != nil {
				t.Fatal(err)
			}
			if m["msg"] != tc.wantMsg {
				t.Fatalf("msg=%v want %s", m["msg"], tc.wantMsg)
			}
			if m["provider_id"] != tc.wantPID {
				t.Fatalf("provider_id=%v want %s", m["provider_id"], tc.wantPID)
			}
		})
	}
}

func TestNormalizePayloadHTTPAccessProviderProbe(t *testing.T) {
	raw := `{"level":"info","http.method":"GET","http.target":"/api/providers/ollama","http.status_code":200,"http.request_duration_ms":4,"time":"t","message":"request completed"}`
	b := NormalizePayload(raw)
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m["provider_id"] != "ollama" {
		t.Fatalf("provider_id=%v", m["provider_id"])
	}
	if m["progress_detail"] != "gateway admin · provider health probe · ollama" {
		t.Fatalf("progress_detail=%v", m["progress_detail"])
	}
	if m["level"] != "DEBUG" {
		t.Fatalf("level=%v want DEBUG for successful admin probe", m["level"])
	}

	rawGov := `{"level":"info","http.method":"GET","http.target":"/api/governance/providers","http.status_code":200,"time":"t","message":"request completed"}`
	bGov := NormalizePayload(rawGov)
	var mGov map[string]any
	if err := json.Unmarshal(bGov, &mGov); err != nil {
		t.Fatal(err)
	}
	if mGov["progress_detail"] != "gateway admin · configured provider roster" {
		t.Fatalf("progress_detail=%v", mGov["progress_detail"])
	}
	if mGov["level"] != "DEBUG" {
		t.Fatalf("level=%v want DEBUG for successful governance list", mGov["level"])
	}

	rawFail := `{"level":"info","http.method":"GET","http.target":"/api/providers/gemini","http.status_code":503,"time":"t","message":"request completed"}`
	bFail := NormalizePayload(rawFail)
	var mFail map[string]any
	if err := json.Unmarshal(bFail, &mFail); err != nil {
		t.Fatal(err)
	}
	if mFail["level"] == "DEBUG" {
		t.Fatalf("failed admin probe should not be DEBUG, level=%v", mFail["level"])
	}
}

func TestNormalizePayloadSupervisorSecondPass(t *testing.T) {
	raw := `{"level":"info","http.method":"POST","http.target":"/v1/chat","http.status_code":200,"time":"t","message":"request completed"}`
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
	if m["msg"] != "broker.http.access" {
		t.Fatalf("msg=%v", m["msg"])
	}
	if int(m["http_status"].(float64)) != 200 {
		t.Fatalf("http_status=%v", m["http_status"])
	}
}

func TestNormalizePayloadHTTPAccessModelsPollDebug(t *testing.T) {
	raw := `{"level":"info","http.method":"GET","http.target":"/v1/models","http.status_code":200,"time":"t","message":"request completed"}`
	b := NormalizePayload(raw)
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m["msg"] != "broker.http.access" {
		t.Fatalf("msg=%v", m["msg"])
	}
	if m["level"] != "DEBUG" {
		t.Fatalf("level=%v want DEBUG", m["level"])
	}
}

func TestNormalizePayloadHTTPAccessModelsPollDebugFullURL(t *testing.T) {
	raw := `{"level":"info","http.method":"GET","http.target":"http://127.0.0.1:8080/v1/models","http.status_code":200,"time":"t","message":"request completed"}`
	b := NormalizePayload(raw)
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m["level"] != "DEBUG" {
		t.Fatalf("level=%v want DEBUG", m["level"])
	}
}
