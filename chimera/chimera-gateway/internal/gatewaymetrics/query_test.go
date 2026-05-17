package gatewaymetrics

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestStore_QueryRollups(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "m.sqlite")
	st, err := Open(dbPath, testMigrationsDir(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	at := time.Date(2026, 1, 10, 12, 5, 0, 0, time.UTC)
	st.RecordUpstreamResponse(at, "groq/a", 200, 10)
	st.RecordUpstreamResponse(at, "groq/b", 429, 5)

	ctx := context.Background()
	minRows, err := st.QueryMinuteRollups(ctx, "2026-01-10T12:05", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(minRows) < 2 {
		t.Fatalf("minute rows: %+v", minRows)
	}
	dayRows, err := st.QueryDayRollups(ctx, "2026-01-10", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(dayRows) < 2 {
		t.Fatalf("day rows: %+v", dayRows)
	}
	ev, err := st.QueryRecentEvents(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(ev) != 2 {
		t.Fatalf("events len=%d", len(ev))
	}
}
