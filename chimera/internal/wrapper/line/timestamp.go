package line

import (
	"strings"
	"time"
)

// UTCTimestampNow returns the current time as UTC RFC3339 (operator log convention).
func UTCTimestampNow() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// NormalizeTimestampUTC returns a UTC RFC3339 timestamp truncated to whole seconds.
// Empty input yields now (UTC). Unparseable values are returned trimmed unchanged.
func NormalizeTimestampUTC(ts string) string {
	ts = strings.TrimSpace(ts)
	if ts == "" {
		return UTCTimestampNow()
	}
	if t, err := time.Parse(time.RFC3339Nano, ts); err == nil {
		return t.UTC().Truncate(time.Second).Format(time.RFC3339)
	}
	if t, err := time.Parse(time.RFC3339, ts); err == nil {
		return t.UTC().Truncate(time.Second).Format(time.RFC3339)
	}
	return ts
}
