package catalog

import (
	"strings"

	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/providermodels"
)

// FilterOpenAIModelDataByAvailability keeps only upstream models marked available for the tenant snapshot.
// A nil snapshot or empty unavailable set returns data unchanged.
func FilterOpenAIModelDataByAvailability(data []any, snap *providermodels.TenantSnapshot) []any {
	if snap == nil || len(data) == 0 {
		return data
	}
	unavail := snap.UnavailableModelIDs()
	if len(unavail) == 0 {
		return data
	}
	blocked := make(map[string]struct{}, len(unavail))
	for _, id := range unavail {
		blocked[strings.TrimSpace(id)] = struct{}{}
	}
	var out []any
	for _, raw := range data {
		m, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		id, _ := m["id"].(string)
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, skip := blocked[id]; skip {
			continue
		}
		out = append(out, raw)
	}
	return out
}
