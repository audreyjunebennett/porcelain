package rag_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	uirag "github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/adminui/api/rag"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/adminui/handler"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/adminui/session"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/catalog"
	gruntime "github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/runtime"
	"github.com/lynn/porcelain/internal/naming"
	"github.com/lynn/porcelain/internal/operatorapi"
)

func testRAGEmbeddingEnv(t *testing.T, embedModel string) (*http.ServeMux, *handler.Handler, *gruntime.Runtime, string, *httptest.Server) {
	t.Helper()
	dir := t.TempDir()
	gwPath := filepath.Join(dir, naming.GatewayConfigFileTarget)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/health":
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/v1/embeddings" && r.Method == http.MethodPost:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":[{"index":0,"embedding":[0.1,0.2,0.3,0.4,0.5,0.6,0.7,0.8]}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(upstream.Close)

	raw := "gateway:\n  semver: \"0.2.0\"\n  listen_port: 0\n  listen_host: \"127.0.0.1\"\n" +
		"broker:\n  base_url: \"" + upstream.URL + "\"\n  api_key_env: \"" + naming.EnvBrokerAPIKeyTarget + "\"\n" +
		"health:\n  timeout_ms: 2000\n  chat_timeout_ms: 60000\n" +
		"paths:\n  tokens: \"./" + naming.APIKeysFileTarget + "\"\n  routing_policy: \"./" + naming.RoutingPolicyFileTarget + "\"\n" +
		"routing:\n  fallback_chain:\n    - \"m\"\n" +
		"vectorstore:\n  url: \"http://127.0.0.1:6333\"\n" +
		"rag:\n  enabled: true\n  embedding:\n    model: \"" + embedModel + "\"\n    dim: 8\n" +
		"  chunking:\n    size: 128\n    overlap: 32\n  ingest:\n    max_bytes: 10485760\n  defaults:\n    project_id: \"default\"\n"
	if err := os.WriteFile(gwPath, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	tokPath := filepath.Join(dir, naming.APIKeysFileTarget)
	if err := os.WriteFile(tokPath, []byte("api_keys:\n  - secret: \"embed-ui-tok\"\n    tenant_id: \"tenantE\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	routePath := filepath.Join(dir, naming.RoutingPolicyFileTarget)
	if err := os.WriteFile(routePath, []byte("rules: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv(naming.EnvBrokerAPIKeyTarget, "ukey")

	rt, err := gruntime.NewRuntime(gwPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { rt.CloseOperator(); rt.CloseMetrics() })

	rt.SetCatalogSnapshot(catalog.NewTestSnapshotWithModels(time.Now(), []string{
		"test-embed",
		"groq/llama3",
		"ollama/nomic-embed-text:latest",
		"groq/unknown-chat",
	}))

	ui := session.NewUIOptions()
	h := handler.New(rt, nil, ui)
	mux := http.NewServeMux()
	uirag.Register(mux, h)
	sid, err := ui.Sessions.Issue("tenantE")
	if err != nil {
		t.Fatal(err)
	}
	return mux, h, rt, sid, upstream
}

func embeddingRequest(t *testing.T, mux *http.ServeMux, method string, body any, cookieName, sid string) *httptest.ResponseRecorder {
	t.Helper()
	var rdr *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		rdr = bytes.NewReader(b)
	} else {
		rdr = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, "/api/ui/rag/embedding", rdr)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if sid != "" {
		req.AddCookie(&http.Cookie{Name: cookieName, Value: sid})
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func TestRAGEmbeddingGET_listsCandidatesWithLikelyFirst(t *testing.T) {
	mux, h, _, sid, _ := testRAGEmbeddingEnv(t, "test-embed")
	rec := embeddingRequest(t, mux, http.MethodGet, nil, h.CookieName(), sid)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var resp operatorapi.RAGEmbeddingGetResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Model != "test-embed" || resp.Dim != 8 || resp.Status != "ok" || !resp.ModelInCatalog {
		t.Fatalf("resp=%+v", resp)
	}
	if len(resp.Candidates) != 4 {
		t.Fatalf("candidates=%+v", resp.Candidates)
	}
	if !resp.Candidates[0].EmbeddingLikely {
		t.Fatalf("expected embedding-likely first, got %+v", resp.Candidates[0])
	}
	if resp.Candidates[0].ID != "ollama/nomic-embed-text:latest" {
		t.Fatalf("likely order: %+v", resp.Candidates)
	}
}

func TestRAGEmbeddingGET_notInCatalog(t *testing.T) {
	mux, h, _, sid, _ := testRAGEmbeddingEnv(t, "missing-model")
	rec := embeddingRequest(t, mux, http.MethodGet, nil, h.CookieName(), sid)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	var resp operatorapi.RAGEmbeddingGetResponse
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Status != "embed_model_not_in_catalog" || resp.ModelInCatalog {
		t.Fatalf("resp=%+v", resp)
	}
}

func TestRAGEmbeddingPUT_rejectsInvalidModel(t *testing.T) {
	mux, h, _, sid, _ := testRAGEmbeddingEnv(t, "test-embed")
	rec := embeddingRequest(t, mux, http.MethodPut, operatorapi.RAGEmbeddingPutRequest{Model: "not-in-catalog"}, h.CookieName(), sid)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestRAGEmbeddingPUT_persistsAndReloadsIndexerConfig(t *testing.T) {
	mux, h, rt, sid, _ := testRAGEmbeddingEnv(t, "test-embed")
	rec := embeddingRequest(t, mux, http.MethodPut, operatorapi.RAGEmbeddingPutRequest{
		Model: "ollama/nomic-embed-text:latest",
	}, h.CookieName(), sid)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var putResp operatorapi.RAGEmbeddingPutResponse
	if err := json.NewDecoder(rec.Body).Decode(&putResp); err != nil {
		t.Fatal(err)
	}
	if !putResp.OK || putResp.Model != "ollama/nomic-embed-text:latest" || putResp.Dim != 768 {
		t.Fatalf("putResp=%+v", putResp)
	}

	rt.Sync()
	res, _ := rt.Snapshot()
	if res.RAG.EmbeddingModel != "ollama/nomic-embed-text:latest" || res.RAG.EmbeddingDim != 768 {
		t.Fatalf("yaml reload: model=%q dim=%d", res.RAG.EmbeddingModel, res.RAG.EmbeddingDim)
	}
	if rag := rt.RAG(); rag == nil || rag.EmbeddingModel() != "ollama/nomic-embed-text:latest" || rag.EmbedDim() != 768 {
		t.Fatalf("rag reload: svc=%v model=%q dim=%d", rag != nil, func() string {
			if rag == nil {
				return ""
			}
			return rag.EmbeddingModel()
		}(), func() int {
			if rag == nil {
				return 0
			}
			return rag.EmbedDim()
		}())
	}

	raw, err := os.ReadFile(rt.GatewayPath())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "ollama/nomic-embed-text:latest") || !strings.Contains(string(raw), "dim: 768") {
		t.Fatalf("gateway.yaml missing patch: %s", raw)
	}
}

func TestRAGEmbeddingPUT_probesUnknownDim(t *testing.T) {
	mux, h, rt, sid, _ := testRAGEmbeddingEnv(t, "test-embed")
	rec := embeddingRequest(t, mux, http.MethodPut, operatorapi.RAGEmbeddingPutRequest{
		Model: "groq/unknown-chat",
	}, h.CookieName(), sid)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var putResp operatorapi.RAGEmbeddingPutResponse
	_ = json.NewDecoder(rec.Body).Decode(&putResp)
	if putResp.Dim != 8 {
		t.Fatalf("probed dim=%d", putResp.Dim)
	}
	rt.Sync()
	if rag := rt.RAG(); rag == nil || rag.EmbedDim() != 8 {
		t.Fatalf("rag dim after probe: %v", rag)
	}
}

func TestRAGEmbeddingPUT_unauthorized(t *testing.T) {
	mux, _, _, _, _ := testRAGEmbeddingEnv(t, "test-embed")
	rec := embeddingRequest(t, mux, http.MethodPut, operatorapi.RAGEmbeddingPutRequest{Model: "test-embed"}, "", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d", rec.Code)
	}
}

func TestRAGEmbeddingGET_unauthorized(t *testing.T) {
	mux, _, _, _, _ := testRAGEmbeddingEnv(t, "test-embed")
	rec := embeddingRequest(t, mux, http.MethodGet, nil, "", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d", rec.Code)
	}
}

func TestBuildEmbeddingCandidates_emptyWhenNoCatalog(t *testing.T) {
	t.Parallel()
	if got := uirag.BuildEmbeddingCandidatesForTest(nil); got != nil {
		t.Fatalf("got=%v", got)
	}
}

func TestResolveEmbeddingDim_known(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	gwPath := filepath.Join(dir, "gateway.yaml")
	raw := `gateway: { listen_port: 3000 }
paths: { tokens: "./t.yaml" }
vectorstore: { url: "http://127.0.0.1:6333" }
rag:
  enabled: true
  embedding:
    model: "x"
    dim: 8
broker:
  base_url: "http://127.0.0.1:8080"
`
	if err := os.WriteFile(gwPath, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	rt, err := gruntime.NewRuntime(gwPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	res, _ := rt.Snapshot()
	dim, err := uirag.ResolveEmbeddingDimForTest(context.Background(), rt, res, "ollama/nomic-embed-text:latest")
	if err != nil || dim != 768 {
		t.Fatalf("dim=%d err=%v", dim, err)
	}
}
