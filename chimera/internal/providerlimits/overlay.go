package providerlimits

import "strings"

// ContextCatalog supplies live context_length defaults keyed by upstream model id.
// Implemented by the gateway catalog snapshot; nil means YAML-only resolution.
type ContextCatalog interface {
	ContextLength(modelID string) (int64, bool)
}

// OverlayCatalogContext fills ContextWindow from the catalog when YAML left it unset.
// YAML values always win when ContextWindow is already set on eff.
func OverlayCatalogContext(eff Effective, cat ContextCatalog) Effective {
	if cat == nil || eff.ContextWindow != nil {
		return eff
	}
	modelID := strings.TrimSpace(eff.ModelID)
	if modelID == "" {
		return eff
	}
	if n, ok := cat.ContextLength(modelID); ok && n > 0 {
		v := n
		eff.ContextWindow = &v
	}
	return eff
}

// ResolveWithCatalog resolves upstreamID and overlays catalog context_length when YAML omits context_window.
func (c *Config) ResolveWithCatalog(upstreamID string, cat ContextCatalog) Effective {
	if c == nil {
		return OverlayCatalogContext(Effective{ModelID: upstreamID}, cat)
	}
	return OverlayCatalogContext(c.Resolve(upstreamID), cat)
}
