package qdrantline

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNormalizePayload_versionPlain(t *testing.T) {
	b := NormalizePayload("Version: 1.14.1, build: abc")
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m["msg"] != "qdrant.version" {
		t.Fatalf("msg=%v", m["msg"])
	}
	if !strings.Contains(m["qdrant_version"].(string), "1.14.1") {
		t.Fatalf("version=%v", m["qdrant_version"])
	}
}

func TestNormalizePayload_configMissing(t *testing.T) {
	raw := `{"timestamp":"2026-05-07T22:19:40.744448Z","level":"WARN","fields":{"message":"Config file not found: config/config"},"target":"qdrant::settings"}`
	b := NormalizePayload(raw)
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m["msg"] != "qdrant.config.optional_missing" {
		t.Fatalf("msg=%v", m["msg"])
	}
	if m["qdrant_config"] != "supervised" {
		t.Fatalf("qdrant_config=%v", m["qdrant_config"])
	}
}

func TestNormalizePayload_loadingCollection(t *testing.T) {
	raw := `{"timestamp":"t","level":"INFO","fields":{"message":"Loading collection: claudia-default-x"},"target":"storage::content_manager::toc"}`
	b := NormalizePayload(raw)
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m["msg"] != "qdrant.collection.loading" {
		t.Fatal(m["msg"])
	}
	if m["collection"] != "claudia-default-x" {
		t.Fatal(m["collection"])
	}
}

func TestNormalizePayload_httpUpsertOK(t *testing.T) {
	raw := `{"timestamp":"t","level":"INFO","fields":{"message":"127.0.0.1 \"PUT /collections/coll-a/points?wait=true HTTP/1.1\" 200 92 \"-\" \"Go-http-client/1.1\" 0.001"},"target":"actix_web::middleware::logger"}`
	b := NormalizePayload(raw)
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m["msg"] != "qdrant.http.points_upsert_ok" {
		t.Fatal(m["msg"])
	}
	if m["collection"] != "coll-a" {
		t.Fatal(m["collection"])
	}
	if int(m["http_status"].(float64)) != 200 {
		t.Fatal(m["http_status"])
	}
}

func TestNormalizePayload_httpUpsertRejected(t *testing.T) {
	raw := `{"timestamp":"t","level":"INFO","fields":{"message":"127.0.0.1 \"PUT /collections/coll-b/points?wait=true HTTP/1.1\" 400 130 \"-\" \"Go-http-client/1.1\" 0.0003"},"target":"actix_web::middleware::logger"}`
	b := NormalizePayload(raw)
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m["msg"] != "qdrant.http.points_upsert_rejected" {
		t.Fatal(m["msg"])
	}
	if int(m["http_status"].(float64)) != 400 {
		t.Fatal(m["http_status"])
	}
}

func TestNormalizePayload_idempotent(t *testing.T) {
	raw := string(NormalizePayload(`{"timestamp":"t","level":"INFO","fields":{"message":"Distributed mode disabled"},"target":"qdrant"}`))
	b2 := NormalizePayload(raw)
	if string(b2) != raw {
		t.Fatalf("second pass changed output: %s vs %s", b2, raw)
	}
}

func TestNormalizePayload_telemetryDisabled(t *testing.T) {
	raw := `{"timestamp":"t","level":"INFO","fields":{"message":"Telemetry reporting disabled"},"target":"qdrant"}`
	b := NormalizePayload(raw)
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m["msg"] != "qdrant.telemetry.disabled" {
		t.Fatalf("msg=%v", m["msg"])
	}
	if m["qdrant_telemetry"] != "disabled" {
		t.Fatalf("qdrant_telemetry=%v", m["qdrant_telemetry"])
	}
}

func TestNormalizePayload_recoveryMode(t *testing.T) {
	raw := `{"timestamp":"t","level":"WARN","fields":{"message":"Qdrant is loaded in recovery mode: wal"},"target":"qdrant"}`
	b := NormalizePayload(raw)
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m["msg"] != "qdrant.storage.recovery_mode" {
		t.Fatalf("msg=%v", m["msg"])
	}
	if m["qdrant_recovery"] != "active" {
		t.Fatalf("qdrant_recovery=%v", m["qdrant_recovery"])
	}
}

func TestNormalizePayload_serverStartFailed(t *testing.T) {
	raw := `{"timestamp":"t","level":"ERROR","fields":{"message":"Error while starting REST server: bind error"},"target":"qdrant"}`
	b := NormalizePayload(raw)
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m["msg"] != "qdrant.process.server_start_failed" {
		t.Fatalf("msg=%v", m["msg"])
	}
}

func TestNormalizePayload_tlsEnabledRestFallbackTarget(t *testing.T) {
	raw := `{"timestamp":"t","level":"INFO","fields":{"message":"TLS enabled for REST API (TTL: 3600)"},"target":"other"}`
	b := NormalizePayload(raw)
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m["msg"] != "qdrant.listen.tls_enabled_rest" {
		t.Fatalf("msg=%v", m["msg"])
	}
	if m["qdrant_tls_rest"] != "enabled" {
		t.Fatalf("qdrant_tls_rest=%v", m["qdrant_tls_rest"])
	}
}

func TestNormalizePayload_bootstrapUriDuplicateVariants(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{
			"same_as_peer",
			`{"timestamp":"t","level":"WARN","fields":{"message":"Bootstrap URI is the same as peer URI"},"target":"qdrant"}`,
		},
		{
			"equals_peer",
			`{"timestamp":"t","level":"WARN","fields":{"message":"Warning: Bootstrap URI equals the peer address"},"target":"qdrant"}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := NormalizePayload(tc.raw)
			var m map[string]any
			if err := json.Unmarshal(b, &m); err != nil {
				t.Fatal(err)
			}
			if m["msg"] != "qdrant.cluster.bootstrap_uri_duplicate" {
				t.Fatalf("msg=%v", m["msg"])
			}
		})
	}
}

func TestNormalizePayload_jwtApiKeyWarning(t *testing.T) {
	raw := `{"timestamp":"t","level":"WARN","fields":{"message":"Invalid JWT and API key configuration"},"target":"qdrant"}`
	b := NormalizePayload(raw)
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m["msg"] != "qdrant.security.jwt_rbac_warning" {
		t.Fatalf("msg=%v", m["msg"])
	}
}
