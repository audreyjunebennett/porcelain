package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/catalog"
	"github.com/lynn/porcelain/internal/naming"
)

func TestUIVirtualModelGenerate_filtersBySessionTenantAvailability(t *testing.T) {
	t.Setenv(naming.EnvBrokerAPIKeyTarget, "ukey")

	broker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			w.WriteHeader(http.StatusOK)
		case "/v1/models":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"object":"list","data":[
				{"id":"groq/free","object":"model"},
				{"id":"groq/paid","object":"model"}
			]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(broker.Close)

	dir := t.TempDir()
	gwPath := filepath.Join(dir, naming.GatewayConfigFileTarget)
	writeGateway(t, gwPath, broker.URL, []string{"groq/free"}, "")
	tokPath := filepath.Join(dir, naming.APIKeysFileTarget)
	writeTokens(t, tokPath, "gw-vm-gen", "tenant-a")
	routePath := filepath.Join(dir, naming.RoutingPolicyFileTarget)
	if err := os.WriteFile(routePath, []byte("rules: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	rt := mustRuntime(t, gwPath)
	rt.SetCatalogSnapshot(catalog.NewTestSnapshotWithModels(time.Now(), []string{"groq/free", "groq/paid"}))
	st := rt.OperatorStore()
	if st == nil {
		t.Fatal("operator store required")
	}
	if err := st.ReplaceProviderModelAvailability(context.Background(), "tenant-a", "groq", map[string]bool{
		"groq/free": true,
		"groq/paid": false,
	}); err != nil {
		t.Fatal(err)
	}
	if err := rt.ReloadProviderModelAvailability(context.Background()); err != nil {
		t.Fatal(err)
	}

	front := httptest.NewServer(NewMux(rt, testLog(), nil, NewUIOptions()))
	t.Cleanup(front.Close)
	client := vmTestLoginClient(t, front.URL, "gw-vm-gen")

	genRes, err := client.Post(front.URL+"/api/ui/virtual-models/1/routing/generate", "application/json", strings.NewReader(`{"save":false}`))
	if err != nil {
		t.Fatal(err)
	}
	defer genRes.Body.Close()
	if genRes.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(genRes.Body)
		t.Fatalf("generate status=%d body=%s", genRes.StatusCode, b)
	}
	var out struct {
		OK            bool     `json:"ok"`
		FallbackChain []string `json:"fallback_chain"`
		ModelsUsed    int      `json:"models_used"`
	}
	if err := json.NewDecoder(genRes.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if !out.OK || out.ModelsUsed != 1 || len(out.FallbackChain) != 1 || out.FallbackChain[0] != "groq/free" {
		t.Fatalf("expected only groq/free in generated chain: %+v", out)
	}
}

func TestUIVirtualModelGet_reportsFallbackUnavailable(t *testing.T) {
	t.Setenv(naming.EnvBrokerAPIKeyTarget, "ukey")

	broker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(broker.Close)

	dir := t.TempDir()
	gwPath := filepath.Join(dir, naming.GatewayConfigFileTarget)
	writeGateway(t, gwPath, broker.URL, []string{"groq/free", "groq/paid"}, "")
	tokPath := filepath.Join(dir, naming.APIKeysFileTarget)
	writeTokens(t, tokPath, "gw-vm-get", "tenant-a")
	routePath := filepath.Join(dir, naming.RoutingPolicyFileTarget)
	if err := os.WriteFile(routePath, []byte("rules: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	rt := mustRuntime(t, gwPath)
	st := rt.OperatorStore()
	if st == nil {
		t.Fatal("operator store required")
	}
	if err := st.SetVirtualModelFallback(context.Background(), "", 1, []string{"groq/free", "groq/paid"}); err != nil {
		t.Fatal(err)
	}
	if err := st.ReplaceProviderModelAvailability(context.Background(), "tenant-a", "groq", map[string]bool{
		"groq/free": true,
		"groq/paid": false,
	}); err != nil {
		t.Fatal(err)
	}
	if err := rt.ReloadProviderModelAvailability(context.Background()); err != nil {
		t.Fatal(err)
	}

	front := httptest.NewServer(NewMux(rt, testLog(), nil, NewUIOptions()))
	t.Cleanup(front.Close)
	client := vmTestLoginClient(t, front.URL, "gw-vm-get")

	getRes, err := client.Get(front.URL + "/api/ui/virtual-models/1")
	if err != nil {
		t.Fatal(err)
	}
	defer getRes.Body.Close()
	if getRes.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(getRes.Body)
		t.Fatalf("get status=%d body=%s", getRes.StatusCode, b)
	}
	var doc struct {
		FallbackUnavailable []string `json:"fallback_unavailable"`
	}
	if err := json.NewDecoder(getRes.Body).Decode(&doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.FallbackUnavailable) != 1 || doc.FallbackUnavailable[0] != "groq/paid" {
		t.Fatalf("fallback_unavailable: %+v", doc.FallbackUnavailable)
	}
}

func vmTestLoginClient(t *testing.T, baseURL, token string) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar}
	loginRes, err := client.Post(baseURL+"/api/ui/login", "application/json", strings.NewReader(`{"token":"`+token+`"}`))
	if err != nil {
		t.Fatal(err)
	}
	loginRes.Body.Close()
	if loginRes.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(loginRes.Body)
		t.Fatalf("login %d %s", loginRes.StatusCode, b)
	}
	return client
}
