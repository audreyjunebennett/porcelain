package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/operatorstore"
	"github.com/lynn/porcelain/internal/naming"
)

func TestVirtualModels_twoModelsDifferentRouting(t *testing.T) {
	t.Setenv(naming.EnvBrokerAPIKeyTarget, "ukey")
	var seenModel string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			w.WriteHeader(http.StatusOK)
		case "/v1/models":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"object":"list","data":[
				{"id":"groq/fast","object":"model","created":1},
				{"id":"groq/slow","object":"model","created":1},
				{"id":"gemini/flash","object":"model","created":1}
			]}`))
		case "/v1/chat/completions":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			seenModel, _ = body["model"].(string)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"x","object":"chat.completion","choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(up.Close)

	dir := t.TempDir()
	gwPath := filepath.Join(dir, naming.GatewayConfigFileTarget)
	writeGateway(t, gwPath, up.URL, []string{"groq/fast", "groq/slow"}, "")
	tokPath := filepath.Join(dir, naming.APIKeysFileTarget)
	writeTokens(t, tokPath, "tok", "tenant")
	routePath := filepath.Join(dir, naming.RoutingPolicyFileTarget)
	writeRouting(t, routePath, "groq/fast", 99999)

	rt := mustRuntime(t, gwPath)
	st := rt.OperatorStore()
	if st == nil {
		t.Fatal("operator store required")
	}
	ctx := context.Background()
	gemini := operatorstore.Gemini010Seed([]string{"gemini/flash"})
	if _, err := st.InsertVirtualModelFull(ctx, gemini); err != nil {
		t.Fatal(err)
	}
	if err := rt.ReloadVirtualModels(ctx); err != nil {
		t.Fatal(err)
	}

	front := httptest.NewServer(NewMux(rt, testLog(), nil, nil))
	t.Cleanup(front.Close)

	chatBody := `{"model":"Gemini-0.1.0","messages":[{"role":"user","content":"hello"}]}`
	req, _ := http.NewRequest(http.MethodPost, front.URL+"/v1/chat/completions", strings.NewReader(chatBody))
	req.Header.Set("Authorization", "Bearer tok")
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("gemini chat status=%d body=%s", res.StatusCode, b)
	}
	if seenModel != "gemini/flash" {
		t.Fatalf("gemini routed to %q", seenModel)
	}

	seenModel = ""
	chatBody2 := `{"model":"Chimera-0.1.0","messages":[{"role":"user","content":"hello"}]}`
	req2, _ := http.NewRequest(http.MethodPost, front.URL+"/v1/chat/completions", strings.NewReader(chatBody2))
	req2.Header.Set("Authorization", "Bearer tok")
	req2.Header.Set("Content-Type", "application/json")
	res2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	defer res2.Body.Close()
	if res2.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res2.Body)
		t.Fatalf("chimera chat status=%d body=%s", res2.StatusCode, b)
	}
	if seenModel != "groq/fast" {
		t.Fatalf("chimera routed to %q want groq/fast", seenModel)
	}
}

func TestVirtualModels_listIncludesBootstrapped(t *testing.T) {
	t.Setenv(naming.EnvBrokerAPIKeyTarget, "ukey")
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path == "/v1/models" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"groq/x","object":"model","created":1}]}`))
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(up.Close)

	dir := t.TempDir()
	gwPath := filepath.Join(dir, naming.GatewayConfigFileTarget)
	writeGateway(t, gwPath, up.URL, []string{"groq/x"}, "")
	tokPath := filepath.Join(dir, naming.APIKeysFileTarget)
	writeTokens(t, tokPath, "tok", "t1")
	routePath := filepath.Join(dir, naming.RoutingPolicyFileTarget)
	if err := os.WriteFile(routePath, []byte("ambiguous_default_model: groq/x\nrules: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rt := mustRuntime(t, gwPath)
	if rt.VirtualModels() == nil {
		t.Fatal("expected virtual model registry")
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
	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Data) < 2 || payload.Data[0].ID != "Chimera-0.1.0" {
		t.Fatalf("models=%+v", payload.Data)
	}
}

func TestVirtualModels_disabledRejected(t *testing.T) {
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
	writeGateway(t, gwPath, up.URL, []string{"groq/x"}, "")
	tokPath := filepath.Join(dir, naming.APIKeysFileTarget)
	writeTokens(t, tokPath, "tok", "t1")
	routePath := filepath.Join(dir, naming.RoutingPolicyFileTarget)
	if err := os.WriteFile(routePath, []byte("rules: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rt := mustRuntime(t, gwPath)
	st := rt.OperatorStore()
	disabled := false
	if err := st.UpdateVirtualModelMetadata(context.Background(), "", 1, "", "", "", &disabled, nil); err != nil {
		t.Fatal(err)
	}
	_ = rt.ReloadVirtualModels(context.Background())

	front := httptest.NewServer(NewMux(rt, testLog(), nil, nil))
	t.Cleanup(front.Close)
	body := bytes.NewBufferString(`{"model":"Chimera-0.1.0","messages":[{"role":"user","content":"hi"}]}`)
	req, _ := http.NewRequest(http.MethodPost, front.URL+"/v1/chat/completions", body)
	req.Header.Set("Authorization", "Bearer tok")
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("status=%d", res.StatusCode)
	}
}
