package bifrostline

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizePayload_httpAccessAndRateLimit(t *testing.T) {
	raw := `{"level":"info","http.method":"POST","http.target":"/v1/chat/completions","http.status_code":200,"http.request_duration_ms":348,"time":"2026-05-08T14:29:53-05:00","message":"request completed"}`
	b := NormalizePayload(raw)
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m["msg"] != "bifrost.http.access" {
		t.Fatalf("msg=%v", m["msg"])
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
	if m429["msg"] != "bifrost.rate_limit" {
		t.Fatalf("msg=%v", m429["msg"])
	}
}

func TestNormalizePayload_readyAndBootstrap(t *testing.T) {
	raw := `{"level":"info","time":"2026-05-08T14:15:51-05:00","message":"successfully started bifrost, serving UI on http://127.0.0.1:8080"}`
	b := NormalizePayload(raw)
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m["msg"] != "bifrost.ready" {
		t.Fatalf("msg=%v", m["msg"])
	}
	if int(m["listen_port"].(float64)) != 8080 {
		t.Fatal(m["listen_port"])
	}

	rawB := `{"level":"info","time":"t","message":"Time spent in Bifrost server bootstrap 2232 ms"}`
	bb := NormalizePayload(rawB)
	var mb map[string]any
	if err := json.Unmarshal(bb, &mb); err != nil {
		t.Fatal(err)
	}
	if mb["msg"] != "bifrost.bootstrap.complete" {
		t.Fatal(mb["msg"])
	}
	if int(mb["bootstrap_ms"].(float64)) != 2232 {
		t.Fatal(mb["bootstrap_ms"])
	}
}

func TestNormalizePayload_catalogModelCount(t *testing.T) {
	raw := `{"level":"info","time":"2026-05-08T20:00:00-05:00","message":"42 models added to catalog"}`
	b := NormalizePayload(raw)
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m["msg"] != "bifrost.catalog.sync" {
		t.Fatalf("msg=%v", m["msg"])
	}
	if int(m["catalog_model_count"].(float64)) != 42 {
		t.Fatalf("catalog_model_count=%v", m["catalog_model_count"])
	}
}

func TestNormalizePayload_idempotent(t *testing.T) {
	raw := string(NormalizePayload(`{"level":"info","time":"t","message":"bifrost client initialized"}`))
	b2 := NormalizePayload(raw)
	if string(b2) != raw {
		t.Fatalf("second pass changed output: %s vs %s", b2, raw)
	}
}

func TestNormalizePayload_fixturesNoUnparsed(t *testing.T) {
	dir := filepath.Join("testdata")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, ent := range entries {
		if ent.IsDir() || !strings.HasSuffix(ent.Name(), ".log") {
			continue
		}
		t.Run(ent.Name(), func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(dir, ent.Name()))
			if err != nil {
				t.Fatal(err)
			}
			lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
			for i, line := range lines {
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}
				out := NormalizePayload(line)
				var m map[string]any
				if err := json.Unmarshal(out, &m); err != nil {
					t.Fatalf("line %d: invalid json: %v\n%s", i+1, err, line)
				}
				msg, _ := m["msg"].(string)
				if msg == "bifrost.unparsed" {
					t.Fatalf("line %d: unparsed\n%s", i+1, line)
				}
				if !strings.HasPrefix(msg, "bifrost.") {
					t.Fatalf("line %d: msg=%q", i+1, msg)
				}
				if m["service"] != "bifrost" {
					t.Fatalf("line %d: service=%v", i+1, m["service"])
				}
			}
		})
	}
}
