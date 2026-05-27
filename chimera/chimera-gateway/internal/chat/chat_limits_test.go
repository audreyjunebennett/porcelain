package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lynn/porcelain/chimera/internal/providerlimits"
)

// chatLimitsUsageStub returns synthetic usage per model for minute vs longer windows.
type chatLimitsUsageStub struct {
	minuteCalls map[string]int64
	dayCalls    map[string]int64
}

func (s *chatLimitsUsageStub) UsageForModelWindow(_ context.Context, modelID string, start, end time.Time) (int64, int64, error) {
	if end.Sub(start) == time.Minute {
		if s.minuteCalls != nil {
			return s.minuteCalls[modelID], 0, nil
		}
		return 0, 0, nil
	}
	if s.dayCalls != nil {
		return s.dayCalls[modelID], 0, nil
	}
	return 0, 0, nil
}

func TestProxyChatCompletion_providerLimitsDeniesBeforeUpstream(t *testing.T) {
	t.Parallel()
	var sawUpstream bool
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawUpstream = true
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(up.Close)

	cfg, err := providerlimits.Parse([]byte(`
providers:
  groq:
    usage_day_timezone: UTC
    rpm: 1
`))
	if err != nil {
		t.Fatal(err)
	}
	guard := &providerlimits.Guard{
		Cfg: cfg,
		Usage: &chatLimitsUsageStub{
			minuteCalls: map[string]int64{"groq/x": 1},
		},
	}

	body := map[string]json.RawMessage{
		"messages": json.RawMessage(`[{"role":"user","content":"hi"}]`),
	}
	w := httptest.NewRecorder()
	pr := ProxyChatCompletion(context.Background(), w, up.URL, "", "groq/x", false, body, time.Minute, nil, nil, guard, nil)
	if sawUpstream {
		t.Fatal("upstream should not be called when limits deny")
	}
	if pr.Status != http.StatusTooManyRequests || pr.ErrMessage != "" {
		t.Fatalf("want 429 without ErrMessage, got status=%d err=%q body=%s", pr.Status, pr.ErrMessage, string(pr.JSONBody))
	}
	var wrap struct {
		Error struct {
			Type string `json:"type"`
		} `json:"error"`
	}
	if err := json.Unmarshal(pr.JSONBody, &wrap); err != nil || wrap.Error.Type != "gateway_provider_limits" {
		t.Fatalf("body: %s err=%v", pr.JSONBody, err)
	}
}

func TestWithVirtualModelFallback_skipsQuotaExhaustedModel(t *testing.T) {
	t.Parallel()
	var lastModel string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Model string `json:"model"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		lastModel = req.Model
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"id":"ok","choices":[]}`)
	}))
	t.Cleanup(up.Close)

	cfg, err := providerlimits.Parse([]byte(`
providers:
  groq:
    usage_day_timezone: UTC
    rpm: 1
`))
	if err != nil {
		t.Fatal(err)
	}
	guard := &providerlimits.Guard{
		Cfg: cfg,
		Usage: &chatLimitsUsageStub{
			minuteCalls: map[string]int64{
				"groq/exhausted": 1,
				"groq/ok":        0,
			},
		},
	}

	body := map[string]json.RawMessage{
		"messages": json.RawMessage(`[{"role":"user","content":"hello"}]`),
	}
	w := httptest.NewRecorder()
	chain := []string{"groq/exhausted", "groq/ok"}
	WithVirtualModelFallback(context.Background(), w, "groq/exhausted", chain, up.URL, "", false, body, time.Minute, nil, nil, guard, nil)

	if lastModel != "groq/ok" {
		t.Fatalf("upstream should see second model, got %q", lastModel)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestWithVirtualModelFallback_allModelsDeniedByLimits_returns429(t *testing.T) {
	t.Parallel()
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("upstream should not be called")
	}))
	t.Cleanup(up.Close)

	cfg, err := providerlimits.Parse([]byte(`
providers:
  groq:
    usage_day_timezone: UTC
    rpm: 1
`))
	if err != nil {
		t.Fatal(err)
	}
	guard := &providerlimits.Guard{
		Cfg: cfg,
		Usage: &chatLimitsUsageStub{
			minuteCalls: map[string]int64{"groq/a": 1, "groq/b": 1},
		},
	}
	body := map[string]json.RawMessage{"messages": json.RawMessage(`[{"role":"user","content":"x"}]`)}
	w := httptest.NewRecorder()
	WithVirtualModelFallback(context.Background(), w, "groq/a", []string{"groq/a", "groq/b"}, up.URL, "", false, body, time.Minute, nil, nil, guard, nil)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("want 429, got %d %s", w.Code, w.Body.String())
	}
}

func TestWithVirtualModelFallback_413_retries_next_model(t *testing.T) {
	t.Parallel()
	var calls []string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Model string `json:"model"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		calls = append(calls, req.Model)
		w.Header().Set("Content-Type", "application/json")
		if req.Model == "groq/too-big" {
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			_, _ = io.WriteString(w, `{"error":{"message":"context"}}`)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"ok"}}]}`)
	}))
	t.Cleanup(up.Close)

	body := map[string]json.RawMessage{"messages": json.RawMessage(`[{"role":"user","content":"hi"}]`)}
	w := httptest.NewRecorder()
	WithVirtualModelFallback(context.Background(), w, "groq/too-big", []string{"groq/too-big", "groq/ok"}, up.URL, "", false, body, time.Minute, nil, nil, nil, nil)

	if len(calls) != 2 || calls[0] != "groq/too-big" || calls[1] != "groq/ok" {
		t.Fatalf("upstream calls=%v", calls)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d body=%s", w.Code, w.Body.String())
	}
}

type recStub413 struct {
	mu  sync.Mutex
	out []int
}

func (r *recStub413) RecordBrokerResponse(_ time.Time, _ string, status int, _ int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.out = append(r.out, status)
}

func TestWithVirtualModelFallback_413_records_metrics_per_attempt(t *testing.T) {
	t.Parallel()
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Model string `json:"model"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		w.Header().Set("Content-Type", "application/json")
		if req.Model == "groq/a" {
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			_, _ = io.WriteString(w, `{"error":{"message":"nope"}}`)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"y"}}]}`)
	}))
	t.Cleanup(up.Close)

	rec := &recStub413{}
	body := map[string]json.RawMessage{"messages": json.RawMessage(`[{"role":"user","content":"x"}]`)}
	w := httptest.NewRecorder()
	WithVirtualModelFallback(context.Background(), w, "groq/a", []string{"groq/a", "groq/b"}, up.URL, "", false, body, time.Minute, nil, rec, nil, nil)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	rec.mu.Lock()
	got := append([]int(nil), rec.out...)
	rec.mu.Unlock()
	if len(got) != 2 || got[0] != http.StatusRequestEntityTooLarge || got[1] != http.StatusOK {
		t.Fatalf("metrics statuses=%v want [413,200]", got)
	}
}

func TestWithVirtualModelFallback_skips_duplicate_after_413(t *testing.T) {
	t.Parallel()
	var calls []string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Model string `json:"model"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		calls = append(calls, req.Model)
		w.Header().Set("Content-Type", "application/json")
		if req.Model == "groq/dup" {
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			_, _ = io.WriteString(w, `{"error":{}}`)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"z"}}]}`)
	}))
	t.Cleanup(up.Close)

	chain := []string{"groq/dup", "groq/dup", "groq/after"}
	body := map[string]json.RawMessage{"messages": json.RawMessage(`[{"role":"user","content":"h"}]`)}
	w := httptest.NewRecorder()
	WithVirtualModelFallback(context.Background(), w, "groq/dup", chain, up.URL, "", false, body, time.Minute, nil, nil, nil, nil)

	// First dup 413; second dup index skipped without a second upstream call to groq/dup.
	if len(calls) != 2 || calls[0] != "groq/dup" || calls[1] != "groq/after" {
		t.Fatalf("calls=%v", calls)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d", w.Code)
	}
}

func TestWithVirtualModelFallback_404_retries_next_model(t *testing.T) {
	t.Parallel()
	var calls []string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Model string `json:"model"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		calls = append(calls, req.Model)
		w.Header().Set("Content-Type", "application/json")
		if req.Model == "groq/missing" {
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"error":{"message":"model not found"}}`)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"ok"}}]}`)
	}))
	t.Cleanup(up.Close)

	body := map[string]json.RawMessage{"messages": json.RawMessage(`[{"role":"user","content":"hi"}]`)}
	w := httptest.NewRecorder()
	WithVirtualModelFallback(context.Background(), w, "groq/missing", []string{"groq/missing", "groq/ok"}, up.URL, "", false, body, time.Minute, nil, nil, nil, nil)

	if len(calls) != 2 || calls[0] != "groq/missing" || calls[1] != "groq/ok" {
		t.Fatalf("upstream calls=%v", calls)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestWithVirtualModelFallback_404_exhausted_returns_wrapup(t *testing.T) {
	t.Parallel()
	var calls []string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Model string `json:"model"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		calls = append(calls, req.Model)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"error":{"message":"nope"}}`)
	}))
	t.Cleanup(up.Close)

	body := map[string]json.RawMessage{"messages": json.RawMessage(`[{"role":"user","content":"x"}]`)}
	w := httptest.NewRecorder()
	WithVirtualModelFallback(context.Background(), w, "groq/a", []string{"groq/a", "groq/b"}, up.URL, "", false, body, time.Minute, nil, nil, nil, nil)

	if len(calls) != 2 {
		t.Fatalf("calls=%v", calls)
	}
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d body=%s", w.Code, w.Body.String())
	}
	var wrap struct {
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
			Details struct {
				Attempts []struct {
					UpstreamModel string `json:"upstream_model"`
					Status        int    `json:"status"`
					Summary       string `json:"summary"`
				} `json:"attempts"`
			} `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &wrap); err != nil {
		t.Fatal(err)
	}
	if wrap.Error.Type != "gateway_fallback_exhausted" {
		t.Fatalf("error.type=%q", wrap.Error.Type)
	}
	if len(wrap.Error.Details.Attempts) != 2 {
		t.Fatalf("attempts=%v", wrap.Error.Details.Attempts)
	}
	if wrap.Error.Details.Attempts[0].Status != http.StatusNotFound || wrap.Error.Details.Attempts[0].UpstreamModel != "groq/a" {
		t.Fatalf("attempt0=%+v", wrap.Error.Details.Attempts[0])
	}
	if wrap.Error.Details.Attempts[1].UpstreamModel != "groq/b" {
		t.Fatalf("attempt1=%+v", wrap.Error.Details.Attempts[1])
	}
	if wrap.Error.Message == "" {
		t.Fatal("empty summary message")
	}
}

func TestWithVirtualModelFallback_404_records_metrics_per_attempt(t *testing.T) {
	t.Parallel()
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Model string `json:"model"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		w.Header().Set("Content-Type", "application/json")
		if req.Model == "groq/a" {
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"error":{"message":"missing"}}`)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"y"}}]}`)
	}))
	t.Cleanup(up.Close)

	rec := &recStub413{}
	body := map[string]json.RawMessage{"messages": json.RawMessage(`[{"role":"user","content":"x"}]`)}
	w := httptest.NewRecorder()
	WithVirtualModelFallback(context.Background(), w, "groq/a", []string{"groq/a", "groq/b"}, up.URL, "", false, body, time.Minute, nil, rec, nil, nil)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	rec.mu.Lock()
	got := append([]int(nil), rec.out...)
	rec.mu.Unlock()
	if len(got) != 2 || got[0] != http.StatusNotFound || got[1] != http.StatusOK {
		t.Fatalf("metrics statuses=%v want [404,200]", got)
	}
}

func TestUpstreamErrorIndicatesRateLimit(t *testing.T) {
	t.Parallel()
	cases := []struct {
		status int
		body   string
		errMsg string
		want   bool
	}{
		{http.StatusTooManyRequests, `{"error":{"message":"slow down"}}`, "", true},
		{http.StatusBadRequest, `{"error":{"message":"Rate limit reached for model"}}`, "", true},
		{http.StatusBadRequest, `{"error":{"message":"too many requests"}}`, "", true},
		{http.StatusBadRequest, `{"error":{"message":"The model does not exist","type":"invalid_request_error"}}`, "", false},
		{http.StatusBadRequest, `{"error":{"message":"bad json","type":"rate_limit_exceeded"}}`, "", true},
		{http.StatusBadRequest, "", "rate limit exceeded", true},
		{http.StatusInternalServerError, `{"error":{"message":"rate limit"}}`, "", false},
	}
	for _, tc := range cases {
		got := upstreamErrorIndicatesRateLimit(tc.status, []byte(tc.body), tc.errMsg)
		if got != tc.want {
			t.Fatalf("status=%d body=%q err=%q: got %v want %v", tc.status, tc.body, tc.errMsg, got, tc.want)
		}
	}
}

func TestWithVirtualModelFallback_400_rate_limit_retries_next_model(t *testing.T) {
	t.Parallel()
	var calls []string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Model string `json:"model"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		calls = append(calls, req.Model)
		w.Header().Set("Content-Type", "application/json")
		if req.Model == "groq/limited" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"error":{"message":"Rate limit reached for model in organization"}}`)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"ok"}}]}`)
	}))
	t.Cleanup(up.Close)

	body := map[string]json.RawMessage{"messages": json.RawMessage(`[{"role":"user","content":"hi"}]`)}
	w := httptest.NewRecorder()
	WithVirtualModelFallback(context.Background(), w, "groq/limited", []string{"groq/limited", "groq/ok"}, up.URL, "", false, body, time.Minute, nil, nil, nil, nil)

	if len(calls) != 2 || calls[0] != "groq/limited" || calls[1] != "groq/ok" {
		t.Fatalf("upstream calls=%v", calls)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestWithVirtualModelFallback_400_rate_limit_exhausted_returns_400_wrapup(t *testing.T) {
	t.Parallel()
	var calls []string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Model string `json:"model"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		calls = append(calls, req.Model)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":{"message":"Rate limit reached","type":"rate_limit_exceeded"}}`)
	}))
	t.Cleanup(up.Close)

	body := map[string]json.RawMessage{"messages": json.RawMessage(`[{"role":"user","content":"x"}]`)}
	w := httptest.NewRecorder()
	WithVirtualModelFallback(context.Background(), w, "groq/a", []string{"groq/a", "groq/b"}, up.URL, "", false, body, time.Minute, nil, nil, nil, nil)

	if len(calls) != 2 {
		t.Fatalf("calls=%v", calls)
	}
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d body=%s", w.Code, w.Body.String())
	}
	var wrap struct {
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
			Details struct {
				Attempts     []map[string]any `json:"attempts"`
				ModelsTried  []string         `json:"models_tried"`
				AttemptCount int              `json:"attempt_count"`
				ChainLen     int              `json:"chain_len"`
				Cause        string           `json:"cause"`
			} `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &wrap); err != nil {
		t.Fatal(err)
	}
	if wrap.Error.Type != "gateway_rate_limit_exhausted" {
		t.Fatalf("error.type=%q", wrap.Error.Type)
	}
	if len(wrap.Error.Details.Attempts) != 2 {
		t.Fatalf("attempts=%v", wrap.Error.Details.Attempts)
	}
	if len(wrap.Error.Details.ModelsTried) != 2 || wrap.Error.Details.ModelsTried[0] != "groq/a" || wrap.Error.Details.ModelsTried[1] != "groq/b" {
		t.Fatalf("models_tried=%v", wrap.Error.Details.ModelsTried)
	}
	if wrap.Error.Details.AttemptCount != 2 || wrap.Error.Details.ChainLen != 2 || wrap.Error.Details.Cause != "rate_limit" {
		t.Fatalf("details=%+v", wrap.Error.Details)
	}
	if wrap.Error.Message == "" || !strings.Contains(wrap.Error.Message, "rate-limited") {
		t.Fatalf("message=%q", wrap.Error.Message)
	}
}

func TestWithVirtualModelFallback_400_model_not_found_does_not_retry(t *testing.T) {
	t.Parallel()
	var calls []string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Model string `json:"model"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		calls = append(calls, req.Model)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":{"message":"The model `+"`groq/missing`"+` does not exist"}}`)
	}))
	t.Cleanup(up.Close)

	body := map[string]json.RawMessage{"messages": json.RawMessage(`[{"role":"user","content":"hi"}]`)}
	w := httptest.NewRecorder()
	WithVirtualModelFallback(context.Background(), w, "groq/missing", []string{"groq/missing", "groq/ok"}, up.URL, "", false, body, time.Minute, nil, nil, nil, nil)

	if len(calls) != 1 {
		t.Fatalf("upstream calls=%v want single attempt", calls)
	}
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400 passthrough, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestWithVirtualModelFallback_429_exhausted_returns_400_wrapup(t *testing.T) {
	t.Parallel()
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"error":{"message":"rate limited"}}`)
	}))
	t.Cleanup(up.Close)

	body := map[string]json.RawMessage{"messages": json.RawMessage(`[{"role":"user","content":"x"}]`)}
	w := httptest.NewRecorder()
	WithVirtualModelFallback(context.Background(), w, "groq/a", []string{"groq/a", "groq/b"}, up.URL, "", false, body, time.Minute, nil, nil, nil, nil)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d body=%s", w.Code, w.Body.String())
	}
	var wrap struct {
		Error struct {
			Type string `json:"type"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &wrap); err != nil {
		t.Fatal(err)
	}
	if wrap.Error.Type != "gateway_rate_limit_exhausted" {
		t.Fatalf("error.type=%q", wrap.Error.Type)
	}
}

func largePromptBody(t *testing.T) map[string]json.RawMessage {
	t.Helper()
	content := strings.Repeat("hello world ", 80)
	return map[string]json.RawMessage{
		"messages": json.RawMessage(fmt.Sprintf(`[{"role":"user","content":%q}]`, content)),
	}
}

func contextLimitsGuard(t *testing.T, yaml string) *providerlimits.Guard {
	t.Helper()
	cfg, err := providerlimits.Parse([]byte(yaml))
	if err != nil {
		t.Fatal(err)
	}
	return &providerlimits.Guard{Cfg: cfg, Usage: nil}
}

func TestMaxTokensFromBody(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		body map[string]json.RawMessage
		want int64
	}{
		{"nil body", nil, 0},
		{"omitted", map[string]json.RawMessage{"messages": json.RawMessage(`[]`)}, 0},
		{"int", map[string]json.RawMessage{"max_tokens": json.RawMessage(`512`)}, 512},
		{"json number", map[string]json.RawMessage{"max_tokens": json.RawMessage(`1024`)}, 1024},
		{"negative", map[string]json.RawMessage{"max_tokens": json.RawMessage(`-1`)}, 0},
		{"invalid", map[string]json.RawMessage{"max_tokens": json.RawMessage(`"nope"`)}, 0},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := maxTokensFromBody(tc.body); got != tc.want {
				t.Fatalf("got %d want %d", got, tc.want)
			}
		})
	}
}

func TestUpstreamErrorIndicatesContextOverflow(t *testing.T) {
	t.Parallel()
	cases := []struct {
		status int
		body   string
		errMsg string
		want   bool
	}{
		{http.StatusBadRequest, `{"error":{"message":"too big","code":"request_too_large"}}`, "", true},
		{http.StatusBadRequest, `{"error":{"message":"too big","type":"request_too_large"}}`, "", true},
		{http.StatusUnprocessableEntity, `{"error":{"code":"context_length_exceeded"}}`, "", true},
		{http.StatusBadRequest, `{"error":{"message":"Rate limit reached"}}`, "", false},
		{http.StatusBadRequest, `{"error":{"message":"model missing"}}`, "", false},
		{http.StatusRequestEntityTooLarge, `{"error":{"message":"nope"}}`, "", false},
	}
	for _, tc := range cases {
		got := upstreamErrorIndicatesContextOverflow(tc.status, []byte(tc.body), tc.errMsg)
		if got != tc.want {
			t.Fatalf("status=%d body=%q: got %v want %v", tc.status, tc.body, got, tc.want)
		}
	}
}

func TestShouldRetryVirtualModelFallback_contextOverflow(t *testing.T) {
	t.Parallel()
	body := `{"error":{"code":"request_too_large","message":"Request too large for model"}}`
	if !shouldRetryVirtualModelFallback(http.StatusBadRequest, []byte(body), "") {
		t.Fatal("expected retry for request_too_large")
	}
}

func TestWithVirtualModelFallback_skipsContextBlockedModel(t *testing.T) {
	t.Parallel()
	var lastModel string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Model string `json:"model"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		lastModel = req.Model
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"id":"ok","choices":[]}`)
	}))
	t.Cleanup(up.Close)

	guard := contextLimitsGuard(t, `
defaults:
  context_safety_factor: 1.0
providers:
  groq:
    models:
      groq/too-big:
        max_prompt_tokens: 10
      groq/ok:
        max_prompt_tokens: 100000
`)

	w := httptest.NewRecorder()
	chain := []string{"groq/too-big", "groq/ok"}
	WithVirtualModelFallback(context.Background(), w, "groq/too-big", chain, up.URL, "", false, largePromptBody(t), time.Minute, nil, nil, guard, nil)

	if lastModel != "groq/ok" {
		t.Fatalf("upstream should see second model, got %q", lastModel)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestWithVirtualModelFallback_allModelsDeniedByContext_returns429(t *testing.T) {
	t.Parallel()
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("upstream should not be called")
	}))
	t.Cleanup(up.Close)

	guard := contextLimitsGuard(t, `
defaults:
  context_safety_factor: 1.0
providers:
  groq:
    models:
      groq/a:
        max_prompt_tokens: 10
      groq/b:
        max_prompt_tokens: 10
`)

	w := httptest.NewRecorder()
	WithVirtualModelFallback(context.Background(), w, "groq/a", []string{"groq/a", "groq/b"}, up.URL, "", false, largePromptBody(t), time.Minute, nil, nil, guard, nil)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("want 429, got %d %s", w.Code, w.Body.String())
	}
	var wrap struct {
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &wrap); err != nil {
		t.Fatal(err)
	}
	if wrap.Error.Type != "gateway_provider_limits" {
		t.Fatalf("error.type=%q", wrap.Error.Type)
	}
	if !strings.Contains(wrap.Error.Message, "context_window") {
		t.Fatalf("message=%q", wrap.Error.Message)
	}
}

func TestWithVirtualModelFallback_400_request_too_large_retries_next_model(t *testing.T) {
	t.Parallel()
	var calls []string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Model string `json:"model"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		calls = append(calls, req.Model)
		w.Header().Set("Content-Type", "application/json")
		if req.Model == "groq/groq/compound-mini" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"error":{"message":"Request too large","code":"request_too_large"}}`)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"ok"}}]}`)
	}))
	t.Cleanup(up.Close)

	body := map[string]json.RawMessage{"messages": json.RawMessage(`[{"role":"user","content":"hi"}]`)}
	w := httptest.NewRecorder()
	WithVirtualModelFallback(context.Background(), w, "groq/groq/compound-mini", []string{"groq/groq/compound-mini", "ollama/llama3.2:3b"}, up.URL, "", false, body, time.Minute, nil, nil, nil, nil)

	if len(calls) != 2 || calls[0] != "groq/groq/compound-mini" || calls[1] != "ollama/llama3.2:3b" {
		t.Fatalf("upstream calls=%v", calls)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestWithVirtualModelFallback_request_too_large_skips_duplicate_model(t *testing.T) {
	t.Parallel()
	var calls []string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Model string `json:"model"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		calls = append(calls, req.Model)
		w.Header().Set("Content-Type", "application/json")
		if req.Model == "groq/dup" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"error":{"code":"request_too_large"}}`)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"z"}}]}`)
	}))
	t.Cleanup(up.Close)

	chain := []string{"groq/dup", "groq/dup", "groq/after"}
	body := map[string]json.RawMessage{"messages": json.RawMessage(`[{"role":"user","content":"h"}]`)}
	w := httptest.NewRecorder()
	WithVirtualModelFallback(context.Background(), w, "groq/dup", chain, up.URL, "", false, body, time.Minute, nil, nil, nil, nil)

	if len(calls) != 2 || calls[0] != "groq/dup" || calls[1] != "groq/after" {
		t.Fatalf("calls=%v", calls)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d", w.Code)
	}
}

func TestProxyChatCompletion_contextDenyBeforeUpstream(t *testing.T) {
	t.Parallel()
	var sawUpstream bool
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawUpstream = true
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(up.Close)

	guard := contextLimitsGuard(t, `
defaults:
  context_safety_factor: 1.0
providers:
  groq:
    models:
      groq/x:
        max_prompt_tokens: 10
`)

	w := httptest.NewRecorder()
	pr := ProxyChatCompletion(context.Background(), w, up.URL, "", "groq/x", false, largePromptBody(t), time.Minute, nil, nil, guard, nil)
	if sawUpstream {
		t.Fatal("upstream should not be called when context limits deny")
	}
	if pr.Status != http.StatusTooManyRequests {
		t.Fatalf("want 429, got status=%d body=%s", pr.Status, pr.JSONBody)
	}
	var wrap struct {
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(pr.JSONBody, &wrap); err != nil {
		t.Fatal(err)
	}
	if wrap.Error.Type != "gateway_provider_limits" || !strings.Contains(wrap.Error.Message, "context_window") {
		t.Fatalf("body: %s", pr.JSONBody)
	}
}

func TestProviderLimitsBlockedLogArgs_contextFields(t *testing.T) {
	t.Parallel()
	cfg, err := providerlimits.Parse([]byte(`
defaults:
  context_safety_factor: 1.0
providers:
  groq:
    models:
      groq/x:
        max_prompt_tokens: 8192
`))
	if err != nil {
		t.Fatal(err)
	}
	guard := &providerlimits.Guard{Cfg: cfg}
	admission := providerlimits.RequestAdmission{
		EstPromptTokens: 9500,
		MaxTokens:       512,
		BodyBytes:       120000,
	}
	d := providerlimits.Decision{
		Allowed: false,
		Reason:  providerlimits.ReasonContext,
		Detail:  "context cap 8192 would be exceeded",
	}
	args := providerLimitsBlockedLogArgs("groq/x", d, admission, guard)
	m := logArgsMap(args)
	for _, key := range []string{"msg", "reason", "outgoingTokens", "max_tokens", "body_bytes", "context_cap"} {
		if _, ok := m[key]; !ok {
			t.Fatalf("missing log field %q in %v", key, m)
		}
	}
	if m["reason"] != "context_window" {
		t.Fatalf("reason=%q", m["reason"])
	}
	if m["context_cap"] != int64(8192) {
		t.Fatalf("context_cap=%v", m["context_cap"])
	}
}

func TestProviderLimitsBlockedLogArgs_bodySizeFields(t *testing.T) {
	t.Parallel()
	maxBody := int64(3500000)
	cfg, err := providerlimits.Parse([]byte(`
schema_version: 2
defaults:
  max_body_bytes: 3500000
`))
	if err != nil {
		t.Fatal(err)
	}
	guard := &providerlimits.Guard{Cfg: cfg}
	d := providerlimits.Decision{Allowed: false, Reason: providerlimits.ReasonBodySize}
	args := providerLimitsBlockedLogArgs("groq/x", d, providerlimits.RequestAdmission{BodyBytes: 4000000}, guard)
	m := logArgsMap(args)
	if m["max_body_bytes"] != maxBody {
		t.Fatalf("max_body_bytes=%v", m["max_body_bytes"])
	}
}

func logArgsMap(args []any) map[string]any {
	out := make(map[string]any, len(args)/2)
	for i := 0; i+1 < len(args); i += 2 {
		key, ok := args[i].(string)
		if !ok {
			continue
		}
		out[key] = args[i+1]
	}
	return out
}
