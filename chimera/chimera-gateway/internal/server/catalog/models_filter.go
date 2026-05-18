package server

import (
	"github.com/lynn/porcelain/chimera/internal/config"
	"github.com/lynn/porcelain/chimera/internal/providerfreetier"
)

func filterOpenAIModelDataByFreeTier(data []any, res *config.Resolved) []any {
	if res == nil || !res.ShouldApplyFreeTierCatalogFilter() {
		return data
	}
	return filterModelDataBySpec(data, res.ProviderFreeTierSpec)
}

func filterModelDataBySpec(data []any, spec *providerfreetier.Spec) []any {
	if spec == nil {
		return data
	}
	var out []any
	for _, raw := range data {
		m, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		id, _ := m["id"].(string)
		if spec.Match(id) {
			out = append(out, raw)
		}
	}
	return out
}
