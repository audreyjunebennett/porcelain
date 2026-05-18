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

	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/adminui"
	"github.com/lynn/porcelain/internal/naming"
)

func TestUIBrokerProviderHealth_endToEnd(t *testing.T) {
	t.Setenv(naming.EnvBrokerAPIKeyTarget, "ukey")

	chimeraBroker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			w.WriteHeader(http.StatusOK)
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
	rt, err := NewRuntime(gwPath, testLog())
	if err != nil {
		t.Fatal(err)
	}
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

func TestUIBrokerProviderHealth_requiresAuth(t *testing.T) {
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
	rt, err := NewRuntime(gwPath, testLog())
	if err != nil {
		t.Fatal(err)
	}
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
