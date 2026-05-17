package gatewaymetrics

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"
)

func TestUsageForModelWindow_sumsCallsAndTokensInRange(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(filepath.Join(dir, "m.sqlite"), testMigrationsDir(t), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	base := time.Date(2026, 4, 16, 12, 0, 0, 0, time.UTC)
	// Three events for the target model across 2 minutes, plus an event for a different model.
	store.RecordUpstreamResponse(base, "groq/fast", 200, 100)
	store.RecordUpstreamResponse(base.Add(30*time.Second), "groq/fast", 429, 50)
	store.RecordUpstreamResponse(base.Add(2*time.Minute), "groq/fast", 200, 200)
	store.RecordUpstreamResponse(base, "openai/o", 200, 999)

	ctx := context.Background()

	// Minute window: [12:00, 12:01) → two calls, 150 tokens.
	u, err := store.UsageForModelWindow(ctx, "groq/fast", base, base.Add(time.Minute))
	if err != nil {
		t.Fatalf("window: %v", err)
	}
	if u.Calls != 2 || u.EstTokens != 150 {
		t.Fatalf("minute window: %+v", u)
	}

	// Day window: [00:00, 24:00) UTC → three calls, 350 tokens for groq/fast.
	dayStart := time.Date(2026, 4, 16, 0, 0, 0, 0, time.UTC)
	u2, err := store.UsageForModelWindow(ctx, "groq/fast", dayStart, dayStart.Add(24*time.Hour))
	if err != nil {
		t.Fatalf("day: %v", err)
	}
	if u2.Calls != 3 || u2.EstTokens != 350 {
		t.Fatalf("day window: %+v", u2)
	}

	// Next-day window is empty.
	nextDay := dayStart.Add(24 * time.Hour)
	u3, err := store.UsageForModelWindow(ctx, "groq/fast", nextDay, nextDay.Add(24*time.Hour))
	if err != nil {
		t.Fatalf("next day: %v", err)
	}
	if u3.Calls != 0 || u3.EstTokens != 0 {
		t.Fatalf("next day should be empty: %+v", u3)
	}
}
