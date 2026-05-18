package chat

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
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
