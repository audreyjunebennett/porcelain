package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/brokeradmin"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/adminui"
	"github.com/lynn/porcelain/internal/naming"
)

func TestUIBrokerProviderHealth_endToEnd(t *testing.T) {
	brokeradmin.InvalidateProviderConfigIndex()
	t.Setenv(naming.EnvBrokerAPIKeyTarget, "ukey")

	chimeraBroker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			w.WriteHeader(http.StatusOK)
		case "/v1/models":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"object":"list","data":[
				{"id":"groq/llama3","object":"model"},
				{"id":"ollama/qwen","object":"model"}
			]}`))
		case "/api/providers/groq":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"name":"groq","keys":[{"name":"chimera-groq-key-1","value":"env.GROQ_API_KEY"}]}`))
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
	t.Cleanup(chimeraBroker.Close)

	dir := t.TempDir()
	gwPath := filepath.Join(dir, naming.GatewayConfigFileTarget)
	writeGateway(t, gwPath, chimeraBroker.URL, []string{"m"}, "")
	tokPath := filepath.Join(dir, "api-keys.yaml")
	writeTokens(t, tokPath, "gw-chimera-broker-health", "t1")
	routePath := filepath.Join(dir, "routing-policy.yaml")
	if err := os.WriteFile(routePath, []byte("rules: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rt := mustRuntime(t, gwPath)
	front := httptest.NewServer(NewMux(rt, testLog(), nil, NewUIOptions()))
	t.Cleanup(front.Close)

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar}

	loginRes, err := client.Post(front.URL+"/api/ui/login", "application/json", strings.NewReader(`{"token":"gw-chimera-broker-health"}`))
	if err != nil {
		t.Fatal(err)
	}
	loginRes.Body.Close()
	if loginRes.StatusCode != http.StatusOK {
		t.Fatalf("login %d", loginRes.StatusCode)
	}

	res, err := client.Get(front.URL + "/api/ui/chimera-broker/providers")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("provider health %d %s", res.StatusCode, b)
	}
	var doc adminui.ProviderHealthResponse
	if err := json.NewDecoder(res.Body).Decode(&doc); err != nil {
		t.Fatal(err)
	}
	if !doc.BrokerUp {
		t.Fatalf("chimera_broker_up should be true: %+v", doc)
	}
	if doc.FetchedAt.IsZero() {
		t.Fatalf("fetched_at zero: %+v", doc)
	}
	if len(doc.Providers) != 3 {
		t.Fatalf("providers len=%d want 3: %+v", len(doc.Providers), doc.Providers)
	}
	byID := map[string]adminui.ProviderHealthEntry{}
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

func TestUIChimeraBrokerProviderHealth_chimeraBrokerDown(t *testing.T) {
	brokeradmin.InvalidateProviderConfigIndex()
	t.Setenv(naming.EnvBrokerAPIKeyTarget, "ukey")

	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	deadURL := dead.URL
	dead.Close()

	dir := t.TempDir()
	gwPath := filepath.Join(dir, naming.GatewayConfigFileTarget)
	writeGateway(t, gwPath, deadURL, []string{"m"}, "")
	tokPath := filepath.Join(dir, "api-keys.yaml")
	writeTokens(t, tokPath, "gw-chimera-broker-down", "t1")
	routePath := filepath.Join(dir, "routing-policy.yaml")
	if err := os.WriteFile(routePath, []byte("rules: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rt := mustRuntime(t, gwPath)
	front := httptest.NewServer(NewMux(rt, testLog(), nil, NewUIOptions()))
	t.Cleanup(front.Close)

	jar, _ := cookiejar.New(nil)
	cli := &http.Client{Jar: jar}
	loginRes, err := cli.Post(front.URL+"/api/ui/login", "application/json", strings.NewReader(`{"token":"gw-chimera-broker-down"}`))
	if err != nil {
		t.Fatal(err)
	}
	loginRes.Body.Close()
	if loginRes.StatusCode != http.StatusOK {
		t.Fatalf("login %d", loginRes.StatusCode)
	}

	res, err := cli.Get(front.URL + "/api/ui/chimera-broker/providers")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("provider health %d %s", res.StatusCode, b)
	}
	var doc adminui.ProviderHealthResponse
	if err := json.NewDecoder(res.Body).Decode(&doc); err != nil {
		t.Fatal(err)
	}
	if doc.BrokerUp {
		t.Fatalf("chimera_broker_up should be false: %+v", doc)
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

func TestUIBrokerProviderHealth_onlyOllamaConfigured_catalogFail(t *testing.T) {
	brokeradmin.InvalidateProviderConfigIndex()
	t.Setenv(naming.EnvBrokerAPIKeyTarget, "ukey")

	chimeraBroker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			w.WriteHeader(http.StatusOK)
		case "/api/governance/providers":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"providers":[{"provider":"ollama"}],"count":1}`))
		case "/v1/models":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"upstream unavailable"}`))
		case "/api/providers/ollama":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"name":"ollama","keys":[],"network_config":{"base_url":"http://127.0.0.1:11434"}}`))
		case "/api/providers/groq", "/api/providers/gemini":
			t.Errorf("unexpected GET %s when only ollama is configured", r.URL.Path)
			http.NotFound(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(chimeraBroker.Close)

	dir := t.TempDir()
	gwPath := filepath.Join(dir, naming.GatewayConfigFileTarget)
	writeGateway(t, gwPath, chimeraBroker.URL, []string{"m"}, "")
	tokPath := filepath.Join(dir, "api-keys.yaml")
	writeTokens(t, tokPath, "gw-ollama-only", "t1")
	routePath := filepath.Join(dir, "routing-policy.yaml")
	if err := os.WriteFile(routePath, []byte("rules: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rt := mustRuntime(t, gwPath)
	front := httptest.NewServer(NewMux(rt, testLog(), nil, NewUIOptions()))
	t.Cleanup(front.Close)

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginRes, err := client.Post(front.URL+"/api/ui/login", "application/json", strings.NewReader(`{"token":"gw-ollama-only"}`))
	if err != nil {
		t.Fatal(err)
	}
	loginRes.Body.Close()
	if loginRes.StatusCode != http.StatusOK {
		t.Fatalf("login %d", loginRes.StatusCode)
	}

	res, err := client.Get(front.URL + "/api/ui/chimera-broker/providers")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var doc adminui.ProviderHealthResponse
	if err := json.NewDecoder(res.Body).Decode(&doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Providers) != 1 {
		t.Fatalf("providers len=%d want 1 (configured only): %+v", len(doc.Providers), doc.Providers)
	}
	if doc.Providers[0].ID != "ollama" {
		t.Fatalf("ollama: %+v", doc.Providers[0])
	}
	if doc.Providers[0].State != "down" || doc.Providers[0].OllamaBaseURL != "http://127.0.0.1:11434" {
		t.Fatalf("ollama: %+v", doc.Providers[0])
	}

	stateRes, err := client.Get(front.URL + "/api/ui/state")
	if err != nil {
		t.Fatal(err)
	}
	defer stateRes.Body.Close()
	var stateDoc map[string]any
	if err := json.NewDecoder(stateRes.Body).Decode(&stateDoc); err != nil {
		t.Fatal(err)
	}
	prov, _ := stateDoc["providers"].(map[string]any)
	if len(prov) != 1 {
		t.Fatalf("state providers len=%d want 1: %+v", len(prov), prov)
	}
	if _, ok := prov["ollama"]; !ok {
		t.Fatalf("state missing ollama: %+v", prov)
	}
	if _, ok := prov["groq"]; ok {
		t.Fatalf("state must not include groq: %+v", prov)
	}
	cfgIDs, _ := stateDoc["configured_provider_ids"].([]any)
	if len(cfgIDs) != 1 || cfgIDs[0] != "ollama" {
		t.Fatalf("configured_provider_ids=%v", cfgIDs)
	}
}

func TestUIBrokerProviderHealth_requiresAuth(t *testing.T) {
	brokeradmin.InvalidateProviderConfigIndex()
	t.Setenv(naming.EnvBrokerAPIKeyTarget, "ukey")
	chimeraBroker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	t.Cleanup(chimeraBroker.Close)

	dir := t.TempDir()
	gwPath := filepath.Join(dir, naming.GatewayConfigFileTarget)
	writeGateway(t, gwPath, chimeraBroker.URL, []string{"m"}, "")
	tokPath := filepath.Join(dir, "api-keys.yaml")
	writeTokens(t, tokPath, "gw-chimera-broker-auth", "t1")
	routePath := filepath.Join(dir, "routing-policy.yaml")
	if err := os.WriteFile(routePath, []byte("rules: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rt := mustRuntime(t, gwPath)
	front := httptest.NewServer(NewMux(rt, testLog(), nil, NewUIOptions()))
	t.Cleanup(front.Close)

	res, err := http.Get(front.URL + "/api/ui/chimera-broker/providers")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("status %d (want 401) %s", res.StatusCode, b)
	}
}
