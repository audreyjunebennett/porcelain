package catalog

import (
	"sort"
	"strings"
	"time"
)

// NewTestFailedSnapshot builds a fresh failed catalog poll for unit tests.
func NewTestFailedSnapshot(at time.Time, fetchErr string) *CatalogSnapshot {
	return &CatalogSnapshot{
		FetchedAt: at,
		FetchErr:  fetchErr,
	}
}

// NewTestSnapshot builds a minimal OK catalog snapshot for unit tests.
func NewTestSnapshot(at time.Time, providers []string) *CatalogSnapshot {
	set := map[string]struct{}{}
	for _, p := range providers {
		set[strings.TrimSpace(p)] = struct{}{}
	}
	return &CatalogSnapshot{
		FetchedAt:   at,
		OK:          true,
		Providers:   append([]string(nil), providers...),
		providerSet: set,
		modelSet:    map[string]struct{}{},
	}
}

// NewTestSnapshotWithModels builds an OK catalog snapshot with explicit model ids.
func NewTestSnapshotWithModels(at time.Time, modelIDs []string) *CatalogSnapshot {
	provSet := map[string]struct{}{}
	modelSet := map[string]struct{}{}
	provs := make([]string, 0)
	for _, raw := range modelIDs {
		id := strings.TrimSpace(raw)
		if id == "" {
			continue
		}
		modelSet[id] = struct{}{}
		if slash := strings.Index(id, "/"); slash > 0 {
			prov := id[:slash]
			if _, ok := provSet[prov]; !ok {
				provSet[prov] = struct{}{}
				provs = append(provs, prov)
			}
		}
	}
	sort.Strings(provs)
	ids := make([]string, 0, len(modelSet))
	for id := range modelSet {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return &CatalogSnapshot{
		FetchedAt:   at,
		OK:          true,
		Providers:   provs,
		providerSet: provSet,
		ModelIDs:    ids,
		modelSet:    modelSet,
	}
}

// ResetAuditorsForTest clears all registered catalog auditors (test isolation).
func ResetAuditorsForTest() {
	catalogAuditorsMu.Lock()
	catalogAuditors = nil
	catalogAuditorsMu.Unlock()
}
