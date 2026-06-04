package indexer

import (
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
)

// StaleSource reports on-disk content that no longer matches the last indexed digest.
type StaleSource struct {
	Source        string
	IndexedSHA256 string
	LiveSHA256    string
	ProjectID     string
	FlavorID      string
}

// CollectStaleSources compares watched files to sync-state server digests.
func CollectStaleSources(cfg Resolved, gw *IndexerConfig, st *SyncState, roots []Root) []StaleSource {
	if st == nil || len(roots) == 0 {
		return nil
	}
	rootByID := make(map[string]Root, len(roots))
	for _, r := range roots {
		rootByID[r.ID] = r
	}
	var out []StaleSource
	err := st.ForEachEntry(func(row SyncListEntry) error {
		root, ok := rootByID[row.RootID]
		if !ok {
			return nil
		}
		abs := strings.TrimSpace(root.AbsPath)
		rel := strings.TrimSpace(row.RelPath)
		if abs == "" || rel == "" {
			return nil
		}
		path := filepath.Join(abs, filepath.FromSlash(rel))
		_, liveHash, err := ReadNormalizeFile(path)
		if err != nil {
			return nil
		}
		indexed := strings.TrimSpace(row.Entry.ServerSHA)
		if indexed == "" || liveHash == indexed {
			return nil
		}
		proj, flav, _ := effectiveIngestTriple(cfg, root, gw)
		out = append(out, StaleSource{
			Source:        rel,
			IndexedSHA256: indexed,
			LiveSHA256:    liveHash,
			ProjectID:     proj,
			FlavorID:      flav,
		})
		return nil
	})
	if err != nil {
		return nil
	}
	return out
}

// CoherenceReporter rate-limits indexer.coherence.stale DEBUG logs per source.
type CoherenceReporter struct {
	mu   sync.Mutex
	seen map[string]bool
	log  *slog.Logger
}

// NewCoherenceReporter constructs a reporter.
func NewCoherenceReporter(log *slog.Logger) *CoherenceReporter {
	return &CoherenceReporter{seen: map[string]bool{}, log: log}
}

// LogStale emits at most one DEBUG line per source per process for stale drift.
func (r *CoherenceReporter) LogStale(src StaleSource) {
	if r == nil || r.log == nil {
		return
	}
	key := src.ProjectID + "\x00" + src.FlavorID + "\x00" + src.Source
	r.mu.Lock()
	if r.seen[key] {
		r.mu.Unlock()
		return
	}
	r.seen[key] = true
	r.mu.Unlock()
	r.log.Debug("indexed file changed on disk since last ingest",
		"msg", "indexer.coherence.stale",
		"source", src.Source,
		"ingest_project", src.ProjectID,
		"flavor_id", src.FlavorID,
		"indexed_sha256", src.IndexedSHA256,
		"live_sha256", src.LiveSHA256,
	)
}

// Reset clears rate-limit keys (e.g. after full re-index).
func (r *CoherenceReporter) Reset() {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.seen = map[string]bool{}
	r.mu.Unlock()
}
