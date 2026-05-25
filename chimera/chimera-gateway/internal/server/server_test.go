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

	"github.com/lynn/porcelain/internal/naming"
)

func TestStatusEndpoint(t *testing.T) {
	t.Setenv(naming.EnvBrokerAPIKeyTarget, "ukey")
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(up.Close)

	dir := t.TempDir()
	gwPath := filepath.Join(dir, naming.GatewayConfigFileTarget)
	writeGateway(t, gwPath, up.URL, []string{"m"}, "")
	tokPath := filepath.Join(dir, naming.APIKeysFileTarget)
	writeTokens(t, tokPath, "t", "x")
	routePath := filepath.Join(dir, naming.RoutingPolicyFileTarget)
	if err := os.WriteFile(routePath, []byte("rules: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rt, err := NewRuntime(gwPath, testLog())
	if err != nil {
		t.Fatal(err)
	}
	ov := &StatusOverlay{
		EffectiveListen: "127.0.0.1:3999",
		Supervisor: &SupervisorInfo{
			BrokerListen:          "127.0.0.1:8080",
			VectorstoreSupervised: false,
		},
	}
	srv := httptest.NewServer(NewMux(rt, testLog(), ov, nil))
	t.Cleanup(srv.Close)

	res, err := http.Get(srv.URL + "/status")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d", res.StatusCode)
	}
	var doc map[string]any
	if err := json.NewDecoder(res.Body).Decode(&doc); err != nil {
		t.Fatal(err)
	}
	sup, _ := doc["supervisor"].(map[string]any)
	if sup["active"] != true {
		t.Fatalf("supervisor: %+v", sup)
	}
	gw, _ := doc["gateway"].(map[string]any)
	if gw["listen"] != "127.0.0.1:3999" {
		t.Fatalf("gateway.listen: %+v", gw)
	}
}

func TestUILoginAndState(t *testing.T) {
	t.Setenv(naming.EnvBrokerAPIKeyTarget, "ukey")
	chimeraBroker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			w.WriteHeader(http.StatusOK)
		case "/api/providers/groq":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"name":"groq","keys":[{"value":{"value":"***"}}]}`))
		case "/api/providers/gemini":
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"not found"}`))
		case "/api/providers/ollama":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"name":"ollama","keys":[],"network_config":{"base_url":"http://localhost:11434"}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(chimeraBroker.Close)

	dir := t.TempDir()
	gwPath := filepath.Join(dir, naming.GatewayConfigFileTarget)
	writeGateway(t, gwPath, chimeraBroker.URL, []string{"m"}, "")
	tokPath := filepath.Join(dir, "api-keys.yaml")
	writeTokens(t, tokPath, "gw-ui-secret", "t1")
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

	loginBody := `{"token":"gw-ui-secret"}`
	lr, err := client.Post(front.URL+"/api/ui/login", "application/json", strings.NewReader(loginBody))
	if err != nil {
		t.Fatal(err)
	}
	defer lr.Body.Close()
	if lr.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(lr.Body)
		t.Fatalf("login status %d %s", lr.StatusCode, b)
	}

	sr, err := client.Get(front.URL + "/api/ui/state")
	if err != nil {
		t.Fatal(err)
	}
	defer sr.Body.Close()
	if sr.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(sr.Body)
		t.Fatalf("state status %d %s", sr.StatusCode, b)
	}
	var st map[string]any
	if err := json.NewDecoder(sr.Body).Decode(&st); err != nil {
		t.Fatal(err)
	}
	gw, _ := st["gateway"].(map[string]any)
	if gw["virtual_model_id"] != "Chimera-0.1.0" {
		t.Fatalf("gateway: %+v", gw)
	}
	ov, _ := gw["service_overview"].(map[string]any)
	if ov == nil {
		t.Fatalf("missing gateway.service_overview: %+v", gw)
	}
	if _, ok := ov["overall_state"].(string); !ok {
		t.Fatalf("missing service_overview.overall_state: %+v", ov)
	}
	if _, ok := ov["refreshed_at"].(string); !ok {
		t.Fatalf("missing service_overview.refreshed_at: %+v", ov)
	}
	prov, _ := st["providers"].(map[string]any)
	groq, _ := prov["groq"].(map[string]any)
	if groq["ok"] != true {
		t.Fatalf("groq: %+v", groq)
	}
	if groq["key_configured"] != true {
		t.Fatalf("groq: %+v", groq)
	}
	gk, _ := groq["keys"].([]any)
	if len(gk) != 1 {
		t.Fatalf("groq.keys: %+v", groq)
	}
	if _, ok := prov["gemini"]; ok {
		t.Fatalf("gemini should be omitted when not in broker config: %+v", prov)
	}
	oll, _ := prov["ollama"].(map[string]any)
	if oll["ollama_base_url"] != "http://localhost:11434" {
		t.Fatalf("ollama: %+v", oll)
	}

	bad, err := client.Post(front.URL+"/api/ui/login", "application/json", strings.NewReader(`{"token":"nope"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer bad.Body.Close()
	if bad.StatusCode != http.StatusUnauthorized {
		t.Fatalf("bad login %d", bad.StatusCode)
	}
}

func TestUISaveGroqKey(t *testing.T) {
	t.Setenv(naming.EnvBrokerAPIKeyTarget, "ukey")
	var lastPutMethod, lastPutPath string
	var lastPutBody []byte
	chimeraBroker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/health" && r.Method == http.MethodGet:
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/api/providers/groq" && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"name":"groq","keys":[{"id":"k1","name":"groq-default","weight":1,"value":{"value":"***"}}],"concurrency_and_buffer_size":{"concurrency":5,"buffer_size":10}}`))
		case r.URL.Path == "/api/providers/groq" && r.Method == http.MethodPut:
			lastPutMethod = r.Method
			lastPutPath = r.URL.Path
			lastPutBody, _ = io.ReadAll(r.Body)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(chimeraBroker.Close)

	dir := t.TempDir()
	gwPath := filepath.Join(dir, naming.GatewayConfigFileTarget)
	writeGateway(t, gwPath, chimeraBroker.URL, []string{"m"}, "")
	tokPath := filepath.Join(dir, "api-keys.yaml")
	writeTokens(t, tokPath, "gw-save", "t1")
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
	client := &http.Client{Jar: jar}
	loginRes, err := client.Post(front.URL+"/api/ui/login", "application/json", strings.NewReader(`{"token":"gw-save"}`))
	if err != nil {
		t.Fatal(err)
	}
	loginRes.Body.Close()
	if loginRes.StatusCode != http.StatusOK {
		t.Fatalf("login %d", loginRes.StatusCode)
	}

	saveRes, err := client.Post(front.URL+"/api/ui/provider/groq/keys", "application/json", strings.NewReader(`{"value":"new-groq-secret"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer saveRes.Body.Close()
	if saveRes.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(saveRes.Body)
		t.Fatalf("save %d %s", saveRes.StatusCode, b)
	}
	if lastPutMethod != http.MethodPut || lastPutPath != "/api/providers/groq" {
		t.Fatalf("chimera-broker got %s %s", lastPutMethod, lastPutPath)
	}
	var putDoc map[string]any
	if err := json.Unmarshal(lastPutBody, &putDoc); err != nil {
		t.Fatal(err)
	}
	keys := putDoc["keys"].([]any)
	if len(keys) != 2 {
		t.Fatalf("want 2 keys after append, got %d: %+v", len(keys), putDoc)
	}
	k1 := keys[1].(map[string]any)
	if k1["value"] != "new-groq-secret" {
		t.Fatalf("put body keys[1].value: %+v", putDoc)
	}
	if k1["name"] != "chimera-groq-key-1" {
		t.Fatalf("name: %+v", k1)
	}
}

func TestUISaveGroqKey_providerMissing404(t *testing.T) {
	t.Setenv(naming.EnvBrokerAPIKeyTarget, "ukey")
	var lastPutBody []byte
	chimeraBroker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/health" && r.Method == http.MethodGet:
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/api/providers/groq" && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"Provider not found"}`))
		case r.URL.Path == "/api/providers/groq" && r.Method == http.MethodPut:
			lastPutBody, _ = io.ReadAll(r.Body)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(chimeraBroker.Close)

	dir := t.TempDir()
	gwPath := filepath.Join(dir, naming.GatewayConfigFileTarget)
	writeGateway(t, gwPath, chimeraBroker.URL, []string{"m"}, "")
	tokPath := filepath.Join(dir, "api-keys.yaml")
	writeTokens(t, tokPath, "gw-save404", "t1")
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
	client := &http.Client{Jar: jar}
	loginRes, err := client.Post(front.URL+"/api/ui/login", "application/json", strings.NewReader(`{"token":"gw-save404"}`))
	if err != nil {
		t.Fatal(err)
	}
	loginRes.Body.Close()
	if loginRes.StatusCode != http.StatusOK {
		t.Fatalf("login %d", loginRes.StatusCode)
	}

	saveRes, err := client.Post(front.URL+"/api/ui/provider/groq/keys", "application/json", strings.NewReader(`{"value":"brand-new"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer saveRes.Body.Close()
	if saveRes.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(saveRes.Body)
		t.Fatalf("save %d %s", saveRes.StatusCode, b)
	}
	var putDoc map[string]any
	if err := json.Unmarshal(lastPutBody, &putDoc); err != nil {
		t.Fatal(err)
	}
	keys := putDoc["keys"].([]any)
	if len(keys) != 1 {
		t.Fatalf("want 1 key for fresh provider, got %d", len(keys))
	}
	k0 := keys[0].(map[string]any)
	if k0["value"] != "brand-new" {
		t.Fatalf("put body keys[0].value: %+v", putDoc)
	}
	if k0["name"] != "chimera-groq-key-1" {
		t.Fatalf("name: %+v", k0)
	}
}

func TestUISaveRemoveGroqKey(t *testing.T) {
	t.Setenv(naming.EnvBrokerAPIKeyTarget, "ukey")
	var lastPutBody []byte
	chimeraBroker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/health" && r.Method == http.MethodGet:
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/api/providers/groq" && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"name":"groq","keys":[
				{"name":"chimera-groq-key-1","value":"a","weight":1},
				{"name":"chimera-groq-key-2","value":"b","weight":1}
			]}`))
		case r.URL.Path == "/api/providers/groq" && r.Method == http.MethodPut:
			lastPutBody, _ = io.ReadAll(r.Body)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(chimeraBroker.Close)

	dir := t.TempDir()
	gwPath := filepath.Join(dir, naming.GatewayConfigFileTarget)
	writeGateway(t, gwPath, chimeraBroker.URL, []string{"m"}, "")
	tokPath := filepath.Join(dir, "api-keys.yaml")
	writeTokens(t, tokPath, "gw-rm", "t1")
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
	client := &http.Client{Jar: jar}
	loginRes, err := client.Post(front.URL+"/api/ui/login", "application/json", strings.NewReader(`{"token":"gw-rm"}`))
	if err != nil {
		t.Fatal(err)
	}
	loginRes.Body.Close()
	if loginRes.StatusCode != http.StatusOK {
		t.Fatalf("login %d", loginRes.StatusCode)
	}

	rmRes, err := client.Post(front.URL+"/api/ui/provider/groq/keys/delete", "application/json", strings.NewReader(`{"name":"chimera-groq-key-1"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer rmRes.Body.Close()
	if rmRes.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(rmRes.Body)
		t.Fatalf("remove %d %s", rmRes.StatusCode, b)
	}
	var putDoc map[string]any
	if err := json.Unmarshal(lastPutBody, &putDoc); err != nil {
		t.Fatal(err)
	}
	keys := putDoc["keys"].([]any)
	if len(keys) != 1 {
		t.Fatalf("want 1 key after remove, got %d", len(keys))
	}
	if keys[0].(map[string]any)["name"] != "chimera-groq-key-2" {
		t.Fatalf("%+v", keys[0])
	}
}

func TestUISaveOllamaURL_providerMissingEnvelope(t *testing.T) {
	t.Setenv(naming.EnvBrokerAPIKeyTarget, "ukey")
	var lastPutBody []byte
	envelope := `{"is_chimera_broker_error":false,"status_code":404,"error":{"message":"Provider not found: not found"},"extra_fields":{}}`
	chimeraBroker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/health" && r.Method == http.MethodGet:
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/api/providers/ollama" && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(envelope))
		case r.URL.Path == "/api/providers/ollama" && r.Method == http.MethodPut:
			lastPutBody, _ = io.ReadAll(r.Body)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(chimeraBroker.Close)

	dir := t.TempDir()
	gwPath := filepath.Join(dir, naming.GatewayConfigFileTarget)
	writeGateway(t, gwPath, chimeraBroker.URL, []string{"m"}, "")
	tokPath := filepath.Join(dir, "api-keys.yaml")
	writeTokens(t, tokPath, "gw-ollama-env", "t1")
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
	client := &http.Client{Jar: jar}
	loginRes, err := client.Post(front.URL+"/api/ui/login", "application/json", strings.NewReader(`{"token":"gw-ollama-env"}`))
	if err != nil {
		t.Fatal(err)
	}
	loginRes.Body.Close()
	if loginRes.StatusCode != http.StatusOK {
		t.Fatalf("login %d", loginRes.StatusCode)
	}

	saveRes, err := client.Post(front.URL+"/api/ui/provider/ollama/base_url", "application/json", strings.NewReader(`{"base_url":"http://localhost:11434"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer saveRes.Body.Close()
	if saveRes.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(saveRes.Body)
		t.Fatalf("save %d %s", saveRes.StatusCode, b)
	}
	var putDoc map[string]any
	if err := json.Unmarshal(lastPutBody, &putDoc); err != nil {
		t.Fatal(err)
	}
	nc := putDoc["network_config"].(map[string]any)
	if nc["base_url"] != "http://localhost:11434" {
		t.Fatalf("put network_config: %+v", putDoc)
	}
}

func TestChatVirtualModelFallback429(t *testing.T) {
	t.Setenv(naming.EnvBrokerAPIKeyTarget, "ukey")

	var seenModels []string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		case "/v1/chat/completions":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			m, _ := body["model"].(string)
			seenModels = append(seenModels, m)
			if m == "groq/a" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte(`{"error":"rate"}`))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":"ok"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(up.Close)

	dir := t.TempDir()
	gwPath := filepath.Join(dir, naming.GatewayConfigFileTarget)
	writeGateway(t, gwPath, up.URL, []string{"groq/a", "groq/b"}, "")
	tokPath := filepath.Join(dir, "api-keys.yaml")
	writeTokens(t, tokPath, "secret-gw", "t1")
	routePath := filepath.Join(dir, "routing-policy.yaml")
	writeRouting(t, routePath, "groq/a", 999999) // no rule match for short message → ambiguous or chain

	rt, err := NewRuntime(gwPath, testLog())
	if err != nil {
		t.Fatal(err)
	}
	h := NewMux(rt, testLog(), nil, nil)
	front := httptest.NewServer(h)
	t.Cleanup(front.Close)

	reqBody := `{"model":"Chimera-0.1.0","messages":[{"role":"user","content":"hi"}],"stream":false}`
	req, _ := http.NewRequest(http.MethodPost, front.URL+"/v1/chat/completions", strings.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer secret-gw")
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("status %d body %s", res.StatusCode, b)
	}
	if len(seenModels) < 2 {
		t.Fatalf("expected retry, seenModels=%v", seenModels)
	}
	if seenModels[0] != "groq/a" || seenModels[1] != "groq/b" {
		t.Fatalf("order: %v", seenModels)
	}
}
