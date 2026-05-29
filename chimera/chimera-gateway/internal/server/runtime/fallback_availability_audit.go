package runtime

import (
	"context"
	"log/slog"
	"strings"
	"sync"

	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/catalog"
	"github.com/lynn/porcelain/chimera/internal/config"
)

var (
	fallbackAvailabilityAuditorOnce sync.Once
	fallbackAuditMu                 sync.Mutex
	fallbackAuditPrev               map[string]fallbackViolation
)

type fallbackViolation struct {
	TenantID string
	Source   string
	ModelID  string
}

func (v fallbackViolation) key() string {
	return v.TenantID + "\x00" + v.Source + "\x00" + v.ModelID
}

// EnsureFallbackAvailabilityCatalogAuditor registers a catalog auditor that warns when
// configured fallback chains newly reference operator-marked unavailable models for a tenant.
func EnsureFallbackAvailabilityCatalogAuditor(rt *Runtime) {
	if rt == nil {
		return
	}
	fallbackAvailabilityAuditorOnce.Do(func() {
		catalog.RegisterCatalogAuditor(func(ctx context.Context, snap *catalog.CatalogSnapshot, res *config.Resolved, log *slog.Logger) {
			auditConfiguredFallbackAvailability(ctx, rt, res, log)
		})
	})
}

func auditConfiguredFallbackAvailability(_ context.Context, rt *Runtime, res *config.Resolved, log *slog.Logger) {
	current := collectFallbackViolations(rt, res)
	fallbackAuditMu.Lock()
	prev := fallbackAuditPrev
	for key, v := range current {
		if _, ok := prev[key]; ok {
			continue
		}
		if log != nil {
			log.Warn("fallback chain references operator-unavailable model",
				"msg", "gateway.catalog.fallback_unavailable_model",
				"tenant_id", v.TenantID,
				"source", v.Source,
				"model_id", v.ModelID,
			)
		}
	}
	next := make(map[string]fallbackViolation, len(current))
	for key, v := range current {
		next[key] = v
	}
	fallbackAuditPrev = next
	fallbackAuditMu.Unlock()
}

func collectFallbackViolations(rt *Runtime, res *config.Resolved) map[string]fallbackViolation {
	out := make(map[string]fallbackViolation)
	if rt == nil || res == nil {
		return out
	}
	reg := rt.ProviderModels()
	if reg == nil {
		return out
	}
	tenantIDs := reg.TenantIDs()
	if len(tenantIDs) == 0 {
		return out
	}

	type chainRef struct {
		source string
		chain  []string
	}
	var chains []chainRef
	if fc := res.FallbackChain; len(fc) > 0 {
		chains = append(chains, chainRef{source: "gateway.fallback_chain", chain: fc})
	}
	if vmReg := rt.VirtualModels(); vmReg != nil {
		for _, vm := range vmReg.AllEnabled() {
			if vm == nil || len(vm.FallbackChain) == 0 {
				continue
			}
			src := "virtual_model:" + vm.ModelID
			chains = append(chains, chainRef{source: src, chain: vm.FallbackChain})
		}
	}
	if len(chains) == 0 {
		return out
	}

	for _, tenantID := range tenantIDs {
		snapAvail := reg.Snapshot(tenantID)
		if len(snapAvail.UnavailableModelIDs()) == 0 {
			continue
		}
		for _, cref := range chains {
			for _, mid := range cref.chain {
				mid = strings.TrimSpace(mid)
				if mid == "" || snapAvail.IsAvailable(mid) {
					continue
				}
				v := fallbackViolation{TenantID: tenantID, Source: cref.source, ModelID: mid}
				out[v.key()] = v
			}
		}
	}
	return out
}

func resetFallbackAvailabilityAuditStateForTest() {
	fallbackAuditMu.Lock()
	fallbackAuditPrev = nil
	fallbackAuditMu.Unlock()
}
