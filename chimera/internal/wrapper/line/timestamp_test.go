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
	if !strings.HasSuffix(got, "Z") {
		t.Fatalf("expected UTC Z suffix, got %q", got)
	}
}

func TestNormalizeTimestampUTCPreservesUTC(t *testing.T) {
	in := "2026-05-19T02:19:38.653148Z"
	got := NormalizeTimestampUTC(in)
	if got != in {
		t.Fatalf("got %q want %q", got, in)
	}
}

func TestUTCTimestampNowParseable(t *testing.T) {
	got := UTCTimestampNow()
	if _, err := time.Parse(time.RFC3339Nano, got); err != nil {
		t.Fatalf("parse: %v", err)
	}
}
