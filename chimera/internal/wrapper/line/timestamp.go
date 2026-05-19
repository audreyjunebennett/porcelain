package line

import (
	"strings"
	"time"
)

// UTCTimestampNow returns the current time as UTC RFC3339Nano (operator log convention).
func UTCTimestampNow() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

// NormalizeTimestampUTC returns a UTC RFC3339Nano timestamp. Empty input yields now (UTC).
// Unparseable values are returned trimmed unchanged.
func NormalizeTimestampUTC(ts string) string {
	ts = strings.TrimSpace(ts)
	if ts == "" {
		return UTCTimestampNow()
	}
	if t, err := time.Parse(time.RFC3339Nano, ts); err == nil {
		return t.UTC().Format(time.RFC3339Nano)
	}
	if t, err := time.Parse(time.RFC3339, ts); err == nil {
		return t.UTC().Format(time.RFC3339Nano)
	}
	return ts
}
