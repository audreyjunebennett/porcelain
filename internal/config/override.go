package config

import "strings"

// CloneResolved returns a deep-enough copy for safe mutation (fallback chain slice).
func CloneResolved(r *Resolved) *Resolved {
	if r == nil {
		return nil
	}
	n := *r
	if r.FallbackChain != nil {
		n.FallbackChain = append([]string(nil), r.FallbackChain...)
	}
	if r.RouterModels != nil {
		n.RouterModels = append([]string(nil), r.RouterModels...)
	}
	n.ToolRouterEnabled = r.ToolRouterEnabled
	n.ToolRouterConfidenceThreshold = r.ToolRouterConfidenceThreshold
	n.FilterFreeTierModels = r.FilterFreeTierModels
	n.ProviderFreeTierPath = r.ProviderFreeTierPath
	n.ProviderFreeTierSpec = r.ProviderFreeTierSpec
	n.WitnessSampleMaxChars = r.WitnessSampleMaxChars
	n.WitnessSampleForceAtDebug = r.WitnessSampleForceAtDebug
	return &n
}

// PatchResolvedUpstream sets upstream base and default {base}/health (supervised local BiFrost).
func PatchResolvedUpstream(r *Resolved, baseURL string) {
	if r == nil {
		return
	}
	base := strings.TrimSuffix(strings.TrimSpace(baseURL), "/")
	if base == "" {
		return
	}
	r.UpstreamBaseURL = base
	r.HealthUpstreamURL = base + "/health"
}
