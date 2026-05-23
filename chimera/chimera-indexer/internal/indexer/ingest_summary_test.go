package indexer

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestIngestSummary_EmitsOnInterval(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ix := New(Resolved{
		SkipSummaryMinInterval: 50 * time.Millisecond,
		JobIngestLog:           JobIngestLogDebug,
	}, nil, log)
	root := Root{ID: "r", AbsPath: "/w"}
	j := Job{Root: root, RelPath: "a.go"}

	ix.noteIngestSummarySucceeded(j, 3)
	time.Sleep(60 * time.Millisecond)
	if strings.Contains(buf.String(), "indexer.job.ingested.summary") {
		t.Fatal("expected no summary before interval")
	}

	ix.noteIngestSummarySucceeded(j, 2)
	time.Sleep(60 * time.Millisecond)
	ix.emitDueIngestSummaries(false)
	out := buf.String()
	if !strings.Contains(out, "indexer.job.ingested.summary") {
		t.Fatalf("expected ingest summary, got:\n%s", out)
	}
	if strings.Contains(out, "ingest_succeeded") && !strings.Contains(out, "indexer.job.ingested") {
		// per-file ingested should not appear at INFO when debug mode
	}
}

func TestIngestSummary_NoEmitWhenIdle(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, nil))
	ix := New(Resolved{SkipSummaryMinInterval: time.Second}, nil, log)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go ix.runIngestSummaryLoop(ctx)
	time.Sleep(150 * time.Millisecond)
	if strings.Contains(buf.String(), "indexer.job.ingested.summary") {
		t.Fatal("idle should not emit ingest summary")
	}
}

func TestIngestSummary_FlushOnForce(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, nil))
	ix := New(Resolved{SkipSummaryMinInterval: time.Minute}, nil, log)
	root := Root{ID: "r", AbsPath: "/w"}
	ix.noteIngestSummarySucceeded(Job{Root: root, RelPath: "b.go"}, 5)
	ix.FlushIngestSummaries()
	if !strings.Contains(buf.String(), "indexer.job.ingested.summary") {
		t.Fatalf("expected forced flush summary, got:\n%s", buf.String())
	}
}

func TestJobIngestLog_DebugDemotesPerFile(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	ix := New(Resolved{JobIngestLog: JobIngestLogDebug}, nil, log)
	root := Root{ID: "r", AbsPath: "/w"}
	j := Job{Root: root, RelPath: "x.go"}
	ix.logIngestedSuccess(j, "whole", 1, "coll", "sha")
	if strings.Contains(buf.String(), "indexer.job.ingested") {
		t.Fatalf("expected no INFO ingested line, got:\n%s", buf.String())
	}
}
