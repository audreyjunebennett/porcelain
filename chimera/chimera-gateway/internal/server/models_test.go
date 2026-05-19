package server

import (
	"encoding/json"
	"github.com/lynn/porcelain/internal/naming"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestModelsList_VirtualModelFirst(t *testing.T) {
	t.Setenv(naming.EnvBrokerAPIKeyTarget, "ukey")

	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path == "/v1/models" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"groq/x","object":"model","created":1,"owned_by":"groq"}]}`))
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(up.Close)

	dir := t.TempDir()
	gwPath := filepath.Join(dir, naming.GatewayConfigFileTarget)
	writeGateway(t, gwPath, up.URL, []string{"groq/x"}, "")
	tokPath := filepath.Join(dir, "api-keys.yaml")
	writeTokens(t, tokPath, "tok", "t1")
	routePath := filepath.Join(dir, "routing-policy.yaml")
	if err := os.WriteFile(routePath, []byte("rules: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	rt, err := NewRuntime(gwPath, testLog())
	if err != nil {
		t.Fatal(err)
	}
	front := httptest.NewServer(NewMux(rt, testLog(), nil, nil))
	t.Cleanup(front.Close)

	req, _ := http.NewRequest(http.MethodGet, front.URL+"/v1/models", nil)
	req.Header.Set("Authorization", "Bearer tok")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("%d %s", res.StatusCode, b)
	}
	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Data) < 2 {
		t.Fatalf("want virtual + upstream, got %#v", payload.Data)
	}
	if payload.Data[0].ID != "Chimera-0.1.0" {
		t.Fatalf("virtual first: %q", payload.Data[0].ID)
	}
	if payload.Data[1].ID != "groq/x" {
		t.Fatalf("upstream second: %q", payload.Data[1].ID)
	}
}

func TestModelsList_NormalizesMissingOpenAIFields(t *testing.T) {
	t.Setenv(naming.EnvBrokerAPIKeyTarget, "ukey")

	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path == "/v1/models" {
			w.Header().Set("Content-Type", "application/json")
			// Like BiFrost: id + extra fields, no object/created
			_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"gemini/x","owned_by":"google","context_length":4096}]}`))
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(up.Close)

	dir := t.TempDir()
	gwPath := filepath.Join(dir, naming.GatewayConfigFileTarget)
	writeGateway(t, gwPath, up.URL, []string{"gemini/x"}, "")
	tokPath := filepath.Join(dir, "api-keys.yaml")
	writeTokens(t, tokPath, "tok", "t1")
	routePath := filepath.Join(dir, "routing-policy.yaml")
	if err := os.WriteFile(routePath, []byte("rules: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	rt, err := NewRuntime(gwPath, testLog())
	if err != nil {
		t.Fatal(err)
	}
	front := httptest.NewServer(NewMux(rt, testLog(), nil, nil))
	t.Cleanup(front.Close)

	req, _ := http.NewRequest(http.MethodGet, front.URL+"/v1/models", nil)
	req.Header.Set("Authorization", "Bearer tok")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("%d %s", res.StatusCode, b)
	}
	var payload map[string]any
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	arr, _ := payload["data"].([]any)
	if len(arr) < 2 {
		t.Fatalf("data: %v", arr)
	}
	m, _ := arr[1].(map[string]any)
	if m["object"] != "model" {
		t.Fatalf("object: %v", m["object"])
	}
	if _, ok := m["created"]; !ok {
		t.Fatal("missing created")
	}
}

func TestUIModels_NoGatewayToken(t *testing.T) {
	t.Setenv(naming.EnvBrokerAPIKeyTarget, "ukey")

	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path == "/v1/models" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"ollama/qwen","object":"model","created":1,"owned_by":"ollama"}]}`))
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(up.Close)

	dir := t.TempDir()
	gwPath := filepath.Join(dir, naming.GatewayConfigFileTarget)
	writeGateway(t, gwPath, up.URL, []string{"ollama/qwen"}, "")
	tokPath := filepath.Join(dir, "api-keys.yaml")
	writeTokens(t, tokPath, "tok", "t1")
	routePath := filepath.Join(dir, "routing-policy.yaml")
	if err := os.WriteFile(routePath, []byte("rules: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	rt, err := NewRuntime(gwPath, testLog())
	if err != nil {
		t.Fatal(err)
	}
	front := httptest.NewServer(NewMux(rt, testLog(), nil, nil))
	t.Cleanup(front.Close)

	req, _ := http.NewRequest(http.MethodGet, front.URL+"/ui/models", nil)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("status %d: %s", res.StatusCode, b)
	}
	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Data) < 2 {
		t.Fatalf("want virtual + upstream, got %#v", payload.Data)
	}
	if payload.Data[0].ID != "Chimera-0.1.0" {
		t.Fatalf("virtual first: %q", payload.Data[0].ID)
	}
	if payload.Data[1].ID != "ollama/qwen" {
		t.Fatalf("upstream second: %q", payload.Data[1].ID)
	}
}

func TestModelsList_FreeTierFilter(t *testing.T) {
	t.Setenv(naming.EnvBrokerAPIKeyTarget, "ukey")

	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path == "/v1/models" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"object":"list","data":[
				{"id":"groq/x","object":"model","created":1},
				{"id":"groq/y","object":"model","created":1}
			]}`))
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(up.Close)

	dir := t.TempDir()
	gwPath := filepath.Join(dir, naming.GatewayConfigFileTarget)
	gwRaw := "gateway:\n  semver: \"0.1.0\"\n  listen_port: 0\n  listen_host: \"127.0.0.1\"\n" +
		"broker:\n  base_url: \"" + up.URL + "\"\n  api_key_env: \"" + naming.EnvBrokerAPIKeyTarget + "\"\n" +
		"health:\n  timeout_ms: 2000\n  chat_timeout_ms: 60000\n" +
		"paths:\n  tokens: \"./api-keys.yaml\"\n  routing_policy: \"./routing-policy.yaml\"\n" +
		"  provider_free_tier: \"./provider-free-tier.yaml\"\n" +
		"routing:\n  filter_free_tier_models: true\n  fallback_chain:\n    - \"groq/x\"\n"
	if err := os.WriteFile(gwPath, []byte(gwRaw), 0o644); err != nil {
		t.Fatal(err)
	}
	ftPath := filepath.Join(dir, "provider-free-tier.yaml")
	ftRaw := "format_version: 1\neffective_date: \"2026-01-01\"\nmodels:\n  - groq/x\n"
	if err := os.WriteFile(ftPath, []byte(ftRaw), 0o644); err != nil {
		t.Fatal(err)
	}
	tokPath := filepath.Join(dir, "api-keys.yaml")
	writeTokens(t, tokPath, "tok", "t1")
	routePath := filepath.Join(dir, "routing-policy.yaml")
	if err := os.WriteFile(routePath, []byte("rules: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	rt, err := NewRuntime(gwPath, testLog())
	if err != nil {
		t.Fatal(err)
	}
	front := httptest.NewServer(NewMux(rt, testLog(), nil, nil))
	t.Cleanup(front.Close)

	req, _ := http.NewRequest(http.MethodGet, front.URL+"/v1/models", nil)
	req.Header.Set("Authorization", "Bearer tok")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("status %d: %s", res.StatusCode, b)
	}
	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Data) != 2 {
		t.Fatalf("want virtual + groq/x only, got %#v", payload.Data)
	}
	if payload.Data[0].ID != "Chimera-0.1.0" || payload.Data[1].ID != "groq/x" {
		t.Fatalf("ids: %#v", payload.Data)
	}
}
