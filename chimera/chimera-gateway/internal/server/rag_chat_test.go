package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/lynn/porcelain/internal/platform/requestid"
	"github.com/lynn/porcelain/chimera/internal/rag"
	"github.com/lynn/porcelain/chimera/internal/vectorstore"
)

// setupRAGChatServer wires a runtime where:
//   - the upstream stub serves /health and /v1/chat/completions and records
//     the messages it sees,
//   - RAG is enabled and pre-loaded with one ingested doc.
func setupRAGChatServer(t *testing.T) (string, *capturedReqs, *Runtime) {
	t.Helper()
	t.Setenv("CHIMERA_UPSTREAM_API_KEY", "ukey")

	captured := &capturedReqs{}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			w.WriteHeader(http.StatusOK)
		case "/v1/chat/completions":
			body, _ := io.ReadAll(r.Body)
			captured.add(body)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"resp","choices":[{"index":0,"message":{"role":"assistant","content":"ok"}}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(upstream.Close)

	dir := t.TempDir()
	gwPath := filepath.Join(dir, "gateway.yaml")
	writeGatewayWithRAG(t, gwPath, upstream.URL, []string{"groq/m"}, "http://127.0.0.1:1")
	tokPath := filepath.Join(dir, "api-keys.yaml")
	writeTokens(t, tokPath, "rag-tok", "tenantR")
	routePath := filepath.Join(dir, "routing-policy.yaml")
	if err := os.WriteFile(routePath, []byte("rules: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rt, err := NewRuntime(gwPath, testLog())
	if err != nil {
		t.Fatal(err)
	}
	store := newMemStore()
	svc, err := rag.New(rag.Options{
		Store:        store,
		Embedder:     stubEmbedder{dim: 8},
		ChunkSize:    128,
		ChunkOverlap: 32,
		TopK:         4,
		EmbeddingDim: 8,
		Log:          testLog(),
	})
	if err != nil {
		t.Fatal(err)
	}
	rt.SetRAGForTest(svc)

	if _, err := svc.Ingest(context.Background(), rag.IngestRequest{
		Coords: vectorstore.Coords{TenantID: "tenantR", ProjectID: "default"},
		Source: "knowledge.md",
		Text:   strings.Repeat("retrieved-knowledge ", 50),
	}); err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(NewMux(rt, testLog(), nil, nil))
	t.Cleanup(srv.Close)
	return srv.URL, captured, rt
}

type capturedReqs struct {
	mu     sync.Mutex
	bodies [][]byte
}

func (c *capturedReqs) add(b []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.bodies = append(c.bodies, b)
}

func (c *capturedReqs) snapshot() [][]byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([][]byte, len(c.bodies))
	copy(out, c.bodies)
	return out
}

func TestV1Chat_echoesCorrelationHeaders(t *testing.T) {
	url, _, _ := setupRAGChatServer(t)
	body := `{"model":"Chimera-0.2.0","messages":[{"role":"user","content":"hi"}],"stream":false}`
	req, err := http.NewRequest(http.MethodPost, url+"/v1/chat/completions", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer rag-tok")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(headerConversationID, "client-conv-1")
	req.Header.Set(requestid.HeaderName, "req-hdr-1")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("status %d %s", res.StatusCode, b)
	}
	if got := res.Header.Get(headerConversationID); got != "client-conv-1" {
		t.Fatalf("conversation header: got %q", got)
	}
	if got := res.Header.Get(requestid.HeaderName); got != "req-hdr-1" {
		t.Fatalf("request id header: got %q", got)
	}
}

func TestVirtualModelChat_InjectsRetrievedContext(t *testing.T) {
	url, cap, _ := setupRAGChatServer(t)
	body := `{"model":"Chimera-0.2.0","messages":[{"role":"user","content":"tell me about retrieved-knowledge"}],"stream":false}`
	req, _ := http.NewRequest(http.MethodPost, url+"/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer rag-tok")
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("status %d %s", res.StatusCode, b)
	}

	bodies := cap.snapshot()
	if len(bodies) == 0 {
		t.Fatalf("upstream not called")
	}
	var sent struct {
		Model    string `json:"model"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(bodies[0], &sent); err != nil {
		t.Fatal(err)
	}
	if len(sent.Messages) < 2 {
		t.Fatalf("expected system+user, got %+v", sent.Messages)
	}
	if sent.Messages[0].Role != "system" || !strings.Contains(sent.Messages[0].Content, "Retrieved context") {
		t.Fatalf("missing retrieved-context system message: %+v", sent.Messages[0])
	}
	if !strings.Contains(sent.Messages[0].Content, "knowledge.md") {
		t.Fatalf("system message should cite source: %q", sent.Messages[0].Content)
	}
	// User message preserved as last.
	if sent.Messages[len(sent.Messages)-1].Role != "user" {
		t.Fatalf("last message should still be user")
	}
}

func TestVirtualModelChat_NoContextWhenRAGDisabled(t *testing.T) {
	t.Setenv("CHIMERA_UPSTREAM_API_KEY", "ukey")
	captured := &capturedReqs{}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			w.WriteHeader(http.StatusOK)
		case "/v1/chat/completions":
			b, _ := io.ReadAll(r.Body)
			captured.add(b)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"resp"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(upstream.Close)

	dir := t.TempDir()
	gwPath := filepath.Join(dir, "gateway.yaml")
	writeGateway(t, gwPath, upstream.URL, []string{"groq/m"})
	tokPath := filepath.Join(dir, "api-keys.yaml")
	writeTokens(t, tokPath, "tok", "ten")
	routePath := filepath.Join(dir, "routing-policy.yaml")
	_ = os.WriteFile(routePath, []byte("rules: []\n"), 0o644)
	rt, err := NewRuntime(gwPath, testLog())
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(NewMux(rt, testLog(), nil, nil))
	t.Cleanup(srv.Close)

	body := `{"model":"Chimera-0.1.0","messages":[{"role":"user","content":"hi"}],"stream":false}`
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer tok")
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d", res.StatusCode)
	}
	bodies := captured.snapshot()
	if len(bodies) == 0 {
		t.Fatal("upstream not called")
	}
	var sent struct {
		Messages []struct {
			Role string `json:"role"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(bodies[0], &sent); err != nil {
		t.Fatal(err)
	}
	if len(sent.Messages) != 1 || sent.Messages[0].Role != "user" {
		t.Fatalf("RAG-disabled path should not inject system message: %+v", sent.Messages)
	}
}
