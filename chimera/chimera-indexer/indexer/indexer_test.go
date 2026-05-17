package indexer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
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
	mu       sync.Mutex
	ingest   []ingestRecord
	failOnce map[string]int
	srv      *httptest.Server
	healthOK atomic.Bool
}

func newFakeGateway(t *testing.T) *fakeGateway {
	g := &fakeGateway{failOnce: map[string]int{}}
	g.healthOK.Store(true)
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/indexer/config", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"gateway_version":"v0.4","embedding_model":"m","embedding_dim":8,"chunk_size":512,"chunk_overlap":128,"ingest_path":"/v1/ingest","max_ingest_bytes":1048576,"max_whole_file_bytes":1048576,"ingest_session_path":"/v1/ingest/session"}`))
	})
	mux.HandleFunc("/v1/indexer/storage/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if g.healthOK.Load() {
			_, _ = w.Write([]byte(`{"ok":true,"status":"ready"}`))
			return
		}
		_, _ = w.Write([]byte(`{"ok":false,"status":"down"}`))
	})
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if !g.healthOK.Load() {
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
		mt, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || !strings.HasPrefix(mt, "multipart/") {
			http.Error(w, "bad ct", http.StatusBadRequest)
			return
		}
		mr := multipart.NewReader(r.Body, params["boundary"])
		rec := ingestRecord{
			Project: r.Header.Get("X-Chimera-Project"),
			Flavor:  r.Header.Get("X-Chimera-Flavor-Id"),
		}
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
				rec.Source = string(b)
			case "content_hash":
				rec.Hash = string(b)
			case "file":
				rec.Body = string(b)
			}
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
		sum := sha256.Sum256([]byte(rec.Body))
		sha := "sha256:" + hex.EncodeToString(sum[:])
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"ingest.result","tenant_id":"t","project_id":"default","flavor_id":"_","source":"` + rec.Source + `","content_hash":"` + sha + `","content_sha256":"` + sha + `","chunks":1,"collection":"c"}`))
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
