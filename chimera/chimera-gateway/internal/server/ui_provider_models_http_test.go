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
	"time"

	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/brokeradmin"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/catalog"
	"github.com/lynn/porcelain/internal/naming"
	"github.com/lynn/porcelain/internal/operatorapi"
)

func TestUIProviderModels_endToEnd(t *testing.T) {
	brokeradmin.InvalidateProviderConfigIndex()
	t.Setenv(naming.EnvBrokerAPIKeyTarget, "ukey")

	chimeraBroker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			w.WriteHeader(http.StatusOK)
		case "/v1/models":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"object":"list","data":[
				{"id":"groq/free","object":"model"},
				{"id":"groq/paid","object":"model"}
			]}`))
		case "/api/providers/groq":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"name":"groq","keys":[{"name":"k","value":"env.GROQ_API_KEY"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(chimeraBroker.Close)

	dir := t.TempDir()
	gwPath := filepath.Join(dir, naming.GatewayConfigFileTarget)
	writeGateway(t, gwPath, chimeraBroker.URL, []string{"groq/free"}, "")
	ftPath := filepath.Join(dir, "provider-free-tier.yaml")
	if err := os.WriteFile(ftPath, []byte("format_version: 1\nmodels:\n  - groq/free\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tokPath := filepath.Join(dir, "api-keys.yaml")
	if err := os.WriteFile(tokPath, []byte(`api_keys:
  - secret: "gw-provider-models"
    tenant_id: "tenant-a"
  - secret: "gw-provider-models-b"
    tenant_id: "tenant-b"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	appendGatewayProviderFreeTierPath(t, gwPath, "./provider-free-tier.yaml")
	routePath := filepath.Join(dir, "routing-policy.yaml")
	if err := os.WriteFile(routePath, []byte("rules: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	rt := mustRuntime(t, gwPath)
	rt.SetCatalogSnapshot(catalog.NewTestSnapshotWithModels(time.Now(), []string{"groq/free", "groq/paid"}))
	front := httptest.NewServer(NewMux(rt, testLog(), nil, NewUIOptions()))
	t.Cleanup(front.Close)

	clientA := uiLoginClient(t, front.URL, "gw-provider-models")
	clientB := uiLoginClient(t, front.URL, "gw-provider-models-b")

	putReq, err := http.NewRequest(http.MethodPut, front.URL+"/api/ui/providers/groq/models", strings.NewReader(`{"models":{"groq/free":true,"groq/paid":false}}`))
	if err != nil {
		t.Fatal(err)
	}
	putReq.Header.Set("Content-Type", "application/json")
	putRes, err := clientA.Do(putReq)
	if err != nil {
		t.Fatal(err)
	}
	putRes.Body.Close()
	if putRes.StatusCode != http.StatusOK {
		t.Fatalf("put status=%d", putRes.StatusCode)
	}

	getRes, err := clientA.Get(front.URL + "/api/ui/providers/groq/models")
	if err != nil {
		t.Fatal(err)
	}
	defer getRes.Body.Close()
	var doc operatorapi.ProviderModelsResponse
	if err := json.NewDecoder(getRes.Body).Decode(&doc); err != nil {
		t.Fatal(err)
	}
	if doc.TenantID != "tenant-a" || !doc.ModelsConfigured {
		t.Fatalf("tenant-a get: %+v", doc)
	}
	if doc.ModelsAvailableCount != 1 || doc.ModelsUnavailableCount != 1 {
		t.Fatalf("counts: %+v", doc)
	}

	getResB, err := clientB.Get(front.URL + "/api/ui/providers/groq/models")
	if err != nil {
		t.Fatal(err)
	}
	defer getResB.Body.Close()
	var docB operatorapi.ProviderModelsResponse
	if err := json.NewDecoder(getResB.Body).Decode(&docB); err != nil {
		t.Fatal(err)
	}
	if docB.ModelsConfigured {
		t.Fatalf("tenant-b should have no saved config: %+v", docB)
	}
	if docB.ModelsAvailableCount != 2 {
		t.Fatalf("tenant-b defaults available: %+v", docB)
	}

	applyRes, err := clientA.Post(front.URL+"/api/ui/providers/groq/models/apply-free-tier", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	defer applyRes.Body.Close()
	var draft operatorapi.ProviderModelsApplyFreeTierResponse
	if err := json.NewDecoder(applyRes.Body).Decode(&draft); err != nil {
		t.Fatal(err)
	}
	if !draft.OK || draft.ModelsAvailableCount != 1 || draft.ModelsUnavailableCount != 1 {
		t.Fatalf("apply-free-tier draft: %+v", draft)
	}

	getRes2, err := clientA.Get(front.URL + "/api/ui/providers/groq/models")
	if err != nil {
		t.Fatal(err)
	}
	defer getRes2.Body.Close()
	var after operatorapi.ProviderModelsResponse
	if err := json.NewDecoder(getRes2.Body).Decode(&after); err != nil {
		t.Fatal(err)
	}
	if after.ModelsUnavailableCount != 1 {
		t.Fatalf("apply-free-tier should not persist: %+v", after)
	}

	stateRes, err := clientA.Get(front.URL + "/api/ui/state")
	if err != nil {
		t.Fatal(err)
	}
	defer stateRes.Body.Close()
	var state operatorapi.StateResponse
	if err := json.NewDecoder(stateRes.Body).Decode(&state); err != nil {
		t.Fatal(err)
	}
	groq := state.Providers["groq"]
	if !groq.ModelsConfigured || groq.ModelsAvailableCount != 1 || groq.ModelsUnavailableCount != 1 {
		t.Fatalf("state providers.groq: %+v", groq)
	}

	badRes, err := clientA.Get(front.URL + "/api/ui/providers/unknown/models")
	if err != nil {
		t.Fatal(err)
	}
	badRes.Body.Close()
	if badRes.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown provider status=%d", badRes.StatusCode)
	}
}

func TestUIProviderModels_ollamaApplyFreeTierNoOp(t *testing.T) {
	brokeradmin.InvalidateProviderConfigIndex()
	t.Setenv(naming.EnvBrokerAPIKeyTarget, "ukey")

	chimeraBroker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			w.WriteHeader(http.StatusOK)
		case "/v1/models":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"ollama/llama3","object":"model"}]}`))
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
	writeGateway(t, gwPath, chimeraBroker.URL, []string{"ollama/llama3"}, "")
	tokPath := filepath.Join(dir, "api-keys.yaml")
	writeTokens(t, tokPath, "gw-ollama-models", "tenant-a")
	routePath := filepath.Join(dir, "routing-policy.yaml")
	if err := os.WriteFile(routePath, []byte("rules: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	rt := mustRuntime(t, gwPath)
	rt.SetCatalogSnapshot(catalog.NewTestSnapshotWithModels(time.Now(), []string{"ollama/llama3"}))
	front := httptest.NewServer(NewMux(rt, testLog(), nil, NewUIOptions()))
	t.Cleanup(front.Close)
	client := uiLoginClient(t, front.URL, "gw-ollama-models")

	applyRes, err := client.Post(front.URL+"/api/ui/providers/ollama/models/apply-free-tier", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	defer applyRes.Body.Close()
	var draft operatorapi.ProviderModelsApplyFreeTierResponse
	if err := json.NewDecoder(applyRes.Body).Decode(&draft); err != nil {
		t.Fatal(err)
	}
	if draft.Note == "" || draft.ModelsAvailableCount != 1 {
		t.Fatalf("ollama no-op: %+v", draft)
	}
}

func uiLoginClient(t *testing.T, baseURL, token string) *http.Client {
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

func appendGatewayProviderFreeTierPath(t *testing.T, gwPath, relPath string) {
	t.Helper()
	raw, err := os.ReadFile(gwPath)
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	if strings.Contains(s, "provider_free_tier:") {
		return
	}
	s = strings.Replace(s, "  routing_policy:", "  provider_free_tier: \""+relPath+"\"\n  routing_policy:", 1)
	if err := os.WriteFile(gwPath, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}
