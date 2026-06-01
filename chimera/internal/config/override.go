package config

import "strings"

// CloneResolved returns a deep-enough copy for safe mutation.
func CloneResolved(r *Resolved) *Resolved {
	if r == nil {
		return nil
	}
	n := *r
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
