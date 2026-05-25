package indexer

import (
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func testQueueSnapshotLogger(t *testing.T) (*Indexer, *syncBuffer) {
	t.Helper()
	var buf syncBuffer
	log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	cfg := Resolved{
		Workers:                       4,
		QueueDepth:                    8,
		QueueSnapshotIdleInfoInterval: 5 * time.Minute,
	}
	ix := New(cfg, nil, log)
	t0 := time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	ix.hooks.Now = func() time.Time { return t0 }
	return ix, &buf
}

func logLevelLines(out string) (infoN, debugN int) {
	for _, line := range strings.Split(out, "\n") {
		if !strings.Contains(line, "indexer.queue.snapshot") {
			continue
		}
		if strings.Contains(line, "level=INFO") {
			infoN++
		}
		if strings.Contains(line, "level=DEBUG") {
			debugN++
		}
	}
	return infoN, debugN
}

func TestQueueSnapshot_lifecyclePhaseAlwaysInfo(t *testing.T) {
	ix, buf := testQueueSnapshotLogger(t)
	ix.LogQueueSnapshot("run_workers_start")
	out := buf.String()
	if !strings.Contains(out, "level=INFO") {
		t.Fatalf("expected INFO lifecycle snapshot: %q", out)
	}
}

func TestQueueSnapshot_idleUnchangedDemotesToDebug(t *testing.T) {
	ix, buf := testQueueSnapshotLogger(t)
	ix.LogQueueSnapshot("run_workers_start")
	ix.LogQueueSnapshot("worker_drain_tick")
	ix.LogQueueSnapshot("worker_drain_tick")

	infoN, debugN := logLevelLines(buf.String())
	if infoN != 1 {
		t.Fatalf("want one INFO (run_workers_start), got info=%d debug=%d body=%q", infoN, debugN, buf.String())
	}
	if debugN != 2 {
		t.Fatalf("want DEBUG for unchanged idle ticks, got info=%d debug=%d body=%q", infoN, debugN, buf.String())
	}
}

func TestQueueSnapshot_idleHeartbeatAfterInterval(t *testing.T) {
	ix, buf := testQueueSnapshotLogger(t)
	t0 := time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	ix.hooks.Now = func() time.Time { return t0 }
	ix.LogQueueSnapshot("worker_drain_tick")
	ix.LogQueueSnapshot("worker_drain_tick")

	ix.hooks.Now = func() time.Time { return t0.Add(5 * time.Minute) }
	ix.LogQueueSnapshot("worker_drain_tick")

	infoN, debugN := logLevelLines(buf.String())
	if infoN != 2 {
		t.Fatalf("want INFO on first tick and idle heartbeat, got info=%d debug=%d body=%q", infoN, debugN, buf.String())
	}
	if debugN != 1 {
		t.Fatalf("want one DEBUG between heartbeats, got info=%d debug=%d body=%q", infoN, debugN, buf.String())
	}
}

func TestQueueSnapshot_activeQueueStaysInfo(t *testing.T) {
	ix, buf := testQueueSnapshotLogger(t)
	root := Root{ID: "r", AbsPath: t.TempDir(), Scope: ScopeFragment{ProjectID: "p", FlavorID: "f"}}
	if !ix.queue.Enqueue(WorkItem{Kind: WorkIngest, Job: Job{Root: root, RelPath: "a.go"}}) {
		t.Fatal("enqueue failed")
	}
	ix.LogQueueSnapshot("worker_drain_tick")
	ix.LogQueueSnapshot("worker_drain_tick")

	infoN, debugN := logLevelLines(buf.String())
	if infoN != 2 {
		t.Fatalf("active queue should stay INFO, got info=%d debug=%d body=%q", infoN, debugN, buf.String())
	}
}

func TestQueueSnapshot_counterChangeWhileIdleStaysInfo(t *testing.T) {
	ix, buf := testQueueSnapshotLogger(t)
	ix.LogQueueSnapshot("worker_drain_tick")
	atomic.AddInt64(&ix.opsIngestOK, 1)
	ix.LogQueueSnapshot("worker_drain_tick")

	infoN, _ := logLevelLines(buf.String())
	if infoN != 2 {
		t.Fatalf("counter change should emit INFO, got info=%d body=%q", infoN, buf.String())
	}
}

func TestResolve_QueueSnapshotIdleInfoIntervalDefault(t *testing.T) {
	dir := t.TempDir()
	env := func(k string) string {
		if k == EnvGatewayToken {
			return "tok"
		}
		return ""
	}
	r, err := Resolve(FileConfig{GatewayURL: "http://x", Roots: FlexibleRoots{{Path: dir}}}, env, Overrides{})
	if err != nil {
		t.Fatal(err)
	}
	if r.QueueSnapshotIdleInfoInterval != 5*time.Minute {
		t.Fatalf("default interval=%v want 5m", r.QueueSnapshotIdleInfoInterval)
	}
}

func TestResolve_QueueSnapshotIdleInfoIntervalDisabled(t *testing.T) {
	dir := t.TempDir()
	env := func(k string) string {
		if k == EnvGatewayToken {
			return "tok"
		}
		return ""
	}
	r, err := Resolve(FileConfig{
		GatewayURL:                      "http://x",
		Roots:                           FlexibleRoots{{Path: dir}},
		QueueSnapshotIdleInfoIntervalMS: -1,
	}, env, Overrides{})
	if err != nil {
		t.Fatal(err)
	}
	if r.QueueSnapshotIdleInfoInterval != 0 {
		t.Fatalf("negative yaml should disable idle INFO, got %v", r.QueueSnapshotIdleInfoInterval)
	}
}
