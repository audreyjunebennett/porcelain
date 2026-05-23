package indexer

import (
	"context"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestClient_FetchConfigAndHealth(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/indexer/config", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			http.Error(w, "auth", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"gateway_version":"v0.2","embedding_model":"m","embedding_dim":8,"chunk_size":512,"chunk_overlap":128,"ingest_path":"/v1/ingest","max_ingest_bytes":1024,"corpus_inventory_path":"/v1/indexer/corpus/inventory"}`))
	})
	mux.HandleFunc("/v1/indexer/storage/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"status":"ready"}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := NewGatewayClient(srv.URL, "tok", 5*time.Second)
	ctx := context.Background()
	cfg, err := c.FetchConfig(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.EmbeddingDim != 8 || cfg.ChunkSize != 512 {
		t.Fatalf("cfg=%+v", cfg)
	}
	if cfg.CorpusInventoryPath != "/v1/indexer/corpus/inventory" {
		t.Fatalf("corpus path: %q", cfg.CorpusInventoryPath)
	}
	h, err := c.CheckHealth(ctx)
	if err != nil || !h.OK {
		t.Fatalf("health=%+v err=%v", h, err)
	}
}

func TestClient_Ingest_Multipart(t *testing.T) {
	var seenSource, seenHash, seenFilename, seenBody, seenProj, seenFlavor string
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/ingest", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			http.Error(w, "auth", http.StatusUnauthorized)
			return
		}
		seenProj = r.Header.Get("X-Chimera-Project")
		seenFlavor = r.Header.Get("X-Chimera-Flavor-Id")
		mt, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || !strings.HasPrefix(mt, "multipart/") {
			http.Error(w, "bad ct", http.StatusBadRequest)
			return
		}
		mr := multipart.NewReader(r.Body, params["boundary"])
		for {
			p, err := mr.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				http.Error(w, err.Error(), 400)
				return
			}
			b, _ := io.ReadAll(p)
			switch p.FormName() {
			case "source":
				seenSource = string(b)
			case "content_hash":
				seenHash = string(b)
			case "file":
				seenFilename = p.FileName()
				seenBody = string(b)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"ingest.result","tenant_id":"t","project_id":"p","flavor_id":"f","source":"src/main.go","content_hash":"sha256:abc","content_sha256":"sha256:abc","chunks":3,"collection":"c"}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	c := NewGatewayClient(srv.URL, "tok", 5*time.Second)
	res, err := c.Ingest(context.Background(), IngestRequest{
		Source:      "src/main.go",
		ContentHash: "sha256:abc",
		Body:        strings.NewReader("hello"),
		Project:     "p",
		Flavor:      "f",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Chunks != 3 {
		t.Fatalf("chunks=%d", res.Chunks)
	}
	if seenSource != "src/main.go" || seenHash != "sha256:abc" || seenBody != "hello" || seenFilename != "main.go" {
		t.Fatalf("multipart fields: source=%q hash=%q body=%q file=%q", seenSource, seenHash, seenBody, seenFilename)
	}
	if seenProj != "p" || seenFlavor != "f" {
		t.Fatalf("scope headers: project=%q flavor=%q", seenProj, seenFlavor)
	}
}

func TestClient_RetryClassification(t *testing.T) {
	cases := []struct {
		status       int
		retry, fatal bool
	}{
		{http.StatusServiceUnavailable, true, false},
		{http.StatusTooManyRequests, true, false},
		{http.StatusInternalServerError, true, false},
		{http.StatusUnauthorized, false, true},
		{http.StatusForbidden, false, true},
		{http.StatusBadRequest, false, true},
	}
	for _, c := range cases {
		err := &HTTPError{Path: "/v1/ingest", Status: c.status}
		if got := IsRetryable(err); got != c.retry {
			t.Fatalf("status %d: retry=%v want %v", c.status, got, c.retry)
		}
		if got := IsFatal(err); got != c.fatal {
			t.Fatalf("status %d: fatal=%v want %v", c.status, got, c.fatal)
		}
	}
}

func TestClient_Ingest_RetryableThenSuccess(t *testing.T) {
	var calls int32
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/ingest", func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n < 3 {
			http.Error(w, "busy", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"chunks":1}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	c := NewGatewayClient(srv.URL, "tok", 5*time.Second)

	var lastErr error
	var resp *IngestResponse
	for attempt := 0; attempt < 5; attempt++ {
		resp, lastErr = c.Ingest(context.Background(), IngestRequest{
			Source: "a.txt", Body: strings.NewReader("x"),
		})
		if lastErr == nil {
			break
		}
		if !IsRetryable(lastErr) {
			t.Fatalf("non-retryable err on attempt %d: %v", attempt, lastErr)
		}
	}
	if lastErr != nil {
		t.Fatalf("expected eventual success, got %v", lastErr)
	}
	if resp == nil || resp.Chunks != 1 || atomic.LoadInt32(&calls) < 3 {
		t.Fatalf("resp=%+v calls=%d", resp, calls)
	}
}

func TestClient_CheckHealth_RAGDisabledStructuredError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/indexer/storage/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":{"message":"RAG is not enabled","type":"gateway_config"}}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	c := NewGatewayClient(srv.URL, "tok", time.Second)
	h, err := c.CheckHealth(context.Background())
	if err != nil {
		t.Fatalf("structured-error response should not be a hard error: %v", err)
	}
	if h == nil || h.OK {
		t.Fatalf("h=%+v", h)
	}
	if !h.RAGDisabled {
		t.Fatalf("expected RAGDisabled=true, got %+v", h)
	}
	if h.Message == "" || h.ErrorType != "gateway_config" {
		t.Fatalf("unexpected fields: %+v", h)
	}
}

func TestClient_CheckHealth_DegradedDetailShape(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/indexer/storage/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"object":"indexer.storage.health","status":"degraded","ok":false,"checks":{"vectorstore":{"ok":false,"status":"unavailable","detail":"qdrant down","reason_code":"vectorstore_unreachable"},"embedding":{"ok":true,"status":"ok","model":"m","model_in_catalog":true}}}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	c := NewGatewayClient(srv.URL, "tok", time.Second)
	h, err := c.CheckHealth(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if h.OK || h.RAGDisabled || h.Status != "degraded" {
		t.Fatalf("unexpected: %+v", h)
	}
	if h.VectorstoreOK() {
		t.Fatalf("vectorstore should be down")
	}
	if !h.EmbedOK() {
		t.Fatalf("embedding should be ok in fixture")
	}
	if h.ReasonCode() != "vectorstore_unreachable" {
		t.Fatalf("reason=%q", h.ReasonCode())
	}
	if d := h.HealthDetail(); d != "qdrant down" {
		t.Fatalf("detail=%q", d)
	}
}

func TestHealthStatus_IngestReady(t *testing.T) {
	allOK := &HealthStatus{
		OK: true,
		Checks: &HealthChecks{
			Vectorstore: HealthCheck{OK: true},
			Embedding:   HealthCheck{OK: true},
		},
	}
	if !allOK.IngestReady() {
		t.Fatal("expected ingest ready")
	}
	embedDown := &HealthStatus{
		OK: false,
		Checks: &HealthChecks{
			Vectorstore: HealthCheck{OK: true},
			Embedding:   HealthCheck{OK: false, ReasonCode: "embed_provider_down"},
		},
	}
	if embedDown.IngestReady() {
		t.Fatal("expected not ingest ready")
	}
}

func TestClient_FetchConfig_SendsOptionalScopeHeaders(t *testing.T) {
	var gotProj, gotFlavor string
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/indexer/config", func(w http.ResponseWriter, r *http.Request) {
		gotProj = r.Header.Get("X-Chimera-Project")
		gotFlavor = r.Header.Get("X-Chimera-Flavor-Id")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"gateway_version":"v0.3","embedding_model":"m","embedding_dim":8,"chunk_size":512,"chunk_overlap":128,"ingest_path":"/v1/ingest","max_ingest_bytes":1024}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	c := NewGatewayClient(srv.URL, "tok", 5*time.Second)
	_, err := c.FetchConfig(context.Background(), map[string]string{
		"X-Chimera-Project":   "acme",
		"X-Chimera-Flavor-Id": "prod",
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotProj != "acme" || gotFlavor != "prod" {
		t.Fatalf("headers project=%q flavor=%q", gotProj, gotFlavor)
	}
}

// Sanity: HTTPError formats and surfaces JSON status fields.
func TestHTTPError_String(t *testing.T) {
	e := &HTTPError{Path: "/v1/x", Status: 503, Body: `{"error":"busy"}`}
	if !strings.Contains(e.Error(), "503") || !strings.Contains(e.Error(), "/v1/x") {
		t.Fatalf("err: %s", e.Error())
	}
}

// Ensure the JSON tags survive a round-trip; cheap regression check on the
// IngestResponse shape we depend on.
func TestIngestResponse_JSONShape(t *testing.T) {
	in := IngestResponse{Object: "ingest.result", Chunks: 2}
	b, _ := json.Marshal(in)
	var out IngestResponse
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if out.Chunks != 2 || out.Object != "ingest.result" {
		t.Fatalf("round-trip mismatch: %+v", out)
	}
}

func TestClient_CheckGatewayRootHealth(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	c := NewGatewayClient(srv.URL, "tok", time.Second)
	h, err := c.CheckGatewayRootHealth(context.Background())
	if err != nil || h == nil || !h.OK {
		t.Fatalf("h=%+v err=%v", h, err)
	}

	mux2 := http.NewServeMux()
	mux2.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"degraded":true,"status":"degraded"}`))
	})
	s2 := httptest.NewServer(mux2)
	defer s2.Close()
	c2 := NewGatewayClient(s2.URL, "tok", time.Second)
	h2, err := c2.CheckGatewayRootHealth(context.Background())
	if err != nil || h2 == nil || h2.OK {
		t.Fatalf("h2=%+v err=%v", h2, err)
	}
}

func TestClient_IngestChunked_RetriesChunkPUT(t *testing.T) {
	var chunkCalls atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/ingest/session", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"session_id":"s1","max_chunk_bytes":4,"max_total_bytes":9999}`))
	})
	mux.HandleFunc("/v1/ingest/session/s1/chunk", func(w http.ResponseWriter, r *http.Request) {
		n := chunkCalls.Add(1)
		if n == 1 {
			http.Error(w, "busy", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/v1/ingest/session/s1/complete", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"ingest.result","chunks":2,"content_sha256":"sha256:ab"}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	gw := &IndexerConfig{IngestSessionPath: "/v1/ingest/session", MaxIngestBytes: 10000}
	c := NewGatewayClient(srv.URL, "tok", 5*time.Second)
	pol := SessionRetryPolicy{MaxAttempts: 5, BaseDelay: time.Millisecond, MaxDelay: 5 * time.Millisecond}
	tmp := filepath.Join(t.TempDir(), "f.txt")
	if err := os.WriteFile(tmp, []byte("abcdefgh"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := c.IngestChunked(context.Background(), tmp, IngestRequest{Source: "f.txt", ContentHash: "sha256:deadbeef"}, gw, pol)
	if err != nil {
		t.Fatal(err)
	}
	if n := chunkCalls.Load(); n < 3 {
		t.Fatalf("expected at least one retried chunk PUT, got %d calls", n)
	}
}

func TestClient_IndexRunIDHeader(t *testing.T) {
	var saw string
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/indexer/config", func(w http.ResponseWriter, r *http.Request) {
		saw = r.Header.Get("X-Chimera-Index-Run-Id")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"gateway_version":"v0.2","embedding_model":"m","embedding_dim":8,"chunk_size":512,"chunk_overlap":128,"ingest_path":"/v1/ingest","max_ingest_bytes":1024}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	c := NewGatewayClient(srv.URL, "tok", 5*time.Second)
	c.IndexRunID = "run-test-1"
	_, err := c.FetchConfig(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if saw != "run-test-1" {
		t.Fatalf("header: %q", saw)
	}
}

func TestClient_FetchWorkspaces_BuildsRoots(t *testing.T) {
	rootDir := t.TempDir()
	payload := map[string]any{
		"object":    "indexer.workspaces",
		"tenant_id": "tenantA",
		"workspaces": []map[string]any{
			{
				"workspace_id": int64(7),
				"project_id":   "p1",
				"flavor_id":    "f1",
				"paths": []map[string]any{
					{"path_id": int64(2), "path": rootDir},
				},
			},
		},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/indexer/workspaces", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			http.Error(w, "auth", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(raw)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := NewGatewayClient(srv.URL, "tok", 5*time.Second)
	ctx := context.Background()
	resp, err := c.FetchWorkspaces(ctx, nil, SessionRetryPolicy{MaxAttempts: 3, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	roots, err := RootsFromWorkspacesResponse(resp)
	if err != nil {
		t.Fatal(err)
	}
	if len(roots) != 1 {
		t.Fatalf("roots=%+v", roots)
	}
	if roots[0].Scope.ProjectID != "p1" || roots[0].Scope.FlavorID != "f1" || roots[0].Scope.WorkspaceID != "7" {
		t.Fatalf("scope=%+v", roots[0].Scope)
	}
	if roots[0].AbsPath != filepath.Clean(rootDir) {
		t.Fatalf("path=%q", roots[0].AbsPath)
	}

	cfg := Resolved{
		SupervisedLayer:  true,
		GatewayURL:       srv.URL,
		Token:            "tok",
		RequestTimeout:   5 * time.Second,
		RetryMaxAttempts: 3,
		RetryBaseDelay:   time.Millisecond,
		RetryMaxDelay:    time.Millisecond,
	}
	if err := MaterializeRootsFromGateway(ctx, c, &cfg, RetryPolicyFromResolved(cfg)); err != nil {
		t.Fatal(err)
	}
	if len(cfg.Roots) != 1 || cfg.Roots[0].Scope.WorkspaceID != "7" {
		t.Fatalf("cfg.Roots=%+v", cfg.Roots)
	}
}
