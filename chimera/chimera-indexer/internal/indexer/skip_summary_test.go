package indexer

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestSkipSummary_EmitsOnInterval(t *testing.T) {
	var buf syncBuffer
	log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg := Resolved{
		SkipSummaryMinInterval: 50 * time.Millisecond,
		Workers:                1,
		QueueDepth:             4,
	}
	ix := New(cfg, nil, log)
	ix.hooks.Now = func() time.Time { return time.Unix(100, 0) }
	root := Root{ID: "r", AbsPath: "/tmp", Scope: ScopeFragment{ProjectID: "p", FlavorID: "f"}}
	j := Job{Root: root, RelPath: "a.go"}

	ix.noteSkipSummaryUnchanged(j, "unchanged_local_sync")
	ix.emitDueSkipSummaries(false)
	if strings.Contains(buf.String(), "indexer.job.skipped.summary") {
		t.Fatal("should not emit before min interval on first tick without force")
	}

	ix.hooks.Now = func() time.Time { return time.Unix(100, 0).Add(60 * time.Millisecond) }
	ix.noteSkipSummaryUnchanged(j, "unchanged_local_sync")
	ix.emitDueSkipSummaries(false)
	out := buf.String()
	if !strings.Contains(out, "indexer.job.skipped.summary") {
		t.Fatalf("expected summary emit, got %q", out)
	}
	if !strings.Contains(out, "skip_unchanged_local_sync") {
		t.Fatalf("missing skip delta: %q", out)
	}
}

func TestSkipSummary_NoEmitWhenIdle(t *testing.T) {
	var buf syncBuffer
	log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	ix := New(Resolved{SkipSummaryMinInterval: time.Second}, nil, log)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go ix.runSkipSummaryLoop(ctx)
	time.Sleep(1100 * time.Millisecond)
	cancel()
	if strings.Contains(buf.String(), "indexer.job.skipped.summary") {
		t.Fatalf("idle loop should not emit: %q", buf.String())
	}
}

func TestSkipSummary_FlushOnForce(t *testing.T) {
	var buf syncBuffer
	log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	ix := New(Resolved{SkipSummaryMinInterval: time.Minute}, nil, log)
	root := Root{ID: "r", AbsPath: "/tmp", Scope: ScopeFragment{ProjectID: "p", FlavorID: "f"}}
	ix.noteSkipSummaryEmpty(Job{Root: root, RelPath: "b.go"})
	ix.FlushSkipSummaries()
	if !strings.Contains(buf.String(), "indexer.job.skipped.summary") {
		t.Fatalf("flush should force emit: %q", buf.String())
	}
}

type syncBuffer struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

var _ io.Writer = (*syncBuffer)(nil)
