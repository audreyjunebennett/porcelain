package providermodels

import (
	"context"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/operatorstore"
)

// TenantSnapshot is a read-only view of explicit unavailable models for one tenant.
// Models without a stored unavailable row are available (opt-out default).
type TenantSnapshot struct {
	TenantID    string
	unavailable map[string]struct{}
}

// IsAvailable reports whether modelID is exposed for the tenant.
func (s *TenantSnapshot) IsAvailable(modelID string) bool {
	if s == nil {
		return true
	}
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return true
	}
	if len(s.unavailable) == 0 {
		return true
	}
	_, blocked := s.unavailable[modelID]
	return !blocked
}

// UnavailableModelIDs returns a sorted copy of explicitly unavailable model ids.
func (s *TenantSnapshot) UnavailableModelIDs() []string {
	if s == nil || len(s.unavailable) == 0 {
		return nil
	}
	out := make([]string, 0, len(s.unavailable))
	for id := range s.unavailable {
		out = append(out, id)
	}
	return out
}

// SetUnavailableForTest replaces the unavailable set (tests only).
func (s *TenantSnapshot) SetUnavailableForTest(blocked map[string]struct{}) {
	if s == nil {
		return
	}
	if len(blocked) == 0 {
		s.unavailable = nil
		return
	}
	cp := make(map[string]struct{}, len(blocked))
	for id := range blocked {
		cp[id] = struct{}{}
	}
	s.unavailable = cp
}

// AvailableModelIDs returns broker ids marked available for the tenant, preserving input order.
func AvailableModelIDs(snap *TenantSnapshot, brokerIDs []string) []string {
	if snap == nil {
		out := make([]string, 0, len(brokerIDs))
		for _, id := range brokerIDs {
			if strings.TrimSpace(id) != "" {
				out = append(out, id)
			}
		}
		return out
	}
	out := make([]string, 0, len(brokerIDs))
	for _, id := range brokerIDs {
		if snap.IsAvailable(id) {
			out = append(out, id)
		}
	}
	return out
}

// FirstAvailable returns the first id in candidates that is available, or "" when none.
func FirstAvailable(snap *TenantSnapshot, candidates []string) string {
	for _, id := range candidates {
		if snap == nil || snap.IsAvailable(id) {
			return strings.TrimSpace(id)
		}
	}
	return ""
}

// Registry caches per-tenant unavailable model sets from operator SQLite.
type Registry struct {
	mu        sync.RWMutex
	revision  atomic.Int64
	byTenant  map[string]*TenantSnapshot
	allTenant map[string]struct{}
}

// NewRegistry constructs an empty registry.
func NewRegistry() *Registry {
	return &Registry{
		byTenant:  make(map[string]*TenantSnapshot),
		allTenant: make(map[string]struct{}),
	}
}

// Revision returns the cache generation (bumped on reload).
func (reg *Registry) Revision() int64 {
	if reg == nil {
		return 0
	}
	return reg.revision.Load()
}

// BumpRevision invalidates in-process consumers watching revision.
func (reg *Registry) BumpRevision() {
	if reg == nil {
		return
	}
	reg.revision.Add(1)
}

// Reload replaces the cache from operator store.
func (reg *Registry) Reload(ctx context.Context, store *operatorstore.Store) error {
	if reg == nil {
		return nil
	}
	if store == nil {
		reg.mu.Lock()
		reg.byTenant = make(map[string]*TenantSnapshot)
		reg.allTenant = make(map[string]struct{})
		reg.mu.Unlock()
		reg.BumpRevision()
		return nil
	}
	index, err := store.LoadProviderModelAvailabilityIndex(ctx)
	if err != nil {
		return err
	}
	byTenant := make(map[string]*TenantSnapshot, len(index))
	allTenant := make(map[string]struct{}, len(index))
	for tenantID, blocked := range index {
		cp := make(map[string]struct{}, len(blocked))
		for id := range blocked {
			cp[id] = struct{}{}
		}
		byTenant[tenantID] = &TenantSnapshot{TenantID: tenantID, unavailable: cp}
		allTenant[tenantID] = struct{}{}
	}
	reg.mu.Lock()
	reg.byTenant = byTenant
	reg.allTenant = allTenant
	reg.mu.Unlock()
	reg.BumpRevision()
	return nil
}

// Snapshot returns the tenant view, or an empty snapshot when unknown (all models available).
func (reg *Registry) Snapshot(tenantID string) *TenantSnapshot {
	if reg == nil {
		return &TenantSnapshot{TenantID: tenantID}
	}
	reg.mu.RLock()
	defer reg.mu.RUnlock()
	if snap, ok := reg.byTenant[tenantID]; ok && snap != nil {
		return snap
	}
	return &TenantSnapshot{TenantID: tenantID}
}

// IsModelAvailable reports availability for tenant+model using the cached snapshot.
func (reg *Registry) IsModelAvailable(tenantID, modelID string) bool {
	return reg.Snapshot(tenantID).IsAvailable(modelID)
}

// TenantIDs returns tenant ids with at least one explicit unavailable model row, sorted.
func (reg *Registry) TenantIDs() []string {
	if reg == nil {
		return nil
	}
	reg.mu.RLock()
	defer reg.mu.RUnlock()
	out := make([]string, 0, len(reg.allTenant))
	for tid := range reg.allTenant {
		out = append(out, tid)
	}
	sort.Strings(out)
	return out
}

// FilterAvailable returns ids from modelIDs that are available for the tenant, preserving order.
func (reg *Registry) FilterAvailable(tenantID string, modelIDs []string) []string {
	snap := reg.Snapshot(tenantID)
	if snap == nil || len(snap.unavailable) == 0 {
		out := make([]string, 0, len(modelIDs))
		for _, id := range modelIDs {
			if strings.TrimSpace(id) != "" {
				out = append(out, id)
			}
		}
		return out
	}
	out := make([]string, 0, len(modelIDs))
	for _, id := range modelIDs {
		if snap.IsAvailable(id) {
			out = append(out, id)
		}
	}
	return out
}
