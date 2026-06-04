package indexer_test

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/operatorstore"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/rag"
	uiindexer "github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/adminui/api/indexer"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/adminui/handler"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/adminui/session"
	gruntime "github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/runtime"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/testsupport"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/vectorstore"
	"github.com/lynn/porcelain/internal/naming"
)

type purgeTestStore struct {
	mu                  sync.Mutex
	collections         map[string]int
	points              map[string][]vectorstore.Point
	deleteCollectionErr error
}

func newPurgeTestStore() *purgeTestStore {
	return &purgeTestStore{
		collections: map[string]int{},
		points:      map[string][]vectorstore.Point{},
	}
}

func (s *purgeTestStore) EnsureCollection(_ context.Context, name string, dim int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.collections[name]; !ok {
		s.collections[name] = dim
	}
	return nil
}
func (s *purgeTestStore) Upsert(_ context.Context, c string, pts []vectorstore.Point) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.points[c] = append(s.points[c], pts...)
	return nil
}
func (s *purgeTestStore) Search(_ context.Context, c string, _ []float32, k int, _ float32, _ *vectorstore.Coords) ([]vectorstore.Hit, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]vectorstore.Hit, 0, len(s.points[c]))
	for i, p := range s.points[c] {
		if i >= k {
			break
		}
		out = append(out, vectorstore.Hit{ID: p.ID, Score: 0.95, Payload: p.Payload})
	}
	return out, nil
}
func (s *purgeTestStore) Health(context.Context) error { return nil }
func (s *purgeTestStore) Stats(_ context.Context, c string) (vectorstore.Stats, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return vectorstore.Stats{Collection: c, Points: int64(len(s.points[c])), VectorDim: s.collections[c]}, nil
}
func (s *purgeTestStore) DeleteBySource(context.Context, string, string) error { return nil }
func (s *purgeTestStore) DeleteCollection(_ context.Context, c string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.deleteCollectionErr != nil {
		return s.deleteCollectionErr
	}
	delete(s.collections, c)
	delete(s.points, c)
	return nil
}
func (s *purgeTestStore) ScrollPoints(context.Context, string, *vectorstore.Coords, int, string) (vectorstore.ScrollBatch, error) {
	return vectorstore.ScrollBatch{}, nil
}

func (s *purgeTestStore) GetPoints(_ context.Context, c string, ids []string) ([]vectorstore.PointPayload, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	byID := map[string]vectorstore.Point{}
	for _, p := range s.points[c] {
		byID[p.ID] = p
	}
	var out []vectorstore.PointPayload
	for _, id := range ids {
		if p, ok := byID[id]; ok {
			out = append(out, vectorstore.PointPayload{ID: p.ID, Payload: p.Payload})
		}
	}
	return out, nil
}

type stubEmbed struct{ dim int }

func (stubEmbed) EmbedBatch(_ context.Context, in []string) ([][]float32, error) {
	out := make([][]float32, len(in))
	for i := range in {
		out[i] = make([]float32, 8)
		out[i][0] = float32(i + 1)
	}
	return out, nil
}
func (s stubEmbed) EmbedOne(_ context.Context, _ string) ([]float32, error) {
	return make([]float32, s.dim), nil
}
func (stubEmbed) Model() string { return "test-embed" }

func testWorkspaceDeleteEnv(t *testing.T) (*http.ServeMux, *handler.Handler, *gruntime.Runtime, *purgeTestStore, string) {
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
	if err := os.WriteFile(filepath.Join(dir, naming.APIKeysFileTarget), []byte("api_keys:\n  - secret: \"ws-del-tok\"\n    tenant_id: \"tenantA\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, naming.RoutingPolicyFileTarget), []byte("rules: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv(naming.EnvBrokerAPIKeyTarget, "ukey")

	rt, err := gruntime.NewRuntime(gwPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	opStore, err := operatorstore.Open(filepath.Join(dir, "operator.sqlite"), testsupport.GatewayOperatorMigrationsDir(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	rt.SetOperatorStoreForTest(opStore)
	t.Cleanup(func() { rt.CloseOperator(); rt.CloseMetrics() })

	mem := newPurgeTestStore()
	svc, err := rag.New(rag.Options{
		Store:        mem,
		Embedder:     stubEmbed{dim: 8},
		ChunkSize:    128,
		ChunkOverlap: 32,
		TopK:         4,
		EmbeddingDim: 8,
		Log:          slog.New(slog.NewTextHandler(ioDiscard{}, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}
	rt.SetRAGForTest(svc)

	ui := session.NewUIOptions()
	h := handler.New(rt, nil, ui)
	mux := http.NewServeMux()
	uiindexer.Register(mux, h)
	sid, err := ui.Sessions.Issue("tenantA")
	if err != nil {
		t.Fatal(err)
	}
	return mux, h, rt, mem, sid
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) { return len(p), nil }

func authedReq(method, path, sid, cookie string) *http.Request {
	r := httptest.NewRequest(method, path, nil)
	r.AddCookie(&http.Cookie{Name: cookie, Value: sid})
	return r
}

func TestWorkspaceDELETE_purgesScopedCollectionOnly(t *testing.T) {
	mux, h, rt, mem, sid := testWorkspaceDeleteEnv(t)
	ctx := context.Background()
	st := rt.OperatorStore()
	if st == nil {
		t.Fatal("operator store unavailable")
	}
	dirA := t.TempDir()
	dirB := t.TempDir()
	wsA, err := st.CreateWorkspace(ctx, "", "projA", "flA", []string{dirA})
	if err != nil {
		t.Fatal(err)
	}
	wsB, err := st.CreateWorkspace(ctx, "", "projB", "flB", []string{dirB})
	if err != nil {
		t.Fatal(err)
	}
	ragSvc := rt.RAG()
	if ragSvc == nil {
		t.Fatal("rag unavailable")
	}
	text := strings.Repeat("alpha ", 80)
	if _, err := ragSvc.Ingest(ctx, rag.IngestRequest{
		Coords: vectorstore.Coords{TenantID: "tenantA", ProjectID: wsA.ProjectID, FlavorID: wsA.FlavorID},
		Source: "a.txt",
		Text:   text,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := ragSvc.Ingest(ctx, rag.IngestRequest{
		Coords: vectorstore.Coords{TenantID: "tenantA", ProjectID: wsB.ProjectID, FlavorID: wsB.FlavorID},
		Source: "b.txt",
		Text:   text,
	}); err != nil {
		t.Fatal(err)
	}
	collA := vectorstore.CollectionName(vectorstore.Coords{TenantID: "tenantA", ProjectID: wsA.ProjectID, FlavorID: wsA.FlavorID})
	collB := vectorstore.CollectionName(vectorstore.Coords{TenantID: "tenantA", ProjectID: wsB.ProjectID, FlavorID: wsB.FlavorID})
	if len(mem.points[collA]) == 0 || len(mem.points[collB]) == 0 {
		t.Fatalf("expected ingested points in both collections")
	}

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, authedReq(http.MethodDelete, fmt.Sprintf("/api/ui/indexer/workspaces/%d", wsA.ID), sid, h.CookieName()))
	if rec.Code != http.StatusOK {
		t.Fatalf("delete status=%d body=%s", rec.Code, rec.Body.String())
	}
	if got, err := st.GetWorkspace(ctx, "", wsA.ID); err != nil || got != nil {
		t.Fatalf("workspace A should be deleted: %+v err=%v", got, err)
	}
	if got, err := st.GetWorkspace(ctx, "", wsB.ID); err != nil || got == nil {
		t.Fatalf("workspace B should remain: %+v err=%v", got, err)
	}
	if len(mem.points[collA]) != 0 {
		t.Fatalf("collection A should be purged, still has %d points", len(mem.points[collA]))
	}
	if len(mem.points[collB]) == 0 {
		t.Fatal("collection B should be untouched")
	}
	hits, err := ragSvc.Retrieve(ctx, rag.RetrieveRequest{
		Coords: vectorstore.Coords{TenantID: "tenantA", ProjectID: wsA.ProjectID, FlavorID: wsA.FlavorID},
		Query:  "alpha",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 0 {
		t.Fatalf("retrieve for deleted scope should be empty, got %d hits", len(hits))
	}
	hitsB, err := ragSvc.Retrieve(ctx, rag.RetrieveRequest{
		Coords: vectorstore.Coords{TenantID: "tenantA", ProjectID: wsB.ProjectID, FlavorID: wsB.FlavorID},
		Query:  "alpha",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(hitsB) == 0 {
		t.Fatal("retrieve for other scope should still return hits")
	}
}

func TestWorkspaceDELETE_purgeFailureKeepsWorkspace(t *testing.T) {
	mux, h, rt, mem, sid := testWorkspaceDeleteEnv(t)
	ctx := context.Background()
	st := rt.OperatorStore()
	dir := t.TempDir()
	ws, err := st.CreateWorkspace(ctx, "", "projFail", "flFail", []string{dir})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rt.RAG().Ingest(ctx, rag.IngestRequest{
		Coords: vectorstore.Coords{TenantID: "tenantA", ProjectID: ws.ProjectID, FlavorID: ws.FlavorID},
		Source: "fail.txt",
		Text:   strings.Repeat("beta ", 80),
	}); err != nil {
		t.Fatal(err)
	}
	mem.deleteCollectionErr = fmt.Errorf("qdrant down")

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, authedReq(http.MethodDelete, fmt.Sprintf("/api/ui/indexer/workspaces/%d", ws.ID), sid, h.CookieName()))
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("delete status=%d want 502 body=%s", rec.Code, rec.Body.String())
	}
	if got, err := st.GetWorkspace(ctx, "", ws.ID); err != nil || got == nil {
		t.Fatalf("workspace should remain after purge failure: %+v err=%v", got, err)
	}
}
