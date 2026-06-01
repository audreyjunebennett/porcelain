package rag_test

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

	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/rag"
	uirag "github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/adminui/api/rag"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/adminui/handler"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/adminui/session"
	gruntime "github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/runtime"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/vectorstore"
	"github.com/lynn/porcelain/internal/naming"
	"github.com/lynn/porcelain/internal/operatorapi"
)

type memSearchStore struct {
	points map[string][]vectorstore.Point
}

func (m *memSearchStore) EnsureCollection(_ context.Context, _ string, _ int) error { return nil }
func (m *memSearchStore) Upsert(_ context.Context, c string, pts []vectorstore.Point) error {
	if m.points == nil {
		m.points = map[string][]vectorstore.Point{}
	}
	m.points[c] = append(m.points[c], pts...)
	return nil
}
func (m *memSearchStore) Search(_ context.Context, c string, _ []float32, k int, _ float32, _ *vectorstore.Coords) ([]vectorstore.Hit, error) {
	pts := m.points[c]
	out := make([]vectorstore.Hit, 0, len(pts))
	for i, p := range pts {
		if i >= k {
			break
		}
		out = append(out, vectorstore.Hit{ID: p.ID, Score: 0.95 - float32(i)*0.01, Payload: p.Payload})
	}
	return out, nil
}
func (m *memSearchStore) Health(context.Context) error { return nil }
func (m *memSearchStore) Stats(_ context.Context, c string) (vectorstore.Stats, error) {
	return vectorstore.Stats{Collection: c, Points: int64(len(m.points[c])), VectorDim: 8}, nil
}
func (m *memSearchStore) DeleteBySource(context.Context, string, string) error { return nil }
func (m *memSearchStore) ScrollPoints(context.Context, string, *vectorstore.Coords, int, string) (vectorstore.ScrollBatch, error) {
	return vectorstore.ScrollBatch{}, nil
}

type stubEmbed struct{ dim int }

func (stubEmbed) EmbedBatch(_ context.Context, in []string) ([][]float32, error) {
	out := make([][]float32, len(in))
	for i := range in {
		out[i] = make([]float32, 8)
	}
	return out, nil
}
func (s stubEmbed) EmbedOne(_ context.Context, _ string) ([]float32, error) {
	return make([]float32, s.dim), nil
}
func (stubEmbed) Model() string { return "test-embed" }

func testRAGSearchEnv(t *testing.T) (*http.ServeMux, *handler.Handler, string, string) {
	t.Helper()
	dir := t.TempDir()
	gwPath := filepath.Join(dir, naming.GatewayConfigFileTarget)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(upstream.Close)

	raw := "gateway:\n  semver: \"0.2.0\"\n  listen_port: 0\n  listen_host: \"127.0.0.1\"\n" +
		"broker:\n  base_url: \"" + upstream.URL + "\"\n  api_key_env: \"" + naming.EnvBrokerAPIKeyTarget + "\"\n" +
		"health:\n  timeout_ms: 2000\n  chat_timeout_ms: 60000\n" +
		"paths:\n  tokens: \"./" + naming.APIKeysFileTarget + "\"\n  routing_policy: \"./" + naming.RoutingPolicyFileTarget + "\"\n" +
		"routing:\n  fallback_chain:\n    - \"m\"\n" +
		"vectorstore:\n  url: \"http://127.0.0.1:6333\"\n" +
		"rag:\n  enabled: true\n  embedding:\n    model: \"test-embed\"\n    dim: 8\n" +
		"  chunking:\n    size: 128\n    overlap: 32\n  ingest:\n    max_bytes: 10485760\n  defaults:\n    project_id: \"default\"\n"
	if err := os.WriteFile(gwPath, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	tokPath := filepath.Join(dir, naming.APIKeysFileTarget)
	if err := os.WriteFile(tokPath, []byte("api_keys:\n  - secret: \"rag-ui-tok\"\n    tenant_id: \"tenantR\"\n"), 0o644); err != nil {
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

	store := &memSearchStore{points: map[string][]vectorstore.Point{}}
	svc, err := rag.New(rag.Options{Store: store, Embedder: stubEmbed{dim: 8}, EmbeddingDim: 8, TopK: 4, ScoreThreshold: 0.72})
	if err != nil {
		t.Fatal(err)
	}
	rt.SetRAGForTest(svc)

	coords := vectorstore.Coords{TenantID: "tenantR", ProjectID: "default"}
	collection := vectorstore.CollectionName(coords)
	store.points[collection] = []vectorstore.Point{{
		ID: "pt-1",
		Payload: vectorstore.Payload{
			Source: "docs/readme.md",
			Text:   "workspace search fixture text",
		},
	}}

	ui := session.NewUIOptions()
	h := handler.New(rt, nil, ui)
	mux := http.NewServeMux()
	uirag.Register(mux, h)
	sid, err := ui.Sessions.Issue("tenantR")
	if err != nil {
		t.Fatal(err)
	}
	return mux, h, sid, "rag-ui-tok"
}

func postSearch(t *testing.T, mux *http.ServeMux, body any, cookieName, sid, bearer string) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/ui/rag/search", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	if sid != "" {
		req.AddCookie(&http.Cookie{Name: cookieName, Value: sid})
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func TestRAGSearchAPI_unauthorized(t *testing.T) {
	mux, _, _, _ := testRAGSearchEnv(t)
	rec := postSearch(t, mux, operatorapi.RAGSearchRequest{
		Query: "hello", ProjectID: "default",
	}, "", "", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestRAGSearchAPI_emptyQuery(t *testing.T) {
	mux, h, sid, _ := testRAGSearchEnv(t)
	rec := postSearch(t, mux, operatorapi.RAGSearchRequest{
		Query: "", ProjectID: "default",
	}, h.CookieName(), sid, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var resp operatorapi.RAGSearchResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Hits) != 0 {
		t.Fatalf("expected empty hits, got %d", len(resp.Hits))
	}
}

func TestRAGSearchAPI_sessionAndBearer(t *testing.T) {
	mux, h, sid, tok := testRAGSearchEnv(t)
	body := operatorapi.RAGSearchRequest{
		Query:     "fixture",
		ProjectID: "default",
	}

	rec := postSearch(t, mux, body, h.CookieName(), sid, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("session status=%d body=%s", rec.Code, rec.Body.String())
	}
	var sessionResp operatorapi.RAGSearchResponse
	if err := json.NewDecoder(rec.Body).Decode(&sessionResp); err != nil {
		t.Fatal(err)
	}
	if len(sessionResp.Hits) != 1 || sessionResp.Hits[0].Source != "docs/readme.md" {
		t.Fatalf("session hits: %+v", sessionResp.Hits)
	}
	if sessionResp.ScoreThreshold != 0.72 {
		t.Fatalf("score_threshold=%v", sessionResp.ScoreThreshold)
	}

	rec = postSearch(t, mux, body, "", "", tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("bearer status=%d body=%s", rec.Code, rec.Body.String())
	}
	var bearerResp operatorapi.RAGSearchResponse
	if err := json.NewDecoder(io.MultiReader(rec.Body)).Decode(&bearerResp); err != nil {
		t.Fatal(err)
	}
	if len(bearerResp.Hits) != 1 {
		t.Fatalf("bearer hits: %+v", bearerResp.Hits)
	}
}

func TestRAGSearchAPI_wrongScopeEmptyHits(t *testing.T) {
	mux, h, sid, _ := testRAGSearchEnv(t)
	rec := postSearch(t, mux, operatorapi.RAGSearchRequest{
		Query: "fixture", ProjectID: "other-project",
	}, h.CookieName(), sid, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	var resp operatorapi.RAGSearchResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Hits) != 0 {
		t.Fatalf("expected empty hits for wrong scope, got %+v", resp.Hits)
	}
	if !strings.Contains(resp.Collection, "other-project") {
		t.Fatalf("collection=%q", resp.Collection)
	}
}

func TestRAGSearchAPI_customScoreThreshold(t *testing.T) {
	mux, h, sid, _ := testRAGSearchEnv(t)
	thr := 0.5
	rec := postSearch(t, mux, operatorapi.RAGSearchRequest{
		Query:          "fixture",
		ProjectID:      "default",
		ScoreThreshold: &thr,
	}, h.CookieName(), sid, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var resp operatorapi.RAGSearchResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.ScoreThreshold != 0.5 {
		t.Fatalf("score_threshold=%v", resp.ScoreThreshold)
	}
}
