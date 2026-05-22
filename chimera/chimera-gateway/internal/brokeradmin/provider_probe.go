package brokeradmin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	internalhttp "github.com/lynn/porcelain/chimera/internal/http"
)

// ProviderProbeDecision describes whether GET /api/providers/{name} should run.
type ProviderProbeDecision struct {
	HTTPProbe         bool
	MissingFromConfig bool
	EmptyKeysInConfig bool
}

const configuredProvidersCacheTTL = 60 * time.Second

var (
	probeCacheMu sync.Mutex
	probeAbsent  = map[string]bool{}
	probeNoKeys  = map[string]bool{}

	configuredMu      sync.Mutex
	configuredAt      time.Time
	configuredOK      bool
	configuredBaseURL string
	configuredNames   map[string]struct{}
)

// InvalidateProviderConfigIndex clears broker provider probe caches (after admin saves).
func InvalidateProviderConfigIndex() {
	InvalidateProviderProbeCacheFor("")
}

// InvalidateProviderProbeCacheFor clears skip state for one provider after admin saves.
// An empty provider clears the configured-provider list cache as well.
func InvalidateProviderProbeCacheFor(provider string) {
	name := strings.ToLower(strings.TrimSpace(provider))
	probeCacheMu.Lock()
	if name == "" {
		probeAbsent = map[string]bool{}
		probeNoKeys = map[string]bool{}
	} else {
		delete(probeAbsent, name)
		delete(probeNoKeys, name)
	}
	probeCacheMu.Unlock()

	configuredMu.Lock()
	configuredAt = time.Time{}
	configuredOK = false
	configuredNames = nil
	configuredBaseURL = ""
	configuredMu.Unlock()
}

// ListConfiguredProviders returns provider ids registered in the live chimera-broker config store.
// It uses GET /api/governance/providers (BiFrost has no GET /api/providers list endpoint).
// The bool is false when the list call failed; callers should fall back to per-provider GET probes.
func ListConfiguredProviders(ctx context.Context, client *Client) (map[string]struct{}, bool) {
	if client == nil || strings.TrimSpace(client.BaseURL) == "" {
		return nil, false
	}
	base := strings.TrimSuffix(strings.TrimSpace(client.BaseURL), "/")

	configuredMu.Lock()
	defer configuredMu.Unlock()

	if configuredOK && configuredBaseURL == base && time.Since(configuredAt) < configuredProvidersCacheTTL && configuredNames != nil {
		return copyNameSet(configuredNames), true
	}

	// Fetch while holding the lock so concurrent /api/ui/state and
	// /api/ui/chimera-broker/providers handlers share one governance list call.
	names, ok := fetchConfiguredProviders(ctx, client)
	if !ok || len(names) == 0 {
		// BiFrost may return HTTP 200 with providers:null while GET /api/providers/{name}
		// still works (file-backed config). Treat an empty list as unavailable so callers
		// fall back to per-provider GET probes instead of skipping every provider.
		return nil, false
	}

	configuredAt = time.Now()
	configuredOK = true
	configuredBaseURL = base
	configuredNames = names
	return copyNameSet(names), true
}

func fetchConfiguredProviders(ctx context.Context, client *Client) (map[string]struct{}, bool) {
	base := strings.TrimSuffix(strings.TrimSpace(client.BaseURL), "/")
	if base == "" {
		return nil, false
	}
	u := base + "/api/governance/providers"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, false
	}
	if h := client.authHeader(); h != "" {
		req.Header.Set("Authorization", h)
	}
	resp, err := client.httpClient().Do(req)
	if err != nil {
		return nil, false
	}
	body, err := internalhttp.ReadAndCloseLimited(resp.Body, 2<<20)
	if err != nil || resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, false
	}
	return parseConfiguredProviderNames(body), true
}

func parseConfiguredProviderNames(body []byte) map[string]struct{} {
	out := map[string]struct{}{}
	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil {
		return out
	}
	items, _ := doc["providers"].([]any)
	for _, raw := range items {
		m, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		name, _ := m["provider"].(string)
		name = strings.ToLower(strings.TrimSpace(name))
		if name != "" {
			out[name] = struct{}{}
		}
	}
	return out
}

func copyNameSet(in map[string]struct{}) map[string]struct{} {
	out := make(map[string]struct{}, len(in))
	for k := range in {
		out[k] = struct{}{}
	}
	return out
}

func probeDecision(name string, configured map[string]struct{}, listOK bool) ProviderProbeDecision {
	name = strings.ToLower(strings.TrimSpace(name))
	if listOK {
		if _, ok := configured[name]; !ok {
			return ProviderProbeDecision{MissingFromConfig: true}
		}
	} else {
		probeCacheMu.Lock()
		if probeAbsent[name] {
			probeCacheMu.Unlock()
			return ProviderProbeDecision{MissingFromConfig: true}
		}
		if probeNoKeys[name] {
			probeCacheMu.Unlock()
			return ProviderProbeDecision{EmptyKeysInConfig: true}
		}
		probeCacheMu.Unlock()
	}
	return ProviderProbeDecision{HTTPProbe: true}
}

func rememberProviderProbeResult(name string, st int, body []byte) {
	name = strings.ToLower(strings.TrimSpace(name))
	probeCacheMu.Lock()
	defer probeCacheMu.Unlock()
	if IsProviderMissingGET(st, body) {
		probeAbsent[name] = true
		delete(probeNoKeys, name)
		return
	}
	delete(probeAbsent, name)
	if st >= 200 && st < 300 && !strings.EqualFold(name, "ollama") {
		sum, err := SummarizeProvider(name, body)
		if err == nil && !sum.KeyConfigured {
			probeNoKeys[name] = true
			return
		}
	}
	delete(probeNoKeys, name)
}

// SyntheticProviderGETBody returns a JSON body/status suitable for ClassifyBrokerProviderResult
// or state assembly when HTTPProbe is false.
func SyntheticProviderGETBody(name string, dec ProviderProbeDecision) (body []byte, status int) {
	if dec.MissingFromConfig {
		return []byte(`{"is_chimera_broker_error":false,"status_code":404,"error":{"message":"Provider not found"}}`), 200
	}
	if dec.EmptyKeysInConfig {
		return []byte(`{"name":"` + strings.TrimSpace(name) + `","keys":[]}`), 200
	}
	return nil, 0
}

// GetProviderForProbe returns provider config from the broker HTTP API when needed, or a synthetic
// response when the provider is not registered or has no configured keys (from prior GET cache).
// The httpProbed return is true only when a live HTTP GET /api/providers/{name} was attempted.
func GetProviderForProbe(ctx context.Context, client *Client, name string) (body []byte, status int, err error, httpProbed bool) {
	configured, listOK := ListConfiguredProviders(ctx, client)
	return GetProviderForProbeWithList(ctx, client, name, configured, listOK)
}

// GetProviderForProbeWithList is like GetProviderForProbe but reuses a governance list already
// fetched by the caller so one UI poll issues a single GET /api/governance/providers.
func GetProviderForProbeWithList(ctx context.Context, client *Client, name string, configured map[string]struct{}, listOK bool) (body []byte, status int, err error, httpProbed bool) {
	dec := probeDecision(name, configured, listOK)
	if !dec.HTTPProbe {
		body, status = SyntheticProviderGETBody(name, dec)
		return body, status, nil, false
	}
	if client == nil {
		return nil, 0, fmt.Errorf("chimera-broker-admin: nil client"), true
	}
	body, status, err = client.GetProvider(ctx, name)
	if err == nil {
		rememberProviderProbeResult(name, status, body)
		if !listOK {
			dec = probeDecision(name, nil, false)
			if !dec.HTTPProbe {
				body, status = SyntheticProviderGETBody(name, dec)
				return body, status, nil, true
			}
		}
	}
	return body, status, err, true
}
