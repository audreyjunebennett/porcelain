package chat

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Phase 2: /v1/chat/completions passes a logger wrapped with request_id, conversation_id,
// and principal_id; chat relay logs must inherit those attributes from the handler.
func TestProxyChatCompletion_logsInheritCorrelationTriple(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      "x",
			"object":  "chat.completion",
			"created": 1,
			"model":   "m",
			"choices": []any{map[string]any{
				"index": 0,
				"message": map[string]any{
					"role":    "assistant",
					"content": "ok",
				},
				"finish_reason": "stop",
			}},
		})
	}))
	t.Cleanup(up.Close)

	var buf strings.Builder
	base := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	log := base.With(
		"request_id", "corr-req-1",
		"conversation_id", "corr-conv-1",
		"principal_id", "corr-principal-1",
		"service", "gateway",
	)

	body := map[string]json.RawMessage{
		"model":    json.RawMessage(`"m"`),
		"messages": json.RawMessage(`[{"role":"user","content":"hi"}]`),
	}
	rec := httptest.NewRecorder()
	_ = ProxyChatCompletion(context.Background(), rec, up.URL, "", "m", false, body, time.Minute, log, nil, nil, nil)

	out := buf.String()
	for _, key := range []string{"request_id=corr-req-1", "conversation_id=corr-conv-1", "principal_id=corr-principal-1"} {
		if !strings.Contains(out, key) {
			t.Fatalf("expected %q in log output:\n%s", key, out)
		}
	}
}

func TestProxyChatCompletion_setsUpstreamRequestIDHeader(t *testing.T) {
	got := make(chan string, 1)
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		got <- r.Header.Get("X-Request-Id")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      "x",
			"object":  "chat.completion",
			"created": 1,
			"model":   "m",
			"choices": []any{map[string]any{
				"index": 0,
				"message": map[string]any{
					"role":    "assistant",
					"content": "ok",
				},
				"finish_reason": "stop",
			}},
		})
	}))
	t.Cleanup(up.Close)

	body := map[string]json.RawMessage{
		"model":    json.RawMessage(`"m"`),
		"messages": json.RawMessage(`[{"role":"user","content":"hi"}]`),
	}
	rec := httptest.NewRecorder()
	_ = ProxyChatCompletion(context.Background(), rec, up.URL, "", "m", false, body, time.Minute, nil, nil, nil, &ProxyOpts{
		UpstreamRequestID: " req-upstream-1 ",
	})

	select {
	case hdr := <-got:
		if hdr != "req-upstream-1" {
			t.Fatalf("X-Request-Id=%q want req-upstream-1", hdr)
		}
	default:
		t.Fatal("upstream did not receive request")
	}
}
