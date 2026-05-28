package virtualmodel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/operatorstore"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/routing"
)

var (
	ErrNotFound   = errors.New("virtual model not found")
	ErrDisabled   = errors.New("virtual model disabled")
	ErrForbidden  = errors.New("virtual model not visible to principal")
	ErrNoFallback = errors.New("virtual model has empty fallback chain")
)

// Resolved holds runtime routing state for one virtual model.
type Resolved struct {
	ID                   int64
	ModelID              string
	Description          string
	FallbackChain        []string
	RoutingPolicyYAML    []byte
	RoutingPolicyEnabled bool
	ToolRouterEnabled    bool
	RouterModels         []string
	ToolRouterConfidence float64
	Visibility           string
	CreatedByPrincipalID string
	policy               *routing.InMemoryPolicy
}

// Policy returns the compiled routing policy for this virtual model.
func (r *Resolved) Policy() *routing.InMemoryPolicy {
	if r == nil {
		return nil
	}
	return r.policy
}

// Registry caches enabled virtual models from operator SQLite.
type Registry struct {
	mu          sync.RWMutex
	revision    atomic.Int64
	bootstrapID string
	byModelID   map[string]*Resolved
}

// NewRegistry constructs an empty registry.
func NewRegistry() *Registry {
	return &Registry{byModelID: make(map[string]*Resolved)}
}

// Revision returns the current cache generation (bumped on reload).
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

// BootstrapModelID returns the first-imported model id (lowest row id), if any.
func (reg *Registry) BootstrapModelID() string {
	if reg == nil {
		return ""
	}
	reg.mu.RLock()
	defer reg.mu.RUnlock()
	return reg.bootstrapID
}

// AllEnabled returns a snapshot slice of enabled virtual models in the registry.
func (reg *Registry) AllEnabled() []*Resolved {
	if reg == nil {
		return nil
	}
	reg.mu.RLock()
	defer reg.mu.RUnlock()
	out := make([]*Resolved, 0, len(reg.byModelID))
	for _, vm := range reg.byModelID {
		out = append(out, vm)
	}
	return out
}

// Reload replaces the cache from operator store.
func (reg *Registry) Reload(ctx context.Context, store *operatorstore.Store) error {
	if reg == nil {
		return nil
	}
	if store == nil {
		reg.mu.Lock()
		reg.byModelID = make(map[string]*Resolved)
		reg.bootstrapID = ""
		reg.mu.Unlock()
		reg.BumpRevision()
		return nil
	}
	vms, err := store.ListEnabledVirtualModels(ctx)
	if err != nil {
		return err
	}
	next := make(map[string]*Resolved, len(vms))
	var bootstrap string
	for _, vm := range vms {
		resolved, err := compileVirtualModel(vm)
		if err != nil {
			return fmt.Errorf("compile virtual model %q: %w", vm.ModelID, err)
		}
		next[vm.ModelID] = resolved
		if bootstrap == "" {
			bootstrap = vm.ModelID
		}
	}
	reg.mu.Lock()
	reg.byModelID = next
	reg.bootstrapID = bootstrap
	reg.mu.Unlock()
	reg.BumpRevision()
	return nil
}

func compileVirtualModel(vm operatorstore.VirtualModel) (*Resolved, error) {
	if len(vm.FallbackChain) == 0 {
		return nil, ErrNoFallback
	}
	polYAML := []byte(vm.RoutingPolicyYAML)
	polEnabled := vm.RoutingPolicyEnabled
	if polEnabled && len(polYAML) > 0 {
		if err := routing.ValidatePolicyYAML(polYAML); err != nil {
			polEnabled = false
			polYAML = nil
		}
	}
	if !polEnabled {
		polYAML = nil
	}
	pol, err := routing.NewInMemoryPolicy(polYAML)
	if err != nil {
		return nil, err
	}
	th := vm.ToolRouterConfidence
	if th <= 0 {
		th = 0.5
	}
	return &Resolved{
		ID:                   vm.ID,
		ModelID:              vm.ModelID,
		Description:          vm.Description,
		FallbackChain:        append([]string(nil), vm.FallbackChain...),
		RoutingPolicyYAML:    polYAML,
		RoutingPolicyEnabled: polEnabled,
		ToolRouterEnabled:    vm.ToolRouterEnabled,
		RouterModels:         append([]string(nil), vm.RouterModels...),
		ToolRouterConfidence: th,
		Visibility:           vm.Visibility,
		CreatedByPrincipalID: vm.CreatedByPrincipalID,
		policy:               pol,
	}, nil
}

// Resolve returns runtime routing for modelID when enabled and visible.
func (reg *Registry) Resolve(modelID, principalID string) (*Resolved, error) {
	if reg == nil {
		return nil, ErrNotFound
	}
	modelID = strings.TrimSpace(modelID)
	reg.mu.RLock()
	vm, ok := reg.byModelID[modelID]
	reg.mu.RUnlock()
	if !ok {
		return nil, ErrNotFound
	}
	if vm.Visibility == operatorstore.VisibilityPrivate {
		if principalID == "" || (vm.CreatedByPrincipalID != "" && vm.CreatedByPrincipalID != principalID) {
			return nil, ErrForbidden
		}
	}
	return vm, nil
}

// ResolveAny loads by model id including disabled models (for error messages).
func (reg *Registry) ResolveAny(modelID string) (*Resolved, bool) {
	if reg == nil {
		return nil, false
	}
	reg.mu.RLock()
	vm, ok := reg.byModelID[modelID]
	reg.mu.RUnlock()
	return vm, ok
}

// ListCatalog returns enabled public models plus private models owned by principalID.
func (reg *Registry) ListCatalog(principalID string) []*Resolved {
	if reg == nil {
		return nil
	}
	reg.mu.RLock()
	defer reg.mu.RUnlock()
	out := make([]*Resolved, 0, len(reg.byModelID))
	for _, vm := range reg.byModelID {
		if vm.Visibility == operatorstore.VisibilityPrivate {
			if principalID == "" || (vm.CreatedByPrincipalID != "" && vm.CreatedByPrincipalID != principalID) {
				continue
			}
		}
		out = append(out, vm)
	}
	return out
}

// PickInitialModel applies per-VM routing policy to select the first upstream model.
func PickInitialModel(vm *Resolved, body map[string]json.RawMessage, log *slog.Logger) (model string, via routing.Via) {
	return PickInitialModelWithAvailability(vm, body, log, nil)
}

// PickInitialModelWithAvailability skips upstream ids the checker reports as unavailable.
func PickInitialModelWithAvailability(vm *Resolved, body map[string]json.RawMessage, log *slog.Logger, available func(string) bool) (model string, via routing.Via) {
	if vm == nil {
		return "", routing.ViaChainOnly
	}
	if vm.RoutingPolicyEnabled && vm.policy != nil {
		return vm.policy.PickInitialModelWithAvailability(body, vm.FallbackChain, vm.ModelID, available, log)
	}
	return routingFirstAvailable(vm.FallbackChain, available), routing.ViaChainOnly
}

func routingFirstAvailable(chain []string, available func(string) bool) string {
	for _, id := range chain {
		if id == "" {
			continue
		}
		if available == nil || available(id) {
			return id
		}
	}
	return ""
}
