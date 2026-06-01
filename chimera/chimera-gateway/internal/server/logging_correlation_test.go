package server

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lynn/porcelain/chimera/internal/platform/requestid"
)

func TestHTTPAccessLogLevel_probesAndErrors(t *testing.T) {
	cases := []struct {
		path   string
		status int
		want   slog.Level
	}{
		{"/health", 200, slog.LevelDebug},
		{"/healthz", 200, slog.LevelDebug},
		{"/readyz", 200, slog.LevelDebug},
		{"/status", 204, slog.LevelDebug},
		{"/api/ui/logs", 200, slog.LevelDebug},
		{"/api/ui/logs/stream", 200, slog.LevelDebug},
		{"/ui/assets/theme-tokens.css", 200, slog.LevelDebug},
		{"/ui/assets/settings/main.js", 200, slog.LevelDebug},
		{"/ui/login", 200, slog.LevelInfo},
		{"/v1/indexer/workspaces", 200, slog.LevelDebug},
		{"/v1/indexer/workspaces", 503, slog.LevelInfo},
		{"/api/ui/tokens", 200, slog.LevelDebug},
		{"/api/ui/tokens", 503, slog.LevelInfo},
		{"/api/ui/state", 200, slog.LevelDebug},
		{"/api/ui/chimera-broker/providers", 200, slog.LevelDebug},
		{"/api/ui/providers/catalog", 200, slog.LevelDebug},
		{"/api/ui/indexer/config", 200, slog.LevelDebug},
		{"/v1/indexer/storage/stats", 200, slog.LevelDebug},
		{"/v1/indexer/storage/stats", 502, slog.LevelInfo},
		{"/v1/ingest", 200, slog.LevelDebug},
		{"/v1/ingest", 502, slog.LevelInfo},
		{"/health", 503, slog.LevelInfo},
		{"/healthz", 503, slog.LevelInfo},
		{"/readyz", 503, slog.LevelInfo},
		{"/v1/chat/completions", 200, slog.LevelInfo},
		{"/v1/chat/completions", 500, slog.LevelInfo},
	}
	for _, tc := range cases {
		if got := httpAccessLogLevel(tc.path, tc.status); got != tc.want {
			t.Fatalf("path=%q status=%d: got %v want %v", tc.path, tc.status, got, tc.want)
		}
	}
}

func TestLoggingMiddleware_emitsRequestID(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	h := requestid.Middleware(loggingMiddleware(log, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	h.ServeHTTP(rec, req)
	out := buf.String()
	if !strings.Contains(out, "request_id=") {
		t.Fatalf("missing request_id in log: %q", out)
	}
	if !strings.Contains(out, "service=gateway") {
		t.Fatalf("missing service=gateway: %q", out)
	}
	if !strings.Contains(out, "timeline_kind=web") {
		t.Fatalf("missing timeline_kind=web for /health: %q", out)
	}
}

func TestLoggingMiddleware_uiPollingSuccessAtDebug(t *testing.T) {
	paths := []string{
		"/api/ui/tokens",
		"/api/ui/state",
		"/api/ui/chimera-broker/providers",
		"/api/ui/providers/catalog",
		"/api/ui/indexer/config",
		"/v1/indexer/storage/stats",
	}
	for _, path := range paths {
		path := path
		t.Run(path, func(t *testing.T) {
			var buf bytes.Buffer
			log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
			h := loggingMiddleware(log, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, path, nil)
			h.ServeHTTP(rec, req)
			if out := buf.String(); out != "" {
				t.Fatalf("expected no INFO log for %s, got %q", path, out)
			}
		})
	}
}

func TestLoggingMiddleware_uiPollingFailureAtInfo(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	h := loggingMiddleware(log, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/ui/state", nil)
	h.ServeHTTP(rec, req)
	out := buf.String()
	if !strings.Contains(out, "level=INFO") {
		t.Fatalf("expected INFO log for failed poll, got %q", out)
	}
	if !strings.Contains(out, "path=/api/ui/state") {
		t.Fatalf("missing path in log: %q", out)
	}
}

func TestOptionalConversationIDFromHeader_set(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	r.Header.Set(headerConversationID, "sess-abc-1")
	if got := OptionalConversationIDFromHeader(r); got != "sess-abc-1" {
		t.Fatalf("got %q", got)
	}
}

func TestOptionalConversationIDFromHeader_empty(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	if got := OptionalConversationIDFromHeader(r); got != "" {
		t.Fatalf("got %q want empty", got)
	}
}
