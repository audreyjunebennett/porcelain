package indexer

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

const syncStateSQLiteFile = "sync-state.sqlite"

// SyncEntry records the last successful ingest for a root-relative job key.
type SyncEntry struct {
	ClientSHA   string `json:"client_sha256"`
	ServerSHA   string `json:"server_sha256"`
	ChunkCount  int    `json:"chunk_count,omitempty"`
	ChunkSchema int    `json:"chunk_schema,omitempty"`
}

// SyncState persists per-file ingest checkpoints in indexer-local SQLite.
type SyncState struct {
	mu         sync.Mutex
	db         *sql.DB
	sqlitePath string
}

// ResolveSyncStateSQLitePath maps a configured sync_state_path to the SQLite file.
// .json paths use sync-state.sqlite in the same directory; explicit .sqlite paths are used as-is.
func ResolveSyncStateSQLitePath(configured string) string {
	configured = strings.TrimSpace(configured)
	if configured == "" {
		return ""
	}
	configured = filepath.Clean(configured)
	ext := strings.ToLower(filepath.Ext(configured))
	switch ext {
	case ".sqlite":
		return configured
	case ".json":
		return filepath.Join(filepath.Dir(configured), syncStateSQLiteFile)
	default:
		return filepath.Join(filepath.Dir(configured), syncStateSQLiteFile)
	}
}

func syncStateDSN(absPath string) string {
	p := filepath.ToSlash(absPath)
	return "file:" + p + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"
}

const syncStateDDL = `
CREATE TABLE IF NOT EXISTS sync_entries (
	job_key TEXT PRIMARY KEY,
	root_id TEXT NOT NULL,
	rel_path TEXT NOT NULL,
	client_sha256 TEXT NOT NULL,
	server_sha256 TEXT NOT NULL,
	chunk_count INTEGER NOT NULL DEFAULT 0,
	chunk_schema INTEGER NOT NULL DEFAULT 0,
	updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_sync_entries_root ON sync_entries(root_id);
`

// OpenSyncState opens or creates the SQLite sync-state store for configuredPath.
// When configuredPath ends in .json and a legacy JSON file exists with a new empty
// SQLite store, entries are imported once and the JSON file is renamed to .bak.
func OpenSyncState(configuredPath string) (*SyncState, error) {
	configuredPath = strings.TrimSpace(configuredPath)
	if configuredPath == "" {
		return nil, nil
	}
	sqlitePath := ResolveSyncStateSQLitePath(configuredPath)
	if err := os.MkdirAll(filepath.Dir(sqlitePath), 0o755); err != nil {
		return nil, err
	}
	abs, err := filepath.Abs(sqlitePath)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", syncStateDSN(abs))
	if err != nil {
		return nil, fmt.Errorf("sync state sqlite open: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetConnMaxLifetime(0)
	if _, err := db.Exec(syncStateDDL); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("sync state schema: %w", err)
	}
	s := &SyncState{db: db, sqlitePath: abs}
	return s, nil
}

// StorePath returns the absolute SQLite path (empty when disabled).
func (s *SyncState) StorePath() string {
	if s == nil {
		return ""
	}
	return s.sqlitePath
}

// Close releases the database handle.
func (s *SyncState) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	err := s.db.Close()
	s.db = nil
	return err
}

func splitJobKey(key string) (rootID, relPath string) {
	if i := strings.Index(key, scopeKeySep); i >= 0 {
		return key[:i], key[i+len(scopeKeySep):]
	}
	return key, ""
}

// Get returns the last recorded entry for key, if any.
func (s *SyncState) Get(key string) (SyncEntry, bool) {
	if s == nil {
		return SyncEntry{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var ent SyncEntry
	var chunkCount, chunkSchema int
	err := s.db.QueryRow(`
SELECT client_sha256, server_sha256, chunk_count, chunk_schema
FROM sync_entries WHERE job_key = ?`, key).Scan(
		&ent.ClientSHA, &ent.ServerSHA, &chunkCount, &chunkSchema)
	if errors.Is(err, sql.ErrNoRows) {
		return SyncEntry{}, false
	}
	if err != nil {
		return SyncEntry{}, false
	}
	ent.ChunkCount = chunkCount
	ent.ChunkSchema = chunkSchema
	return ent, true
}

// Put updates an entry (incremental upsert).
func (s *SyncState) Put(key string, ent SyncEntry) error {
	if s == nil {
		return nil
	}
	rootID, relPath := splitJobKey(key)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`
INSERT INTO sync_entries (
	job_key, root_id, rel_path, client_sha256, server_sha256, chunk_count, chunk_schema, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(job_key) DO UPDATE SET
	client_sha256 = excluded.client_sha256,
	server_sha256 = excluded.server_sha256,
	chunk_count = excluded.chunk_count,
	chunk_schema = excluded.chunk_schema,
	updated_at = excluded.updated_at`,
		key, rootID, relPath, ent.ClientSHA, ent.ServerSHA, ent.ChunkCount, ent.ChunkSchema, now)
	return err
}

// DeleteByRoot removes all checkpoints for a watch root id.
func (s *SyncState) DeleteByRoot(rootID string) error {
	if s == nil {
		return nil
	}
	rootID = strings.TrimSpace(rootID)
	if rootID == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`DELETE FROM sync_entries WHERE root_id = ?`, rootID)
	return err
}

// SyncListEntry is one row from the sync-state table.
type SyncListEntry struct {
	JobKey  string
	RootID  string
	RelPath string
	Entry   SyncEntry
}

// ListEntries returns all sync checkpoints (for coherence scans).
func (s *SyncState) ListEntries() ([]SyncListEntry, error) {
	var out []SyncListEntry
	err := s.ForEachEntry(func(e SyncListEntry) error {
		out = append(out, e)
		return nil
	})
	return out, err
}

// ForEachEntry invokes fn for each sync checkpoint without loading the full table into one slice first.
func (s *SyncState) ForEachEntry(fn func(SyncListEntry) error) error {
	if s == nil {
		return nil
	}
	if fn == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`
SELECT job_key, root_id, rel_path, client_sha256, server_sha256, chunk_count, chunk_schema
FROM sync_entries`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var e SyncListEntry
		var chunkCount, chunkSchema int
		if err := rows.Scan(&e.JobKey, &e.RootID, &e.RelPath,
			&e.Entry.ClientSHA, &e.Entry.ServerSHA, &chunkCount, &chunkSchema); err != nil {
			return err
		}
		e.Entry.ChunkCount = chunkCount
		e.Entry.ChunkSchema = chunkSchema
		if err := fn(e); err != nil {
			return err
		}
	}
	return rows.Err()
}

// DeleteAll removes every checkpoint (force re-index).
func (s *SyncState) DeleteAll() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`DELETE FROM sync_entries`)
	return err
}

