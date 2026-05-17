package bifrostadmin

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Chimera-managed API key names use this pattern so the UI can order keys and
// allocate the next suffix: chimera-<provider>-key-<n> (e.g. chimera-groq-key-3).
func chimeraKeyPrefix(provider string) string {
	p := strings.ToLower(strings.TrimSpace(provider))
	if p == "" {
		p = "unknown"
	}
	return "chimera-" + p + "-key-"
}

func parseChimeraKeyIndex(provider, name string) (index int, ok bool) {
	prefix := chimeraKeyPrefix(provider)
	name = strings.TrimSpace(name)
	if !strings.HasPrefix(name, prefix) {
		return 0, false
	}
	suffix := strings.TrimSpace(name[len(prefix):])
	v, err := strconv.Atoi(suffix)
	if err != nil {
		return 0, false
	}
	return v, true
}

func nextChimeraKeyName(provider string, keys []any) string {
	prefix := chimeraKeyPrefix(provider)
	maxN := 0
	for _, raw := range keys {
		km, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		n, _ := km["name"].(string)
		if idx, ok := parseChimeraKeyIndex(provider, n); ok && idx > maxN {
			maxN = idx
		}
	}
	return fmt.Sprintf("%s%d", prefix, maxN+1)
}

func newChimeraAPIKeyRow(name, plaintext string) map[string]any {
	return map[string]any{
		"name":    name,
		"value":   plaintext,
		"weight":  float64(1),
		"enabled": true,
	}
}

// equalizeKeyWeights sets weight to 1.0 on every key object in keys (BiFrost docs:
// equal weights for even split across keys).
func equalizeKeyWeights(keys []any) {
	for i, raw := range keys {
		km, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		km["weight"] = float64(1)
		keys[i] = km
	}
}

// AppendProviderAPIKey returns JSON suitable for PUT /api/providers/{provider}: copies the
// current document, appends a new key with a generated chimera-<provider>-key-<n> name,
// weight 1, no models field (BiFrost: omit models to allow all models for that key).
func AppendProviderAPIKey(provider string, existingJSON []byte, plaintextKey string) ([]byte, error) {
	var root map[string]any
	if err := json.Unmarshal(existingJSON, &root); err != nil {
		return nil, err
	}
	ensureConcurrency(root)
	keys, ok := root["keys"].([]any)
	if !ok {
		keys = []any{}
	}
	name := nextChimeraKeyName(provider, keys)
	keys = append(keys, newChimeraAPIKeyRow(name, plaintextKey))
	equalizeKeyWeights(keys)
	root["keys"] = keys
	return json.Marshal(root)
}

// RemoveProviderKeyByName returns JSON suitable for PUT /api/providers/{provider} with the
// key row whose name matches (exact, trimmed) removed. Other rows are preserved.
// Remaining keys get weight 1.0 each.
func RemoveProviderKeyByName(existingJSON []byte, keyName string) ([]byte, error) {
	want := strings.TrimSpace(keyName)
	if want == "" {
		return nil, fmt.Errorf("chimera-broker-admin: empty key name")
	}
	var root map[string]any
	if err := json.Unmarshal(existingJSON, &root); err != nil {
		return nil, err
	}
	ensureConcurrency(root)
	keys, ok := root["keys"].([]any)
	if !ok {
		keys = []any{}
	}
	out := make([]any, 0, len(keys))
	for _, raw := range keys {
		km, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		n, _ := km["name"].(string)
		if strings.TrimSpace(n) == want {
			continue
		}
		out = append(out, km)
	}
	equalizeKeyWeights(out)
	root["keys"] = out
	return json.Marshal(root)
}

// MergeOllamaBaseURL updates network_config.base_url while preserving other provider fields.
func MergeOllamaBaseURL(existingJSON []byte, baseURL string) ([]byte, error) {
	var root map[string]any
	if err := json.Unmarshal(existingJSON, &root); err != nil {
		return nil, err
	}
	ensureConcurrency(root)
	if _, ok := root["keys"]; !ok {
		root["keys"] = []any{}
	}
	nc, ok := root["network_config"].(map[string]any)
	if !ok {
		nc = map[string]any{}
	}
	nc["base_url"] = baseURL
	root["network_config"] = nc
	return json.Marshal(root)
}

func ensureConcurrency(root map[string]any) {
	if root == nil {
		return
	}
	if _, ok := root["concurrency_and_buffer_size"]; !ok {
		root["concurrency_and_buffer_size"] = map[string]any{
			"concurrency": float64(100),
			"buffer_size": float64(200),
		}
	}
}
