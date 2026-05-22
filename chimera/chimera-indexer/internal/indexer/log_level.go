package indexer

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/lynn/porcelain/chimera/internal/logfmt"
)

// ParseLogLevel maps indexer YAML/CLI log_level to slog.Level (default INFO).
func ParseLogLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// StderrLogger builds the indexer process stderr logger used by chimera-indexer.
func StderrLogger(logJSON bool, level slog.Level) *slog.Logger {
	return logfmt.NewLogger(os.Stderr, logJSON, level)
}

// JobSkipLogMode selects how per-file skip lines are emitted (see docs indexer.md).
type JobSkipLogMode uint8

const (
	// JobSkipLogInfo emits indexer.job.skipped (INFO) and indexer.job.upload (INFO).
	JobSkipLogInfo JobSkipLogMode = iota
	// JobSkipLogDebug emits indexer.skip.* at DEBUG only (no INFO upload line).
	JobSkipLogDebug
	// JobSkipLogOff suppresses per-file skip and pre-upload verbose lines.
	JobSkipLogOff
)

// ParseJobSkipLog parses job_skip_log YAML (info, debug, off).
func ParseJobSkipLog(s string) (JobSkipLogMode, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "info":
		return JobSkipLogInfo, nil
	case "debug":
		return JobSkipLogDebug, nil
	case "off", "none":
		return JobSkipLogOff, nil
	default:
		return JobSkipLogInfo, fmt.Errorf("job_skip_log: want info, debug, or off, got %q", s)
	}
}
