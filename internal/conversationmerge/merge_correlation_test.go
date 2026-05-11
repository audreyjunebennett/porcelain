package conversationmerge

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/lynn/claudia-gateway/internal/config"
	"github.com/lynn/claudia-gateway/internal/gatewaymetrics"

	_ "modernc.org/sqlite"
)

func testGatewayMigrationsDirMerge(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "migrations", "gateway"))
}

func TestMergeCorrelationAttrs(t *testing.T) {
	in := ResolveInput{TenantID: "ten-a", RequestID: "rid-9"}
	args := mergeCorrelationAttrs(in, "conv-x")
	m := map[string]string{}
	for i := 0; i+1 < len(args); i += 2 {
		k, _ := args[i].(string)
		v, _ := args[i+1].(string)
		m[k] = v
	}
	if m["service"] != "gateway" || m["request_id"] != "rid-9" || m["principal_id"] != "ten-a" || m["conversation_id"] != "conv-x" {
		t.Fatalf("got %#v", m)
	}
}

func TestResolve_embedFailure_logsCorrelation(t *testing.T) {
	t.Parallel()
	embedSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embeddings" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(embedSrv.Close)

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "m.db")
	abs, err := filepath.Abs(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	dsn := "file:" + filepath.ToSlash(abs) + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := gatewaymetrics.ApplyMigrations(db, testGatewayMigrationsDirMerge(t), nil); err != nil {
		t.Fatal(err)
	}

	var buf strings.Builder
	log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))

	cfg := config.ConversationMerge{
		Enabled:                   true,
		MatchThreshold:            0.99,
		RecentWindowMinutes:       10,
		CandidateLimit:            8,
		MaxIdleHours:              168,
		SessionAttachMinutes:      45,
		SessionShortFollowUpRunes: 6,
		SessionAttachMinCosine:    0.5,
	}
	ragCfg := config.RAG{
		Enabled:        true,
		QdrantURL:      "http://127.0.0.1:9",
		EmbeddingModel: "test-model",
		EmbeddingDim:   1536,
	}
	svc := NewService(cfg, db, embedSrv.URL, "api-key", ragCfg, log)
	if svc == nil {
		t.Fatal("NewService returned nil")
	}
	_, err = svc.Resolve(context.Background(), ResolveInput{
		TenantID:     "tenant-merge-test",
		ProjectID:    "p",
		FlavorID:     "",
		LastUserText: "hello from correlation test",
		RequestID:    "merge-test-req-id",
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "request_id=merge-test-req-id") {
		t.Fatalf("missing request_id in log:\n%s", out)
	}
	if !strings.Contains(out, "principal_id=tenant-merge-test") {
		t.Fatalf("missing principal_id in log:\n%s", out)
	}
	if !strings.Contains(out, "conversation_id=") {
		t.Fatalf("missing conversation_id in log:\n%s", out)
	}
}
