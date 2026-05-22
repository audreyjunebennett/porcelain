// Package logfmt provides shared structured logging helpers for Chimera binaries.
package logfmt

import (
	"io"
	"log/slog"
	"os"
	"strings"
	"time"
)

const EnvLogJSON = "CHIMERA_LOG_JSON"

const EnvSupervised = "CHIMERA_SUPERVISED"

// JSONEnabled reports whether CHIMERA_LOG_JSON requests JSON slog output.
func JSONEnabled() bool {
	return parseBool(os.Getenv(EnvLogJSON))
}

// SupervisedMode reports whether CHIMERA_SUPERVISED marks a child of chimera-supervisor.
func SupervisedMode() bool {
	return parseBool(os.Getenv(EnvSupervised))
}

func parseBool(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// NewHandler returns a text or JSON slog handler.
func NewHandler(w io.Writer, json bool, level slog.Level) slog.Handler {
	hopts := &slog.HandlerOptions{
		Level:       level,
		ReplaceAttr: replaceLogTimeAttr,
	}
	if json {
		return slog.NewJSONHandler(w, hopts)
	}
	return slog.NewTextHandler(w, hopts)
}

func replaceLogTimeAttr(groups []string, a slog.Attr) slog.Attr {
	if len(groups) == 0 && a.Key == slog.TimeKey {
		if t, ok := a.Value.Any().(time.Time); ok {
			return slog.String(slog.TimeKey, t.UTC().Truncate(time.Second).Format(time.RFC3339))
		}
	}
	return a
}

// NewLogger builds a slog.Logger with the requested format.
func NewLogger(w io.Writer, json bool, level slog.Level) *slog.Logger {
	return slog.New(NewHandler(w, json, level))
}
