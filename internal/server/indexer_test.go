package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lynn/claudia-gateway/internal/rag"
	"github.com/lynn/claudia-gateway/internal/vectorstore"
)

func TestIndexerConfig_HappyPath(t *testing.T) {
	_, _, srv := setupRAGServer(t)
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/v1/indexer/config", nil)
	req.Header.Set("Authorization", "Bearer ingest-tok")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("status %d %s", res.StatusCode, b)
	}
	var doc map[string]any
	if err := json.NewDecoder(res.Body).Decode(&doc); err != nil {
		t.Fatal(err)
	}
	if doc["chunk_size"].(float64) != 128 || doc["chunk_overlap"].(float64) != 32 {
		t.Fatalf("doc: %+v", doc)
	}
	if doc["embedding_model"] != "test-embed" {
		t.Fatalf("model: %+v", doc["embedding_model"])
	}
	if doc["ingest_path"] != "/v1/ingest" {
		t.Fatalf("ingest_path: %+v", doc["ingest_path"])
	}
	if doc["max_whole_file_bytes"] == nil || doc["ingest_session_path"] == nil {
		t.Fatalf("expected v0.4 indexer fields: %+v", doc)
	}
	hdrs, _ := doc["optional_headers"].([]any)
	if len(hdrs) != 3 {
		t.Fatalf("optional_headers: %+v", hdrs)
	}
}

func TestIndexerWorkspaces_HappyPath_LegacyTenant(t *testing.T) {
	rt, _, srv := setupRAGServer(t)
	dir := t.TempDir()
	st := rt.OperatorStore()
	if st == nil {
		t.Fatal("operator store unavailable")
	}
	ctx := context.Background()
	if _, err := st.CreateWorkspace(ctx, "", "projW", "flW", []string{dir}); err != nil {
		t.Fatal(err)
	}

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/v1/indexer/workspaces", nil)
	req.Header.Set("Authorization", "Bearer ingest-tok")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("status %d %s", res.StatusCode, b)
	}
	var doc struct {
		Object     string `json:"object"`
		TenantID   string `json:"tenant_id"`
		Workspaces []struct {
			WorkspaceID int64  `json:"workspace_id"`
			ProjectID   string `json:"project_id"`
			FlavorID    string `json:"flavor_id"`
			Paths       []struct {
				PathID int64  `json:"path_id"`
				Path   string `json:"path"`
			} `json:"paths"`
		} `json:"workspaces"`
	}
	if err := json.NewDecoder(res.Body).Decode(&doc); err != nil {
		t.Fatal(err)
	}
	if doc.Object != "indexer.workspaces" {
		t.Fatalf("object=%q", doc.Object)
	}
	if len(doc.Workspaces) != 1 || doc.Workspaces[0].ProjectID != "projW" {
		t.Fatalf("workspaces=%+v", doc.Workspaces)
	}
	if len(doc.Workspaces[0].Paths) != 1 || filepath.Clean(doc.Workspaces[0].Paths[0].Path) != filepath.Clean(dir) {
		t.Fatalf("paths=%+v", doc.Workspaces[0].Paths)
	}
}

func TestIndexerHealth_OK(t *testing.T) {
	_, _, srv := setupRAGServer(t)
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/v1/indexer/storage/health", nil)
	req.Header.Set("Authorization", "Bearer ingest-tok")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("status %d %s", res.StatusCode, b)
	}
	var doc map[string]any
	_ = json.NewDecoder(res.Body).Decode(&doc)
	if doc["ok"] != true || doc["status"] != "ok" {
		t.Fatalf("doc: %+v", doc)
	}
}

func TestIndexerHealth_Degraded(t *testing.T) {
	rt, store, srv := setupRAGServer(t)
	store.healthErr = errors.New("connection refused")
	_ = rt
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/v1/indexer/storage/health", nil)
	req.Header.Set("Authorization", "Bearer ingest-tok")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status %d", res.StatusCode)
	}
	var doc map[string]any
	_ = json.NewDecoder(res.Body).Decode(&doc)
	if doc["ok"] != false {
		t.Fatalf("doc: %+v", doc)
	}
}

func TestIndexerStats_AfterIngest(t *testing.T) {
	rt, store, srv := setupRAGServer(t)
	_, err := rt.RAG().Ingest(context.Background(), rag.IngestRequest{
		Coords: vectorstore.Coords{TenantID: "tenantA", ProjectID: "proj"},
		Source: "docs/a.md",
		Text:   strings.Repeat("alpha ", 80),
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = store

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/v1/indexer/storage/stats", nil)
	req.Header.Set("Authorization", "Bearer ingest-tok")
	req.Header.Set(headerProject, "proj")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("status %d %s", res.StatusCode, b)
	}
	var doc map[string]any
	_ = json.NewDecoder(res.Body).Decode(&doc)
	if doc["points"].(float64) < 1 {
		t.Fatalf("doc: %+v", doc)
	}
	if doc["vector_dim"].(float64) != 8 {
		t.Fatalf("doc: %+v", doc)
	}
}

func TestIndexerCorpusInventory_AfterIngest(t *testing.T) {
	rt, _, srv := setupRAGServer(t)
	_, err := rt.RAG().Ingest(context.Background(), rag.IngestRequest{
		Coords:      vectorstore.Coords{TenantID: "tenantA", ProjectID: "proj"},
		Source:      "docs/b.md",
		Text:        strings.Repeat("beta ", 80),
		ContentHash: "sha256:clienthash",
	})
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/v1/indexer/corpus/inventory?limit=50", nil)
	req.Header.Set("Authorization", "Bearer ingest-tok")
	req.Header.Set(headerProject, "proj")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("status %d %s", res.StatusCode, b)
	}
	var doc struct {
		Entries []struct {
			Source            string `json:"source"`
			ContentSHA256     string `json:"content_sha256"`
			ClientContentHash string `json:"client_content_hash"`
		} `json:"entries"`
		HasMore bool `json:"has_more"`
	}
	if err := json.NewDecoder(res.Body).Decode(&doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Entries) < 1 {
		t.Fatalf("entries=%v", doc.Entries)
	}
	found := false
	for _, e := range doc.Entries {
		if e.Source == "docs/b.md" && strings.HasPrefix(e.ContentSHA256, "sha256:") && e.ClientContentHash == "sha256:clienthash" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("missing expected entry: %+v", doc.Entries)
	}
}

func TestHealth_RAGProbeIncluded(t *testing.T) {
	rt, store, srv := setupRAGServer(t)
	_ = rt
	store.healthErr = nil
	res, err := http.Get(srv.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("status %d %s", res.StatusCode, b)
	}
	var doc map[string]any
	_ = json.NewDecoder(res.Body).Decode(&doc)
	checks, _ := doc["checks"].(map[string]any)
	if _, ok := checks["qdrant"]; !ok {
		t.Fatalf("missing qdrant check: %+v", checks)
	}
}

func TestHealth_RAGFailDegrades(t *testing.T) {
	rt, store, srv := setupRAGServer(t)
	store.healthErr = errors.New("nope")
	_ = rt
	res, err := http.Get(srv.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status %d", res.StatusCode)
	}
}

func TestHealth_NoRAGProbeWhenDisabled(t *testing.T) {
	t.Setenv("CLAUDIA_UPSTREAM_API_KEY", "ukey")
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
		}
	}))
	t.Cleanup(upstream.Close)
	dir := t.TempDir()
	gwPath := filepath.Join(dir, "gateway.yaml")
	writeGateway(t, gwPath, upstream.URL, []string{"m"})
	tokPath := filepath.Join(dir, "tokens.yaml")
	writeTokens(t, tokPath, "tok", "ten")
	routePath := filepath.Join(dir, "routing-policy.yaml")
	_ = os.WriteFile(routePath, []byte("rules: []\n"), 0o644)
	rt, err := NewRuntime(gwPath, testLog())
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(NewMux(rt, testLog(), nil, nil))
	t.Cleanup(srv.Close)

	res, err := http.Get(srv.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var doc map[string]any
	_ = json.NewDecoder(res.Body).Decode(&doc)
	checks, _ := doc["checks"].(map[string]any)
	if _, ok := checks["qdrant"]; ok {
		t.Fatalf("qdrant check should not be present when RAG disabled: %+v", checks)
	}
}
