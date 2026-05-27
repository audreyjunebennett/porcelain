package indexer

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func observationTestLogger(t *testing.T) (*Indexer, *syncBuffer) {
	t.Helper()
	var buf syncBuffer
	log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ix := New(Resolved{Workers: 1, QueueDepth: 4}, nil, log)
	ix.initialScanCompleted.Store(true)
	return ix, &buf
}

func observationLevelCounts(out, slug string) (infoN, warnN, debugN int) {
	for _, line := range strings.Split(out, "\n") {
		if !strings.Contains(line, slug) {
			continue
		}
		switch {
		case strings.Contains(line, "level=INFO"):
			infoN++
		case strings.Contains(line, "level=WARN"):
			warnN++
		case strings.Contains(line, "level=DEBUG"):
			debugN++
		}
	}
	return infoN, warnN, debugN
}

func TestStorageStatsObsLevel_unchangedDemotesToDebug(t *testing.T) {
	ix, _ := observationTestLogger(t)
	fp := storageStatsFingerprint(true, 100, 768, "", "coll-a")
	if got := ix.storageStatsObsLevel("ik1", fp); got != slog.LevelInfo {
		t.Fatalf("first=%v want INFO", got)
	}
	if got := ix.storageStatsObsLevel("ik1", fp); got != slog.LevelDebug {
		t.Fatalf("repeat=%v want DEBUG", got)
	}
}

func TestStateObsLevel_unchangedDemotesToDebug(t *testing.T) {
	ix, _ := observationTestLogger(t)
	fp := stateObservationFingerprint("watch_idle", 0, 0, true, true, false, 60397)
	if got := ix.stateObsLevel(fp); got != slog.LevelInfo {
		t.Fatalf("first=%v want INFO", got)
	}
	if got := ix.stateObsLevel(fp); got != slog.LevelDebug {
		t.Fatalf("repeat=%v want DEBUG", got)
	}
}

func TestStateObsLevel_changeEmitsInfo(t *testing.T) {
	ix, _ := observationTestLogger(t)
	fp1 := stateObservationFingerprint("watch_idle", 0, 0, true, true, false, 60397)
	fp2 := stateObservationFingerprint("watch_idle", 0, 0, true, true, false, 60400)
	if ix.stateObsLevel(fp1) != slog.LevelInfo {
		t.Fatal("first emit")
	}
	if ix.stateObsLevel(fp1) != slog.LevelDebug {
		t.Fatal("unchanged repeat")
	}
	if ix.stateObsLevel(fp2) != slog.LevelInfo {
		t.Fatal("changed points should INFO")
	}
}

func TestEmitStorageStatsAndState_missingCollectionRepeatsWarn(t *testing.T) {
	var buf syncBuffer
	log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	root := filepath.Join(t.TempDir(), "proj")
	statsBody, _ := json.Marshal(StorageStatsResponse{
		Object:     "indexer.storage.stats",
		Collection: "coll-a",
		Points:     0,
		VectorDim:  768,
		Available:  false,
		Detail:     "collection missing",
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/indexer/storage/stats" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(statsBody)
	}))
	t.Cleanup(srv.Close)

	client := NewGatewayClient(srv.URL, "tok", 2*time.Second)
	cfg := Resolved{
		GatewayURL: srv.URL,
		Workers:    1,
		QueueDepth: 4,
		Roots: []Root{{
			ID:      "r1",
			AbsPath: root,
			Scope:   ScopeFragment{ProjectID: "porcelain"},
		}},
	}
	ix := New(cfg, client, log)
	ix.initialScanCompleted.Store(true)
	gw := &IndexerConfig{TenantID: "lynn"}
	gw.Defaults.ProjectID = "porcelain"
	ix.lastGW.Store(gw)

	ctx := context.Background()
	ix.EmitStorageStatsAndState(ctx, true)
	ix.EmitStorageStatsAndState(ctx, true)

	statsInfo, statsWarn, statsDebug := observationLevelCounts(buf.String(), "indexer.storage.stats")
	if statsInfo != 0 || statsWarn != 2 || statsDebug != 0 {
		t.Fatalf("missing collection should repeat WARN: info=%d warn=%d debug=%d body=%q", statsInfo, statsWarn, statsDebug, buf.String())
	}
}

func TestEmitStorageStatsAndState_unchangedPollDemotes(t *testing.T) {
	var buf syncBuffer
	log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	root := filepath.Join(t.TempDir(), "proj")
	statsBody, _ := json.Marshal(StorageStatsResponse{
		Object:     "indexer.storage.stats",
		Collection: "coll-a",
		Points:     100,
		VectorDim:  768,
		Available:  true,
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/indexer/storage/stats" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(statsBody)
	}))
	t.Cleanup(srv.Close)

	client := NewGatewayClient(srv.URL, "tok", 2*time.Second)
	cfg := Resolved{
		GatewayURL: srv.URL,
		Workers:    1,
		QueueDepth: 4,
		Roots: []Root{{
			ID:      "r1",
			AbsPath: root,
			Scope:   ScopeFragment{ProjectID: "porcelain"},
		}},
	}
	ix := New(cfg, client, log)
	ix.initialScanCompleted.Store(true)
	gw := &IndexerConfig{TenantID: "lynn"}
	gw.Defaults.ProjectID = "porcelain"
	ix.lastGW.Store(gw)

	ctx := context.Background()
	ix.EmitStorageStatsAndState(ctx, true)
	ix.EmitStorageStatsAndState(ctx, true)

	statsInfo, statsWarn, statsDebug := observationLevelCounts(buf.String(), "indexer.storage.stats")
	stateInfo, _, stateDebug := observationLevelCounts(buf.String(), "indexer.state")

	if statsInfo != 1 || statsDebug != 1 || statsWarn != 0 {
		t.Fatalf("storage stats info=%d warn=%d debug=%d body=%q", statsInfo, statsWarn, statsDebug, buf.String())
	}
	if stateInfo != 1 || stateDebug != 1 {
		t.Fatalf("state info=%d debug=%d body=%q", stateInfo, stateDebug, buf.String())
	}
}

func TestEmitStorageStatsAndState_pointDeltaEmitsInfo(t *testing.T) {
	var buf syncBuffer
	log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	points := int64(100)
	root := filepath.Join(t.TempDir(), "proj")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := json.Marshal(StorageStatsResponse{
			Object: "indexer.storage.stats", Collection: "coll-a",
			Points: points, VectorDim: 768, Available: true,
		})
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	client := NewGatewayClient(srv.URL, "tok", 2*time.Second)
	cfg := Resolved{
		GatewayURL: srv.URL,
		Workers:    1,
		QueueDepth: 4,
		Roots: []Root{{
			ID: "r1", AbsPath: root, Scope: ScopeFragment{ProjectID: "porcelain"},
		}},
	}
	ix := New(cfg, client, log)
	ix.initialScanCompleted.Store(true)
	gw2 := &IndexerConfig{TenantID: "lynn"}
	gw2.Defaults.ProjectID = "porcelain"
	ix.lastGW.Store(gw2)

	ctx := context.Background()
	ix.EmitStorageStatsAndState(ctx, true)
	points = 101
	ix.EmitStorageStatsAndState(ctx, true)

	statsInfo, _, _ := observationLevelCounts(buf.String(), "indexer.storage.stats")
	stateInfo, _, _ := observationLevelCounts(buf.String(), "indexer.state")
	if statsInfo != 2 {
		t.Fatalf("expected INFO on point delta, statsInfo=%d body=%q", statsInfo, buf.String())
	}
	if stateInfo != 2 {
		t.Fatalf("expected INFO on total delta, stateInfo=%d body=%q", stateInfo, buf.String())
	}
}
