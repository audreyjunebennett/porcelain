package server

import (
	"encoding/json"
	"github.com/lynn/porcelain/internal/naming"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lynn/porcelain/chimera/internal/config"
)

func TestUIRoutingGenerate_writesFiles(t *testing.T) {
	t.Setenv(naming.EnvBrokerAPIKeyTarget, "ukey")

	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path == "/v1/models" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"object":"list","data":[
				{"id":"groq/llama-3.3-70b-versatile"},
				{"id":"groq/llama-3.1-8b-instant"}
			]}`))
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(up.Close)

	dir := t.TempDir()
	gwPath := filepath.Join(dir, naming.GatewayConfigFileTarget)
	gwRaw := "gateway:\n  semver: \"0.1.0\"\n  listen_port: 0\n  listen_host: \"127.0.0.1\"\n" +
		"upstream:\n  base_url: \"" + up.URL + "\"\n  api_key_env: \"" + naming.EnvBrokerAPIKeyTarget + "\"\n" +
		"health:\n  timeout_ms: 2000\n  chat_timeout_ms: 60000\n" +
		"paths:\n  tokens: \"./api-keys.yaml\"\n  routing_policy: \"./routing-policy.yaml\"\n" +
		"routing:\n  filter_free_tier_models: false\n  fallback_chain:\n    - \"groq/legacy\"\n"
	if err := os.WriteFile(gwPath, []byte(gwRaw), 0o644); err != nil {
		t.Fatal(err)
	}
	tokPath := filepath.Join(dir, "api-keys.yaml")
	writeTokens(t, tokPath, "gw-rg", "t1")
	routePath := filepath.Join(dir, "routing-policy.yaml")
	if err := os.WriteFile(routePath, []byte("ambiguous_default_model: \"groq/legacy\"\nrules:\n  - name: d\n    when: {}\n    models:\n      - \"groq/legacy\"\n"), 0o644); err != nil {
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
	loginRes, err := client.Post(front.URL+"/api/ui/login", "application/json", strings.NewReader(`{"token":"gw-rg"}`))
	if err != nil {
		t.Fatal(err)
	}
	_ = loginRes.Body.Close()
	if loginRes.StatusCode != http.StatusOK {
		t.Fatalf("login %d", loginRes.StatusCode)
	}

	genRes, err := client.Post(front.URL+"/api/ui/routing/generate", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	defer genRes.Body.Close()
	if genRes.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(genRes.Body)
		t.Fatalf("generate %d: %s", genRes.StatusCode, b)
	}
	var out struct {
		OK     bool     `json:"ok"`
		Saved  bool     `json:"saved"`
		Chain  []string `json:"fallback_chain"`
		YAML   string   `json:"routing_policy_yaml"`
		Models int      `json:"models_used"`
	}
	if err := json.NewDecoder(genRes.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if !out.OK || !out.Saved || len(out.Chain) != 2 || out.Models != 2 || out.YAML == "" {
		t.Fatalf("%+v", out)
	}
	if out.Chain[0] != "groq/llama-3.3-70b-versatile" {
		t.Fatalf("expected 70b first, got %v", out.Chain)
	}

	res2, err := config.LoadGatewayYAML(gwPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res2.FallbackChain) != 2 || res2.FallbackChain[0] != out.Chain[0] {
		t.Fatalf("gateway reload: %#v", res2.FallbackChain)
	}
	if len(res2.RouterModels) < 1 || res2.RouterModels[0] != "groq/llama-3.1-8b-instant" {
		t.Fatalf("expected co-generated router_models to prefer small/fast first, got %#v", res2.RouterModels)
	}
	rp, err := os.ReadFile(routePath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(rp), "long-user-turn") {
		t.Fatalf("routing policy missing generated rule")
	}
}

func TestUIRoutingPolicySave_writesRoutingPolicyOnly(t *testing.T) {
	t.Setenv(naming.EnvBrokerAPIKeyTarget, "ukey")

	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path == "/v1/models" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"groq/x"}]}`))
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(up.Close)

	dir := t.TempDir()
	gwPath := filepath.Join(dir, naming.GatewayConfigFileTarget)
	gwRaw := "gateway:\n  semver: \"0.1.0\"\n  listen_port: 0\n  listen_host: \"127.0.0.1\"\n" +
		"upstream:\n  base_url: \"" + up.URL + "\"\n  api_key_env: \"" + naming.EnvBrokerAPIKeyTarget + "\"\n" +
		"health:\n  timeout_ms: 2000\n  chat_timeout_ms: 60000\n" +
		"paths:\n  tokens: \"./api-keys.yaml\"\n  routing_policy: \"./routing-policy.yaml\"\n" +
		"routing:\n  filter_free_tier_models: false\n  fallback_chain:\n    - \"groq/x\"\n"
	if err := os.WriteFile(gwPath, []byte(gwRaw), 0o644); err != nil {
		t.Fatal(err)
	}
	tokPath := filepath.Join(dir, "api-keys.yaml")
	writeTokens(t, tokPath, "gw-pol", "t1")
	routePath := filepath.Join(dir, "routing-policy.yaml")
	initial := "ambiguous_default_model: \"groq/x\"\nrules:\n  - name: a\n    when: {}\n    models:\n      - \"groq/x\"\n"
	if err := os.WriteFile(routePath, []byte(initial), 0o644); err != nil {
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
	loginRes, err := client.Post(front.URL+"/api/ui/login", "application/json", strings.NewReader(`{"token":"gw-pol"}`))
	if err != nil {
		t.Fatal(err)
	}
	_ = loginRes.Body.Close()
	if loginRes.StatusCode != http.StatusOK {
		t.Fatalf("login %d", loginRes.StatusCode)
	}

	newYAML := "ambiguous_default_model: \"groq/x\"\nrules:\n  - name: b\n    when: {}\n    models:\n      - \"groq/x\"\n"
	bodyJSON, err := json.Marshal(map[string]string{"routing_policy_yaml": newYAML})
	if err != nil {
		t.Fatal(err)
	}
	polRes, err := client.Post(front.URL+"/api/ui/routing/policy", "application/json", strings.NewReader(string(bodyJSON)))
	if err != nil {
		t.Fatal(err)
	}
	defer polRes.Body.Close()
	if polRes.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(polRes.Body)
		t.Fatalf("policy save %d: %s", polRes.StatusCode, b)
	}
	rb, err := os.ReadFile(routePath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(rb), "name: b") {
		t.Fatalf("expected updated rule name in file, got:\n%s", rb)
	}
}
