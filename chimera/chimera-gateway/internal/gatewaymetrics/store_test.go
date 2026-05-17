package gatewaymetrics

import (
	"path/filepath"
	"testing"
	"time"
)

func TestSplitProviderModel(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in, prov, full string
	}{
		{"groq/llama-3", "groq", "groq/llama-3"},
		{"gemini/gemini-3-flash-preview", "gemini", "gemini/gemini-3-flash-preview"},
		{"nope", "", "nope"},
		{"/onlyslash", "", "/onlyslash"},
	}
	for _, tc := range tests {
		p, f := SplitProviderModel(tc.in)
		if p != tc.prov || f != tc.full {
			t.Fatalf("SplitProviderModel(%q) = (%q,%q) want (%q,%q)", tc.in, p, f, tc.prov, tc.full)
		}
	}
}

func TestStore_RecordUpstreamResponse_rollups(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "metrics.sqlite")
	st, err := Open(dbPath, testMigrationsDir(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	at := time.Date(2026, 4, 16, 14, 37, 22, 0, time.UTC)
	st.RecordUpstreamResponse(at, "groq/llama-3.3-70b-versatile", 200, 42)
	st.RecordUpstreamResponse(at, "groq/llama-3.3-70b-versatile", 200, 10)
	st.RecordUpstreamResponse(at, "groq/llama-3.3-70b-versatile", 413, 100)

	var calls200, tok200, calls413 int
	err = st.db.QueryRow(`
SELECT calls, est_tokens FROM upstream_rollup_minute
WHERE provider='groq' AND model_id='groq/llama-3.3-70b-versatile' AND minute_utc='2026-04-16T14:37' AND status=200
`).Scan(&calls200, &tok200)
	if err != nil {
		t.Fatal(err)
	}
	if calls200 != 2 || tok200 != 52 {
		t.Fatalf("minute rollup 200: calls=%d tok=%d want 2,52", calls200, tok200)
	}
	err = st.db.QueryRow(`
SELECT calls FROM upstream_rollup_minute
WHERE provider='groq' AND model_id='groq/llama-3.3-70b-versatile' AND minute_utc='2026-04-16T14:37' AND status=413
`).Scan(&calls413)
	if err != nil {
		t.Fatal(err)
	}
	if calls413 != 1 {
		t.Fatalf("minute rollup 413 calls=%d want 1", calls413)
	}

	var dayCalls int
	err = st.db.QueryRow(`
SELECT calls FROM upstream_rollup_day
WHERE provider='groq' AND model_id='groq/llama-3.3-70b-versatile' AND day_utc='2026-04-16' AND status=200
`).Scan(&dayCalls)
	if err != nil {
		t.Fatal(err)
	}
	if dayCalls != 2 {
		t.Fatalf("day rollup 200 calls=%d want 2", dayCalls)
	}
}
