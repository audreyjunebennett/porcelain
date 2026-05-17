package indexer

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const syncStateVersion = 1

// SyncEntry records the last successful ingest for a root-relative job key.
type SyncEntry struct {
	ClientSHA string `json:"client_sha256"`
	ServerSHA string `json:"server_sha256"`
}

// SyncState persists v0.4 bookkeeping (client vs server content digests) for
// skip-if-unchanged and future reconciliation.
type SyncState struct {
	mu   sync.Mutex
	path string
	data syncStateFile
}

type syncStateFile struct {
	Version int                  `json:"version"`
	Entries map[string]SyncEntry `json:"entries"`
}

// OpenSyncState loads or creates an empty sync state file at path.
func OpenSyncState(path string) (*SyncState, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, nil
	}
	path = filepath.Clean(path)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	s := &SyncState{path: path, data: syncStateFile{Version: syncStateVersion, Entries: map[string]SyncEntry{}}}
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return s, nil
		}
		return nil, err
	}
	if len(b) == 0 {
		return s, nil
	}
	var raw syncStateFile
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	if raw.Entries == nil {
		raw.Entries = map[string]SyncEntry{}
	}
	raw.Version = syncStateVersion
	s.data = raw
	return s, nil
}

// Get returns the last recorded entry for key, if any.
func (s *SyncState) Get(key string) (SyncEntry, bool) {
	if s == nil {
		return SyncEntry{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.data.Entries[key]
	return e, ok
}

// Put updates an entry and flushes to disk.
func (s *SyncState) Put(key string, ent SyncEntry) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data.Entries == nil {
		s.data.Entries = map[string]SyncEntry{}
	}
	s.data.Entries[key] = ent
	return s.saveLocked()
}

func (s *SyncState) saveLocked() error {
	b, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
