package runtime

import (
	"context"
	"log/slog"
	"strings"
	"sync"

	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/catalog"
	"github.com/lynn/porcelain/chimera/internal/config"
)

var fallbackAvailabilityAuditorOnce sync.Once

// EnsureFallbackAvailabilityCatalogAuditor registers a catalog auditor that warns when
// configured fallback chains reference operator-marked unavailable models for a tenant.
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
	if log == nil || rt == nil || res == nil {
		return
	}
	reg := rt.ProviderModels()
	if reg == nil {
		return
	}
	tenantIDs := reg.TenantIDs()
	if len(tenantIDs) == 0 {
		return
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
		return
	}

	seen := make(map[string]struct{})
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
				key := tenantID + "\x00" + cref.source + "\x00" + mid
				if _, ok := seen[key]; ok {
					continue
				}
				seen[key] = struct{}{}
				log.Warn("fallback chain references operator-unavailable model",
					"msg", "gateway.catalog.fallback_unavailable_model",
					"tenant_id", tenantID,
					"source", cref.source,
					"model_id", mid,
				)
			}
		}
	}
}
