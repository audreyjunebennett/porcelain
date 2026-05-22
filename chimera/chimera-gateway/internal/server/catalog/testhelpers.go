package catalog

import "time"

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
		set[p] = struct{}{}
	}
	return &CatalogSnapshot{
		FetchedAt:   at,
		OK:          true,
		Providers:   append([]string(nil), providers...),
		providerSet: set,
		modelSet:    map[string]struct{}{},
	}
}

// ResetAuditorsForTest clears all registered catalog auditors (test isolation).
func ResetAuditorsForTest() {
	catalogAuditorsMu.Lock()
	catalogAuditors = nil
	catalogAuditorsMu.Unlock()
}
