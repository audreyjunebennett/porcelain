package embedline

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestNormalizePayloadPlainAndSecondPass(t *testing.T) {
	first := NormalizePayload("server listening on 127.0.0.1:8090")
	if !strings.HasPrefix(string(first), `{"timestamp":`) || !strings.HasSuffix(string(first), `"_chimera_norm":1}`) {
		t.Fatalf("unexpected normalized shape: %s", first)
	}
	var buf bytes.Buffer
	w := NewWriter(&buf)
	if _, err := w.Write(append(first, '\n')); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &got); err != nil {
		t.Fatal(err)
	}
	if got["msg"] != "embed.llama_server.listening" || got["service"] != "chimera-embed" {
		t.Fatalf("got=%+v", got)
	}
}

func TestNormalizePayloadPreservesEmbedSlog(t *testing.T) {
	raw := `{"time":"2026-08-29T12:00:00-05:00","level":"INFO","msg":"embed.ready","status":"ok"}`
	var got map[string]any
	if err := json.Unmarshal(NormalizePayload(raw), &got); err != nil {
		t.Fatal(err)
	}
	if got["msg"] != "embed.ready" || got["status"] != "ok" {
		t.Fatalf("got=%+v", got)
	}
}
