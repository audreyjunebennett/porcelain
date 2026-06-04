package corpusstale

import (
	"strings"
	"sync"
)

// Entry is one source whose on-disk hash differs from the indexed digest.
type Entry struct {
	Source        string `json:"source"`
	IndexedSHA256 string `json:"indexed_sha256"`
	LiveSHA256    string `json:"live_sha256"`
}

type scopeKey struct {
	tenant  string
	project string
	flavor  string
}

// Store holds indexer-reported stale sources per ingest scope (in-memory).
type Store struct {
	mu    sync.RWMutex
	byKey map[scopeKey][]Entry
}

// NewStore constructs an empty store.
func NewStore() *Store {
	return &Store{byKey: map[scopeKey][]Entry{}}
}

// ReplaceScope replaces stale entries for tenant/project/flavor.
func (s *Store) ReplaceScope(tenantID, projectID, flavorID string, entries []Entry) {
	if s == nil {
		return
	}
	key := scopeKey{
		tenant:  strings.TrimSpace(tenantID),
		project: strings.TrimSpace(projectID),
		flavor:  strings.TrimSpace(flavorID),
	}
	out := make([]Entry, 0, len(entries))
	for _, e := range entries {
		src := strings.TrimSpace(e.Source)
		if src == "" {
			continue
		}
		out = append(out, Entry{
			Source:        src,
			IndexedSHA256: strings.TrimSpace(e.IndexedSHA256),
			LiveSHA256:    strings.TrimSpace(e.LiveSHA256),
		})
	}
	s.mu.Lock()
	s.byKey[key] = out
	s.mu.Unlock()
}

// ListScope returns stale entries for a scope.
func (s *Store) ListScope(tenantID, projectID, flavorID string) []Entry {
	if s == nil {
		return nil
	}
	key := scopeKey{
		tenant:  strings.TrimSpace(tenantID),
		project: strings.TrimSpace(projectID),
		flavor:  strings.TrimSpace(flavorID),
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	src := s.byKey[key]
	if len(src) == 0 {
		return nil
	}
	out := make([]Entry, len(src))
	copy(out, src)
	return out
}

// IsStale reports whether source is stale in scope (indexed hash mismatch).
func (s *Store) IsStale(tenantID, projectID, flavorID, source, indexedSHA string) bool {
	indexedSHA = strings.TrimSpace(indexedSHA)
	if indexedSHA == "" {
		return false
	}
	for _, e := range s.ListScope(tenantID, projectID, flavorID) {
		if e.Source != strings.TrimSpace(source) {
			continue
		}
		if strings.TrimSpace(e.IndexedSHA256) == indexedSHA {
			return true
		}
	}
	return false
}
