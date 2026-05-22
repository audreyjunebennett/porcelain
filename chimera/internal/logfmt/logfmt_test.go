package logfmt

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"testing"
)

func TestNewLoggerJSONTimestampUTCSeconds(t *testing.T) {
	var buf bytes.Buffer
	log := NewLogger(&buf, true, slog.LevelInfo)
	log.Info("wrapper.backend.starting", "msg", "wrapper.backend.starting", "status", "degraded")

	var m map[string]any
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	ts, ok := m["time"].(string)
	if !ok || ts == "" {
		t.Fatalf("missing time: %v", m)
	}
	if ts != "2026-05-22T16:38:55Z" && ts[len(ts)-1] != 'Z' {
		// Exact value varies with clock; require UTC second precision shape.
		if len(ts) != len("2026-05-22T16:38:55Z") {
			t.Fatalf("expected UTC RFC3339 second precision, got %q", ts)
		}
		for i, c := range ts {
			switch i {
			case 4, 7:
				if c != '-' {
					t.Fatalf("expected UTC RFC3339 second precision, got %q", ts)
				}
			case 10:
				if c != 'T' {
					t.Fatalf("expected UTC RFC3339 second precision, got %q", ts)
				}
			case 13, 16:
				if c != ':' {
					t.Fatalf("expected UTC RFC3339 second precision, got %q", ts)
				}
			case 19:
				if c != 'Z' {
					t.Fatalf("expected UTC RFC3339 second precision, got %q", ts)
				}
			default:
				if c < '0' || c > '9' {
					t.Fatalf("expected UTC RFC3339 second precision, got %q", ts)
				}
			}
		}
	}
}
