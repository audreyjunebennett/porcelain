package server

import (
	"context"
	"github.com/lynn/porcelain/internal/naming"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/adminui"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/catalog"
	"github.com/lynn/porcelain/chimera/internal/config"
)

func newTestResolved(upstreamURL string) *config.Resolved {
	return &config.Resolved{
		Semver:               "0.1.0",
		VirtualModelID:       "Chimera-0.1.0",
		UpstreamBaseURL:      upstreamURL,
		HealthTimeoutMs:      2000,
		FilterFreeTierModels: false,
	}
}

func TestBuildCatalogSnapshot_collectsProvidersAndModels(t *testing.T) {
	t.Parallel()
	chimeraBroker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"object": "list",
			"data": [
				{"id": "groq/llama3-70b", "object": "model"},
				{"id": "groq/llama3-8b", "object": "model"},
				{"id": "gemini/gemini-pro", "object": "model"},
				{"id": "ollama/qwen3:8b", "object": "model"}
			]
		}`))
	}))
	t.Cleanup(chimeraBroker.Close)

	res := newTestResolved(chimeraBroker.URL)
	snap := BuildCatalogSnapshot(context.Background(), res, "ukey", 2*time.Second, nil)
	if snap == nil || !snap.OK {
		t.Fatalf("snapshot ok=false: %+v", snap)
	}
	if snap.CatalogModelCount != 5 { // 1 virtual + 4 upstream
		t.Fatalf("catalog_model_count=%d want 5", snap.CatalogModelCount)
	}
	if got := snap.Providers; len(got) != 3 || got[0] != "gemini" || got[1] != "groq" || got[2] != "ollama" {
		t.Fatalf("providers=%v want [gemini groq ollama]", got)
	}
	for _, want := range []string{"groq", "gemini", "ollama"} {
		if !snap.HasProvider(want) {
			t.Fatalf("HasProvider(%q)=false", want)
		}
	}
	if !snap.HasModel("ollama/qwen3:8b") {
		t.Fatalf("HasModel(ollama/qwen3:8b)=false")
	}
	if !snap.IsFresh(time.Now(), CatalogSnapshotFreshness) {
		t.Fatalf("snapshot should be fresh")
	}
}

func TestBuildCatalogSnapshot_ollamaOfflineDropsProvider(t *testing.T) {
	t.Parallel()
	// BiFrost dynamically prunes providers from /v1/models when their upstream is unreachable.
	// This fixture mimics "ollama daemon offline": no ollama/* ids appear in the response.
	chimeraBroker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"object": "list",
			"data": [
				{"id": "groq/llama3-70b", "object": "model"},
				{"id": "gemini/gemini-pro", "object": "model"}
			]
		}`))
	}))
	t.Cleanup(chimeraBroker.Close)

	res := newTestResolved(chimeraBroker.URL)
	snap := BuildCatalogSnapshot(context.Background(), res, "ukey", 2*time.Second, nil)
	if snap == nil || !snap.OK {
		t.Fatalf("snapshot ok=false: %+v", snap)
	}
	if snap.HasProvider("ollama") {
		t.Fatalf("ollama should be absent from live catalog: %v", snap.Providers)
	}
}

func TestBuildCatalogSnapshot_fetchFailureProducesErrorSnapshot(t *testing.T) {
	t.Parallel()
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"err":"bad"}`))
	}))
	dead.Close() // immediately close so dial fails

	res := newTestResolved(dead.URL)
	snap := BuildCatalogSnapshot(context.Background(), res, "ukey", 500*time.Millisecond, nil)
	if snap == nil {
		t.Fatal("snapshot is nil")
	}
	if snap.OK {
		t.Fatalf("ok should be false: %+v", snap)
	}
	if snap.FetchErr == "" {
		t.Fatalf("fetch_err should be set: %+v", snap)
	}
	if snap.HasProvider("groq") {
		t.Fatal("HasProvider on failed snapshot should be false")
	}
}

func TestBuildCatalogSnapshot_emptyAPIKeyShortCircuits(t *testing.T) {
	t.Parallel()
	res := newTestResolved("http://example.invalid")
	snap := BuildCatalogSnapshot(context.Background(), res, "", 100*time.Millisecond, nil)
	if snap.OK {
		t.Fatalf("ok should be false with empty api key: %+v", snap)
	}
	if snap.FetchErr == "" {
		t.Fatal("fetch_err should describe missing key")
	}
}

func TestCatalogSnapshot_freshnessWindow(t *testing.T) {
	t.Parallel()
	now := time.Now()
	stale := &CatalogSnapshot{FetchedAt: now.Add(-3 * time.Minute), OK: true}
	if stale.IsFresh(now, CatalogSnapshotFreshness) {
		t.Fatal("3-minute-old snapshot should be stale at default 2-minute window")
	}
	fresh := &CatalogSnapshot{FetchedAt: now.Add(-30 * time.Second), OK: true}
	if !fresh.IsFresh(now, CatalogSnapshotFreshness) {
		t.Fatal("30-second-old snapshot should be fresh")
	}
	var nilSnap *CatalogSnapshot
	if nilSnap.IsFresh(now, CatalogSnapshotFreshness) {
		t.Fatal("nil snapshot must not report fresh")
	}
}

func TestRefreshAvailableModels_storesSnapshotAndRunsAuditors(t *testing.T) {
	chimeraBroker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"groq/m"},{"id":"gemini/n"}]}`))
	}))
	t.Cleanup(chimeraBroker.Close)

	t.Setenv(naming.EnvBrokerAPIKeyTarget, "ukey")
	rt := runtimeForCatalogTest(t, chimeraBroker.URL)

	defer clearCatalogAuditors()
	clearCatalogAuditors() // make sure no auditor lingers from earlier tests

	var calls atomic.Int32
	var sawProviders atomic.Bool
	RegisterCatalogAuditor(func(ctx context.Context, snap *CatalogSnapshot, res *config.Resolved, log *slog.Logger) {
		calls.Add(1)
		if snap != nil && snap.OK && len(snap.Providers) >= 2 {
			sawProviders.Store(true)
		}
	})

	snap := RefreshAvailableModels(context.Background(), rt, testLog())
	if snap == nil || !snap.OK {
		t.Fatalf("snapshot ok=false: %+v", snap)
	}
	if got := rt.CatalogSnapshot(); got == nil || !got.OK {
		t.Fatalf("runtime snapshot not stored: %+v", got)
	}
	if calls.Load() != 1 {
		t.Fatalf("auditor calls=%d want 1", calls.Load())
	}
	if !sawProviders.Load() {
		t.Fatal("auditor saw empty providers slice")
	}
}

func TestRefreshAvailableModels_recoversFromAuditorPanic(t *testing.T) {
	chimeraBroker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"groq/m"}]}`))
	}))
	t.Cleanup(chimeraBroker.Close)
	t.Setenv(naming.EnvBrokerAPIKeyTarget, "ukey")
	rt := runtimeForCatalogTest(t, chimeraBroker.URL)

	defer clearCatalogAuditors()
	clearCatalogAuditors()

	RegisterCatalogAuditor(func(ctx context.Context, snap *CatalogSnapshot, res *config.Resolved, log *slog.Logger) {
		panic("boom")
	})
	var sawSecond atomic.Bool
	RegisterCatalogAuditor(func(ctx context.Context, snap *CatalogSnapshot, res *config.Resolved, log *slog.Logger) {
		sawSecond.Store(true)
	})

	snap := RefreshAvailableModels(context.Background(), rt, testLog())
	if snap == nil || !snap.OK {
		t.Fatalf("snapshot ok=false: %+v", snap)
	}
	if !sawSecond.Load() {
		t.Fatal("second auditor should still run after first auditor panic")
	}
}

func TestClassifyBrokerProviderResult_liveCatalogOverride(t *testing.T) {
	t.Parallel()
	// Provider config says "ollama is up (base_url present)" but the live catalog has only
	// groq + gemini → classifier must downgrade ollama to "down".
	live := buildSnapshotForTest(time.Now(), []string{"gemini", "groq"})
	body := []byte(`{"name":"ollama","keys":[],"network_config":{"base_url":"http://127.0.0.1:11434"}}`)
	got := adminui.ClassifyBrokerProviderResult("ollama", body, 200, nil, live)
	if got.State != "down" {
		t.Fatalf("state=%q want down (live override): %+v", got.State, got)
	}
	if got.Error == "" {
		t.Fatalf("error annotation should be set: %+v", got)
	}
	bodyGroq := []byte(`{"name":"groq","keys":[{"name":"k","value":"env.GROQ_API_KEY"}]}`)
	gotGroq := adminui.ClassifyBrokerProviderResult("groq", bodyGroq, 200, nil, live)
	if gotGroq.State != "up" {
		t.Fatalf("groq state=%q want up", gotGroq.State)
	}
}

func TestClassifyBrokerProviderResult_staleSnapshotDoesNotOverride(t *testing.T) {
	t.Parallel()
	stale := buildSnapshotForTest(time.Now().Add(-10*time.Minute), []string{"groq"})
	body := []byte(`{"name":"ollama","keys":[],"network_config":{"base_url":"http://127.0.0.1:11434"}}`)
	got := adminui.ClassifyBrokerProviderResult("ollama", body, 200, nil, stale)
	if got.State != "unknown" {
		t.Fatalf("stale snapshot must not confirm liveness; got %q", got.State)
	}
}

func TestClassifyBrokerProviderResult_failedSnapshotMarksConfiguredDown(t *testing.T) {
	t.Parallel()
	failed := &CatalogSnapshot{FetchedAt: time.Now(), OK: false, FetchErr: "boom"}
	body := []byte(`{"name":"ollama","keys":[],"network_config":{"base_url":"http://127.0.0.1:11434"}}`)
	got := adminui.ClassifyBrokerProviderResult("ollama", body, 200, nil, failed)
	if got.State != "down" {
		t.Fatalf("fresh failed catalog poll should mark configured provider down; got %q", got.State)
	}
	if got.Error == "" {
		t.Fatalf("error annotation should be set: %+v", got)
	}
}

func TestClassifyBrokerProviderResult_overrideOnlyAffectsUp(t *testing.T) {
	t.Parallel()
	// gemini has no key → state=key_missing. Live catalog also lacks gemini, but the override
	// must NOT promote/demote a key_missing into "down" (it already explains the failure mode).
	live := buildSnapshotForTest(time.Now(), []string{"groq"})
	body := []byte(`{"name":"gemini","keys":[]}`)
	got := adminui.ClassifyBrokerProviderResult("gemini", body, 200, nil, live)
	if got.State != "key_missing" {
		t.Fatalf("state=%q want key_missing", got.State)
	}
}

// ---------- helpers ----------

func buildSnapshotForTest(at time.Time, providers []string) *CatalogSnapshot {
	return catalog.NewTestSnapshot(at, providers)
}

func clearCatalogAuditors() {
	catalog.ResetAuditorsForTest()
}
