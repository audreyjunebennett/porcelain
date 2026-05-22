package line

import (
	"strings"
	"testing"
	"time"
)

func TestNormalizeTimestampUTCEmpty(t *testing.T) {
	got := NormalizeTimestampUTC("")
	if strings.TrimSpace(got) == "" {
		t.Fatal("expected non-empty timestamp")
	}
}

func TestNormalizeTimestampUTCConvertsLocalOffset(t *testing.T) {
	got := NormalizeTimestampUTC("2026-05-18T21:19:38.6562002-05:00")
	want := "2026-05-19T02:19:38Z"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestNormalizeTimestampUTCTruncatesToSeconds(t *testing.T) {
	in := "2026-05-19T02:19:38.653148Z"
	got := NormalizeTimestampUTC(in)
	want := "2026-05-19T02:19:38Z"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestUTCTimestampNowParseable(t *testing.T) {
	got := UTCTimestampNow()
	if _, err := time.Parse(time.RFC3339, got); err != nil {
		t.Fatalf("parse: %v", err)
	}
}
