package chat

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/lynn/porcelain/chimera/chimera-gateway/gatewaymetrics"
)

func testMigrationsDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "migrations", "chimera-gateway", "metrics"))
}

func TestProxyChatCompletion_recordsMetrics(t *testing.T) {
	t.Parallel()
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"id":"x","choices":[]}`)
	}))
	t.Cleanup(up.Close)

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "metrics.sqlite")
	st, err := gatewaymetrics.Open(dbPath, testMigrationsDir(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	body := map[string]json.RawMessage{
		"model":    json.RawMessage(`"groq/llama-3.3-70b-versatile"`),
		"messages": json.RawMessage(`[{"role":"user","content":"Hello"}]`),
		"stream":   json.RawMessage(`false`),
	}
	w := httptest.NewRecorder()
	_ = ProxyChatCompletion(context.Background(), w, up.URL, "", "groq/llama-3.3-70b-versatile", false, body, time.Minute, nil, st, nil, nil)

	var calls int
	q := `SELECT calls FROM upstream_rollup_minute WHERE provider='groq' AND model_id='groq/llama-3.3-70b-versatile' AND status=200 ORDER BY minute_utc DESC LIMIT 1`
	if err := st.DB().QueryRow(q).Scan(&calls); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("rollup calls=%d want 1", calls)
	}
}
