package gatewaymetrics

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	_ "modernc.org/sqlite"
)

// Store is a SQLite-backed metrics sink. It is safe for concurrent use from HTTP handlers.
// After a permanent write error, recording is disabled for the process lifetime (see plan §3.6.2).
type Store struct {
	db     *sql.DB
	log    *slog.Logger
	mu     sync.RWMutex
	broken atomic.Bool
}

// Open creates parent directories, opens SQLite, and applies pending migrations from migrationsDir.
func Open(sqlitePath, migrationsDir string, log *slog.Logger) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(sqlitePath), 0o755); err != nil {
		return nil, fmt.Errorf("gateway metrics mkdir: %w", err)
	}
	abs, err := filepath.Abs(sqlitePath)
	if err != nil {
		return nil, err
	}
	dsn := sqliteDSN(abs)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("gateway metrics open: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetConnMaxLifetime(0)
	if err := ApplyMigrations(db, migrationsDir, log); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Store{db: db, log: log}, nil
}

func sqliteDSN(absPath string) string {
	// modernc.org/sqlite: file URI with absolute path (Windows C:/... and Unix /...).
	p := filepath.ToSlash(absPath)
	return "file:" + p + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
}

// Close releases the database handle.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// DB returns the underlying connection pool for tests only.
func (s *Store) DB() *sql.DB {
	if s == nil {
		return nil
	}
	return s.db
}

// RecordUpstreamResponse records one upstream HTTP outcome (per plan §3.6.3–3.6.4): one row per
// completed upstream round-trip (including each virtual-model fallback attempt). modelID is the
// full upstream id (e.g. groq/llama-3.3-70b-versatile). estRequestTokens is the gateway estimate
// for the proxied JSON body (tiktoken cl100k_base in the chat path).
func (s *Store) RecordUpstreamResponse(at time.Time, modelID string, status int, estRequestTokens int) {
	if s == nil || s.db == nil {
		return
	}
	if s.broken.Load() {
		return
	}
	provider, _ := SplitProviderModel(modelID)
	minute := at.UTC().Format("2006-01-02T15:04")
	day := at.UTC().Format("2006-01-02")
	occurred := at.UTC().Format(time.RFC3339Nano)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		s.disable(err, "begin tx")
		return
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `INSERT INTO upstream_call_events (occurred_at, provider, model_id, status, est_tokens) VALUES (?,?,?,?,?)`,
		occurred, provider, modelID, status, estRequestTokens); err != nil {
		s.disable(err, "insert event")
		return
	}

	if _, err := tx.ExecContext(ctx, `
INSERT INTO upstream_rollup_minute (provider, model_id, minute_utc, status, calls, est_tokens)
VALUES (?,?,?,?,1,?)
ON CONFLICT(provider, model_id, minute_utc, status) DO UPDATE SET
  calls = calls + 1,
  est_tokens = est_tokens + excluded.est_tokens
`, provider, modelID, minute, status, estRequestTokens); err != nil {
		s.disable(err, "rollup minute")
		return
	}

	if _, err := tx.ExecContext(ctx, `
INSERT INTO upstream_rollup_day (provider, model_id, day_utc, status, calls, est_tokens)
VALUES (?,?,?,?,1,?)
ON CONFLICT(provider, model_id, day_utc, status) DO UPDATE SET
  calls = calls + 1,
  est_tokens = est_tokens + excluded.est_tokens
`, provider, modelID, day, status, estRequestTokens); err != nil {
		s.disable(err, "rollup day")
		return
	}

	if err := tx.Commit(); err != nil {
		s.disable(err, "commit")
		return
	}
}

func (s *Store) disable(err error, step string) {
	if s.broken.Swap(true) {
		return
	}
	if s.log != nil {
		s.log.Error("gateway metrics disabled after write error", "msg", "gateway.metrics.disabled_after_error", "step", step, "err", err)
	}
}

// SplitProviderModel returns provider key (segment before first '/') and the full model id.
// If there is no slash, provider is empty and modelID is returned as-is.
func SplitProviderModel(modelID string) (provider, fullModel string) {
	fullModel = modelID
	i := strings.Index(modelID, "/")
	if i <= 0 {
		return "", modelID
	}
	return modelID[:i], modelID
}
