package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClassifyBifrostProviderResult_states(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		provider  string
		body      []byte
		status    int
		err       error
		wantState string
		wantKeys  int
		wantCfg   bool
		wantBase  string
	}{
		{
			name:      "groq up with one key",
			provider:  "groq",
			body:      []byte(`{"name":"groq","keys":[{"name":"k","value":"env.GROQ_API_KEY"}]}`),
			status:    200,
			wantState: "up",
			wantKeys:  1,
			wantCfg:   true,
		},
		{
			name:      "gemini key_missing when keys array empty",
			provider:  "gemini",
			body:      []byte(`{"name":"gemini","keys":[]}`),
			status:    200,
			wantState: "key_missing",
			wantKeys:  0,
			wantCfg:   false,
		},
		{
			name:      "ollama up via base_url even without keys",
			provider:  "ollama",
			body:      []byte(`{"name":"ollama","keys":[],"network_config":{"base_url":"http://127.0.0.1:11434"}}`),
			status:    200,
			wantState: "up",
			wantKeys:  0,
			wantBase:  "http://127.0.0.1:11434",
		},
		{
			name:      "ollama key_missing when no base_url and no keys",
			provider:  "ollama",
			body:      []byte(`{"name":"ollama","keys":[]}`),
			status:    200,
			wantState: "key_missing",
		},
		{
			name:      "unknown when provider missing 404",
			provider:  "groq",
			body:      []byte(`{"is_bifrost_error":false,"status_code":404,"error":{"message":"Provider not found"}}`),
			status:    200,
			wantState: "unknown",
		},
		{
			name:      "down when bifrost transport error",
			provider:  "gemini",
			err:       errors.New("dial tcp 127.0.0.1:8080: connect: connection refused"),
			wantState: "down",
		},
		{
			name:      "down when bifrost returns 5xx",
			provider:  "gemini",
			body:      []byte(`{"error":"internal"}`),
			status:    503,
			wantState: "down",
		},
		{
			name:      "unknown when bifrost returns 4xx (other than missing)",
			provider:  "gemini",
			body:      []byte(`{"error":"bad request"}`),
			status:    400,
			wantState: "unknown",
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := classifyBifrostProviderResult(tc.provider, tc.body, tc.status, tc.err, nil)
			if got.State != tc.wantState {
				t.Fatalf("state=%q want %q (entry=%+v)", got.State, tc.wantState, got)
			}
			if got.KeyCount != tc.wantKeys {
				t.Fatalf("key_count=%d want %d", got.KeyCount, tc.wantKeys)
			}
			if got.KeyConfigured != tc.wantCfg {
				t.Fatalf("key_configured=%v want %v", got.KeyConfigured, tc.wantCfg)
			}
			if got.OllamaBaseURL != tc.wantBase {
				t.Fatalf("ollama_base_url=%q want %q", got.OllamaBaseURL, tc.wantBase)
			}
		})
	}
}

func TestFetchBifrostProviderHealth_emptyBaseURL(t *testing.T) {
	t.Parallel()
	resp := fetchBifrostProviderHealth(context.Background(), nil, []string{"groq", "ollama"}, nil)
	if resp.BifrostUp {
		t.Fatalf("bifrost_up should be false with nil client")
	}
	if !strings.Contains(resp.Error, "not configured") {
		t.Fatalf("error: %q", resp.Error)
	}
	if len(resp.Providers) != 2 {
		t.Fatalf("providers len=%d", len(resp.Providers))
	}
	for _, p := range resp.Providers {
		if p.State != "down" {
			t.Fatalf("provider %q state=%q want down", p.ID, p.State)
		}
	}
}

func TestUIBifrostProviderHealth_endToEnd(t *testing.T) {
	t.Setenv("CLAUDIA_UPSTREAM_API_KEY", "ukey")

	bifrost := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			w.WriteHeader(http.StatusOK)
		case "/api/providers/groq":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"name":"groq","keys":[{"name":"claudia-groq-key-1","value":"env.GROQ_API_KEY"}]}`))
		case "/api/providers/gemini":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"name":"gemini","keys":[]}`))
		case "/api/providers/ollama":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"name":"ollama","keys":[],"network_config":{"base_url":"http://127.0.0.1:11434"}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(bifrost.Close)

	dir := t.TempDir()
	gwPath := filepath.Join(dir, "gateway.yaml")
	writeGateway(t, gwPath, bifrost.URL, []string{"m"})
	tokPath := filepath.Join(dir, "tokens.yaml")
	writeTokens(t, tokPath, "gw-bifrost-health", "t1")
	routePath := filepath.Join(dir, "routing-policy.yaml")
	if err := os.WriteFile(routePath, []byte("rules: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rt, err := NewRuntime(gwPath, testLog())
	if err != nil {
		t.Fatal(err)
	}
	front := httptest.NewServer(NewMux(rt, testLog(), nil, NewUIOptions()))
	t.Cleanup(front.Close)

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar}

	loginRes, err := client.Post(front.URL+"/api/ui/login", "application/json", strings.NewReader(`{"token":"gw-bifrost-health"}`))
	if err != nil {
		t.Fatal(err)
	}
	loginRes.Body.Close()
	if loginRes.StatusCode != http.StatusOK {
		t.Fatalf("login %d", loginRes.StatusCode)
	}

	res, err := client.Get(front.URL + "/api/ui/bifrost/providers")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("provider health %d %s", res.StatusCode, b)
	}
	var doc bifrostProviderHealthResponse
	if err := json.NewDecoder(res.Body).Decode(&doc); err != nil {
		t.Fatal(err)
	}
	if !doc.BifrostUp {
		t.Fatalf("bifrost_up should be true: %+v", doc)
	}
	if doc.FetchedAt.IsZero() {
		t.Fatalf("fetched_at zero: %+v", doc)
	}
	if len(doc.Providers) != 3 {
		t.Fatalf("providers len=%d want 3: %+v", len(doc.Providers), doc.Providers)
	}
	byID := map[string]bifrostProviderHealthEntry{}
	for _, p := range doc.Providers {
		byID[p.ID] = p
	}
	if got := byID["groq"]; got.State != "up" || got.KeyCount != 1 || !got.KeyConfigured {
		t.Fatalf("groq: %+v", got)
	}
	if got := byID["gemini"]; got.State != "key_missing" || got.KeyConfigured {
		t.Fatalf("gemini: %+v", got)
	}
	if got := byID["ollama"]; got.State != "up" || got.OllamaBaseURL != "http://127.0.0.1:11434" {
		t.Fatalf("ollama: %+v", got)
	}
}

func TestUIBifrostProviderHealth_bifrostDown(t *testing.T) {
	t.Setenv("CLAUDIA_UPSTREAM_API_KEY", "ukey")

	// Allocate a server only to capture an unused port, then close it so dialing fails.
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	deadURL := dead.URL
	dead.Close()

	dir := t.TempDir()
	gwPath := filepath.Join(dir, "gateway.yaml")
	writeGateway(t, gwPath, deadURL, []string{"m"})
	tokPath := filepath.Join(dir, "tokens.yaml")
	writeTokens(t, tokPath, "gw-bifrost-down", "t1")
	routePath := filepath.Join(dir, "routing-policy.yaml")
	if err := os.WriteFile(routePath, []byte("rules: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rt, err := NewRuntime(gwPath, testLog())
	if err != nil {
		t.Fatal(err)
	}
	front := httptest.NewServer(NewMux(rt, testLog(), nil, NewUIOptions()))
	t.Cleanup(front.Close)

	jar, _ := cookiejar.New(nil)
	cli := &http.Client{Jar: jar}
	loginRes, err := cli.Post(front.URL+"/api/ui/login", "application/json", strings.NewReader(`{"token":"gw-bifrost-down"}`))
	if err != nil {
		t.Fatal(err)
	}
	loginRes.Body.Close()
	if loginRes.StatusCode != http.StatusOK {
		t.Fatalf("login %d", loginRes.StatusCode)
	}

	res, err := cli.Get(front.URL + "/api/ui/bifrost/providers")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("provider health %d %s", res.StatusCode, b)
	}
	var doc bifrostProviderHealthResponse
	if err := json.NewDecoder(res.Body).Decode(&doc); err != nil {
		t.Fatal(err)
	}
	if doc.BifrostUp {
		t.Fatalf("bifrost_up should be false: %+v", doc)
	}
	if doc.Error == "" {
		t.Fatalf("expected error annotation: %+v", doc)
	}
	for _, p := range doc.Providers {
		if p.State != "down" {
			t.Fatalf("provider %q state=%q want down", p.ID, p.State)
		}
	}
}

func TestUIBifrostProviderHealth_requiresAuth(t *testing.T) {
	t.Setenv("CLAUDIA_UPSTREAM_API_KEY", "ukey")
	bifrost := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	t.Cleanup(bifrost.Close)

	dir := t.TempDir()
	gwPath := filepath.Join(dir, "gateway.yaml")
	writeGateway(t, gwPath, bifrost.URL, []string{"m"})
	tokPath := filepath.Join(dir, "tokens.yaml")
	writeTokens(t, tokPath, "gw-bifrost-auth", "t1")
	routePath := filepath.Join(dir, "routing-policy.yaml")
	if err := os.WriteFile(routePath, []byte("rules: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rt, err := NewRuntime(gwPath, testLog())
	if err != nil {
		t.Fatal(err)
	}
	front := httptest.NewServer(NewMux(rt, testLog(), nil, NewUIOptions()))
	t.Cleanup(front.Close)

	res, err := http.Get(front.URL + "/api/ui/bifrost/providers")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("status %d (want 401) %s", res.StatusCode, b)
	}
}
