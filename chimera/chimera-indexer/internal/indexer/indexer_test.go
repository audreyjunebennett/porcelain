package indexer

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type ingestRecord struct {
	Source  string
	Hash    string
	Body    string
	Project string
	Flavor  string
}

// fakeGateway implements the v0.2 indexer-facing surface the indexer relies
// on (config, ingest, health). Optional toggles let tests force flakiness.
type fakeGateway struct {
	mu             sync.Mutex
	ingest         []ingestRecord
	failOnce       map[string]int
	srv            *httptest.Server
	healthOK       atomic.Bool
	embedOK        atomic.Bool
	forceIngest502 atomic.Bool
	ingestCalls    atomic.Int32
}

func boolJSON(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

func newFakeGateway(t *testing.T) *fakeGateway {
	g := &fakeGateway{failOnce: map[string]int{}}
	g.healthOK.Store(true)
	g.embedOK.Store(true)
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/indexer/config", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"gateway_version":"v0.4","embedding_model":"m","embedding_dim":8,"chunk_size":512,"chunk_overlap":128,"ingest_path":"/v1/ingest","max_ingest_bytes":1048576,"max_whole_file_bytes":1048576,"ingest_session_path":"/v1/ingest/session"}`))
	})
	mux.HandleFunc("/v1/indexer/storage/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		vsOK := g.healthOK.Load()
		embOK := g.embedOK.Load()
		allOK := vsOK && embOK
		status := "ok"
		if !allOK {
			status = "degraded"
		}
		embExtra := `"model":"m"`
		if !embOK {
			embExtra = `"reason_code":"embed_model_not_in_catalog","detail":"embedding down","model":"m"`
		} else if !vsOK {
			embExtra = `"model":"m"`
		}
		body := `{"ok":` + boolJSON(allOK) + `,"status":"` + status + `","checks":{"vectorstore":{"ok":` + boolJSON(vsOK) + `},"embedding":{"ok":` + boolJSON(embOK) + `,` + embExtra + `}}}`
		_, _ = w.Write([]byte(body))
	})
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if !g.healthOK.Load() || !g.embedOK.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"degraded":true,"status":"degraded"}`))
			return
		}
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	mux.HandleFunc("/v1/indexer/corpus/inventory", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"indexer.corpus.inventory","entries":[],"has_more":false,"next_cursor":""}`))
	})
	mux.HandleFunc("/v1/ingest", func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Content-Type"), "application/json") {
			http.Error(w, "manifest required", http.StatusBadRequest)
			return
		}
		var manifest IngestManifest
		if err := json.NewDecoder(r.Body).Decode(&manifest); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		rec := ingestRecord{
			Source:  manifest.Source,
			Hash:    manifest.ClientContentHash,
			Project: r.Header.Get("X-Chimera-Project"),
			Flavor:  r.Header.Get("X-Chimera-Flavor-Id"),
		}
		for _, c := range manifest.Chunks {
			rec.Body += c.Text
		}
		g.ingestCalls.Add(1)
		if g.forceIngest502.Load() || !g.embedOK.Load() {
			http.Error(w, "embed: connection refused", http.StatusBadGateway)
			return
		}
		g.mu.Lock()
		if remaining, ok := g.failOnce[rec.Source]; ok && remaining > 0 {
			g.failOnce[rec.Source] = remaining - 1
			g.mu.Unlock()
			http.Error(w, "busy", http.StatusServiceUnavailable)
			return
		}
		g.ingest = append(g.ingest, rec)
		g.mu.Unlock()
		sha := manifest.ContentSHA256
		nChunks := len(manifest.Chunks)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"ingest.result","tenant_id":"t","project_id":"default","flavor_id":"_","source":"` + rec.Source + `","content_hash":"` + sha + `","content_sha256":"` + sha + `","chunks":` + strconv.Itoa(nChunks) + `,"collection":"c"}`))
	})
	g.srv = httptest.NewServer(mux)
	t.Cleanup(g.srv.Close)
	return g
}

func (g *fakeGateway) seenSources() []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	out := make([]string, 0, len(g.ingest))
	for _, r := range g.ingest {
		out = append(out, r.Source)
	}
	sort.Strings(out)
	return out
}

func TestIndexer_OneShotIngestsScannedFiles(t *testing.T) {
	g := newFakeGateway(t)
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "src", "main.go"), "package main\n")
	mustWrite(t, filepath.Join(root, "docs", "readme.md"), "# hi\n")
	mustWrite(t, filepath.Join(root, ".env"), "SECRET=1\n") // ignored

	cfg := Resolved{
		GatewayURL:           g.srv.URL,
		Token:                "tok",
		Roots:                []Root{{ID: "r", AbsPath: root}},
		SyncStatePath:        filepath.Join(root, "sync.json"),
		RetryMaxAttempts:     3,
		RetryBaseDelay:       1 * time.Millisecond,
		RetryMaxDelay:        2 * time.Millisecond,
		RecoveryPollInterval: 5 * time.Millisecond,
		Workers:              2,
		QueueDepth:           16,
		MaxFileBytes:         1 << 20,
		RequestTimeout:       2 * time.Second,
		BinaryNullByteSample: 1024,
		BinaryNullByteRatio:  0.001,
	}
	ix := New(cfg, NewGatewayClient(cfg.GatewayURL, cfg.Token, cfg.RequestTimeout), nil)
	defer ix.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if !ix.ScheduleInitialScan() {
		t.Fatal("schedule initial scan")
	}
	done := make(chan struct{})
	go func() {
		ix.RunWorkers(ctx)
		close(done)
	}()
	// Do not use queue length as a completion signal: the scan job can dequeue
	// (making the queue empty) and *then* enqueue fan-out work. Instead, wait
	// for the expected ingests to arrive.
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); {
		if got := g.seenSources(); len(got) == 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	ix.Queue().Close()
	<-done

	got := g.seenSources()
	want := []string{"docs/readme.md", "src/main.go"}
	if len(got) != len(want) {
		t.Fatalf("got=%v want=%v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("got=%v want=%v", got, want)
		}
	}
	for _, r := range g.ingest {
		if !strings.HasPrefix(r.Hash, "sha256:") {
			t.Fatalf("missing sha256 prefix: %+v", r)
		}
		if filepath.IsAbs(r.Source) || strings.Contains(r.Source, root) {
			t.Fatalf("absolute path leaked into source: %+v", r)
		}
	}
}

func TestIndexer_RetriesTransientFailures(t *testing.T) {
	g := newFakeGateway(t)
	g.failOnce["a.txt"] = 2 // first two attempts return 503
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.txt"), "alpha\n")

	cfg := Resolved{
		GatewayURL: g.srv.URL, Token: "tok",
		Roots:                []Root{{ID: "r", AbsPath: root}},
		SyncStatePath:        filepath.Join(root, "sync.json"),
		RetryMaxAttempts:     5,
		RetryBaseDelay:       1 * time.Millisecond,
		RetryMaxDelay:        2 * time.Millisecond,
		RecoveryPollInterval: 5 * time.Millisecond,
		Workers:              1, QueueDepth: 4, MaxFileBytes: 1 << 20,
		RequestTimeout:       2 * time.Second,
		BinaryNullByteSample: 1024, BinaryNullByteRatio: 0.001,
	}
	ix := New(cfg, NewGatewayClient(cfg.GatewayURL, cfg.Token, cfg.RequestTimeout), nil)
	defer ix.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if !ix.ScheduleInitialScan() {
		t.Fatal("schedule initial scan")
	}
	done := make(chan struct{})
	go func() { ix.RunWorkers(ctx); close(done) }()
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); {
		if len(g.seenSources()) > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	ix.Queue().Close()
	<-done
	if got := g.seenSources(); len(got) != 1 || got[0] != "a.txt" {
		t.Fatalf("got=%v", got)
	}
}

func TestIndexer_PausesAndResumesOnHealth(t *testing.T) {
	g := newFakeGateway(t)
	g.failOnce["a.txt"] = 100 // far more than retries; force ErrPaused
	g.healthOK.Store(false)
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.txt"), "alpha\n")

	cfg := Resolved{
		GatewayURL: g.srv.URL, Token: "tok",
		Roots:                []Root{{ID: "r", AbsPath: root}},
		SyncStatePath:        filepath.Join(root, "sync.json"),
		RetryMaxAttempts:     2,
		RetryBaseDelay:       1 * time.Millisecond,
		RetryMaxDelay:        2 * time.Millisecond,
		RecoveryPollInterval: 20 * time.Millisecond,
		Workers:              1, QueueDepth: 4, MaxFileBytes: 1 << 20,
		RequestTimeout:       2 * time.Second,
		BinaryNullByteSample: 1024, BinaryNullByteRatio: 0.001,
		RecoveryIncludeRootHealth: true,
	}
	ix := New(cfg, NewGatewayClient(cfg.GatewayURL, cfg.Token, cfg.RequestTimeout), nil)
	defer ix.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	if !ix.ScheduleInitialScan() {
		t.Fatal("schedule initial scan")
	}
	done := make(chan struct{})
	go func() { ix.RunWorkers(ctx); close(done) }()

	time.Sleep(200 * time.Millisecond)
	g.mu.Lock()
	g.failOnce["a.txt"] = 0
	g.mu.Unlock()
	g.healthOK.Store(true)

	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); {
		if len(g.seenSources()) > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	ix.Queue().Close()
	<-done
	if got := g.seenSources(); len(got) != 1 || got[0] != "a.txt" {
		t.Fatalf("got=%v after recovery", got)
	}
}

func TestIndexer_ShortCircuitsEmbed502(t *testing.T) {
	g := newFakeGateway(t)
	g.forceIngest502.Store(true)
	g.embedOK.Store(false)
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.txt"), "alpha\n")

	cfg := Resolved{
		GatewayURL: g.srv.URL, Token: "tok",
		Roots:                []Root{{ID: "r", AbsPath: root}},
		SyncStatePath:        filepath.Join(root, "sync.json"),
		RetryMaxAttempts:     5,
		RetryBaseDelay:       1 * time.Millisecond,
		RetryMaxDelay:        2 * time.Millisecond,
		RecoveryPollInterval: 20 * time.Millisecond,
		Workers:              1, QueueDepth: 4, MaxFileBytes: 1 << 20,
		RequestTimeout:            2 * time.Second,
		RecoveryIncludeRootHealth: false,
		RetryShortCircuitOnEmbed:  true,
		BinaryNullByteSample:      1024,
		BinaryNullByteRatio:       0.001,
	}
	ix := New(cfg, NewGatewayClient(cfg.GatewayURL, cfg.Token, cfg.RequestTimeout), nil)
	defer ix.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if !ix.ScheduleInitialScan() {
		t.Fatal("schedule initial scan")
	}
	done := make(chan struct{})
	go func() { ix.RunWorkers(ctx); close(done) }()

	time.Sleep(300 * time.Millisecond)
	ix.Queue().Close()
	<-done
	if n := g.ingestCalls.Load(); n != 1 {
		t.Fatalf("expected one ingest attempt before short-circuit, got %d", n)
	}
	if !ix.ingestGate.isClosed() {
		t.Fatal("expected ingest gate closed")
	}
}

func TestIndexer_WaitsForEmbeddingRecovery(t *testing.T) {
	g := newFakeGateway(t)
	g.healthOK.Store(true)
	g.embedOK.Store(false)
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.txt"), "alpha\n")

	cfg := Resolved{
		GatewayURL: g.srv.URL, Token: "tok",
		Roots:                []Root{{ID: "r", AbsPath: root}},
		SyncStatePath:        filepath.Join(root, "sync.json"),
		RetryMaxAttempts:     2,
		RetryBaseDelay:       1 * time.Millisecond,
		RetryMaxDelay:        2 * time.Millisecond,
		RecoveryPollInterval: 15 * time.Millisecond,
		Workers:              1, QueueDepth: 4, MaxFileBytes: 1 << 20,
		RequestTimeout:            2 * time.Second,
		RecoveryIncludeRootHealth: false,
		RetryShortCircuitOnEmbed:  true,
		BinaryNullByteSample:      1024,
		BinaryNullByteRatio:       0.001,
	}
	ix := New(cfg, NewGatewayClient(cfg.GatewayURL, cfg.Token, cfg.RequestTimeout), nil)
	defer ix.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	if !ix.ScheduleInitialScan() {
		t.Fatal("schedule initial scan")
	}
	done := make(chan struct{})
	go func() { ix.RunWorkers(ctx); close(done) }()

	time.Sleep(120 * time.Millisecond)
	g.embedOK.Store(true)

	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); {
		if len(g.seenSources()) > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	ix.Queue().Close()
	<-done
	if got := g.seenSources(); len(got) != 1 || got[0] != "a.txt" {
		t.Fatalf("got=%v after embed recovery", got)
	}
}

// ensure porcelain/chimera/chimera-indexer is buildable in CI without a network dep.
func TestPackageImportable(t *testing.T) {
	// Touching a public symbol keeps coverage honest.
	_ = os.Getenv
	_ = NewGatewayClient
	_ = New
}

func TestIndexer_IngestSendsScopedHeaders(t *testing.T) {
	g := newFakeGateway(t)
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "src", "main.go"), "package main\n")
	mustWrite(t, filepath.Join(root, "docs", "readme.md"), "# hi\n")

	cfg := Resolved{
		GatewayURL:           g.srv.URL,
		Token:                "tok",
		Roots:                []Root{{ID: "r", AbsPath: root, Scope: ScopeFragment{ProjectID: "svc", FlavorID: "base"}}},
		SyncStatePath:        filepath.Join(root, "sync.json"),
		DefaultScope:         ScopeFragment{ProjectID: "ignored", FlavorID: "ignored"},
		GlobOverrides:        []GlobOverride{{Pattern: "**/*.md", Scope: ScopeFragment{FlavorID: "docs"}}},
		RetryMaxAttempts:     3,
		RetryBaseDelay:       1 * time.Millisecond,
		RetryMaxDelay:        2 * time.Millisecond,
		RecoveryPollInterval: 5 * time.Millisecond,
		Workers:              2,
		QueueDepth:           16,
		MaxFileBytes:         1 << 20,
		RequestTimeout:       2 * time.Second,
		BinaryNullByteSample: 1024,
		BinaryNullByteRatio:  0.001,
	}
	ix := New(cfg, NewGatewayClient(cfg.GatewayURL, cfg.Token, cfg.RequestTimeout), nil)
	defer ix.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if !ix.ScheduleInitialScan() {
		t.Fatal("schedule initial scan")
	}
	done := make(chan struct{})
	go func() {
		ix.RunWorkers(ctx)
		close(done)
	}()
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); {
		if ix.Queue().Len() == 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	ix.Queue().Close()
	<-done

	g.mu.Lock()
	defer g.mu.Unlock()
	for _, rec := range g.ingest {
		wantFlavor := "base"
		if strings.HasSuffix(rec.Source, ".md") {
			wantFlavor = "docs"
		}
		if rec.Project != "svc" || rec.Flavor != wantFlavor {
			t.Fatalf("ingest %q: project=%q flavor=%q want project=svc flavor=%s", rec.Source, rec.Project, rec.Flavor, wantFlavor)
		}
	}
}
