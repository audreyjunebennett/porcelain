package brokeradmin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func resetProviderProbeCacheForTest(t *testing.T) {
	t.Helper()
	InvalidateProviderConfigIndex()
}

func TestListConfiguredProviders(t *testing.T) {
	resetProviderProbeCacheForTest(t)
	var governanceCalls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/governance/providers":
			governanceCalls.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"providers": []map[string]any{
					{"provider": "ollama"},
				},
				"count": 1,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	client := &Client{BaseURL: srv.URL}
	names, ok := ListConfiguredProviders(context.Background(), client)
	if !ok {
		t.Fatal("expected governance list ok")
	}
	if _, have := names["ollama"]; !have {
		t.Fatalf("expected ollama in list: %#v", names)
	}
	if _, have := names["groq"]; have {
		t.Fatalf("groq should not be configured: %#v", names)
	}

	names2, ok2 := ListConfiguredProviders(context.Background(), client)
	if !ok2 || len(names2) != 1 {
		t.Fatalf("cached list: ok=%v names=%#v", ok2, names2)
	}
	if got := governanceCalls.Load(); got != 1 {
		t.Fatalf("governance calls=%d want 1 cached", got)
	}
}

func TestListConfiguredProviders_emptyListNotCached(t *testing.T) {
	resetProviderProbeCacheForTest(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/governance/providers" {
			_ = json.NewEncoder(w).Encode(map[string]any{"count": 0, "providers": nil})
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	client := &Client{BaseURL: srv.URL}
	names, ok := ListConfiguredProviders(context.Background(), client)
	if ok {
		t.Fatalf("empty governance list should not be treated as ok: names=%#v", names)
	}
}

func TestGetProviderForProbe_skipsUnconfiguredFromGovernanceList(t *testing.T) {
	resetProviderProbeCacheForTest(t)
	var providerGets atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/governance/providers":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"providers": []map[string]any{{"provider": "ollama"}},
			})
		case "/api/providers/groq":
			providerGets.Add(1)
			http.NotFound(w, r)
		case "/api/providers/ollama":
			providerGets.Add(1)
			_, _ = w.Write([]byte(`{"name":"ollama","keys":[],"network_config":{"base_url":"http://127.0.0.1:11434"}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	client := &Client{BaseURL: srv.URL}
	body, st, err, httpProbed := GetProviderForProbe(context.Background(), client, "groq")
	if err != nil {
		t.Fatal(err)
	}
	if httpProbed || !IsProviderMissingGET(st, body) {
		t.Fatalf("groq should skip provider GET: httpProbed=%v st=%d body=%s", httpProbed, st, body)
	}
	if got := providerGets.Load(); got != 0 {
		t.Fatalf("provider GETs=%d want 0 for groq", got)
	}

	_, _, err, httpProbed = GetProviderForProbe(context.Background(), client, "ollama")
	if err != nil || !httpProbed {
		t.Fatalf("ollama should probe: err=%v httpProbed=%v", err, httpProbed)
	}
	if got := providerGets.Load(); got != 1 {
		t.Fatalf("provider GETs=%d want 1 for ollama", got)
	}
}

func TestGetProviderForProbe_fallbackNegativeCacheWhenGovernanceUnavailable(t *testing.T) {
	resetProviderProbeCacheForTest(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/providers/groq" {
			http.NotFound(w, r)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	client := &Client{BaseURL: srv.URL}
	_, _, err, httpProbed := GetProviderForProbe(context.Background(), client, "groq")
	if err != nil || !httpProbed {
		t.Fatalf("first groq probe should hit HTTP: err=%v httpProbed=%v", err, httpProbed)
	}

	body, st, err, httpProbed := GetProviderForProbe(context.Background(), client, "groq")
	if err != nil {
		t.Fatal(err)
	}
	if httpProbed || !IsProviderMissingGET(st, body) {
		t.Fatalf("second groq should use negative cache: httpProbed=%v st=%d", httpProbed, st)
	}
}

func TestInvalidateProviderProbeCacheFor(t *testing.T) {
	resetProviderProbeCacheForTest(t)
	probeCacheMu.Lock()
	probeNoKeys["gemini"] = true
	probeCacheMu.Unlock()

	InvalidateProviderProbeCacheFor("gemini")
	dec := probeDecision("gemini", nil, false)
	if !dec.HTTPProbe {
		t.Fatalf("expected probe after invalidate: %+v", dec)
	}
}
