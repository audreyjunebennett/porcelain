package bifrostadmin

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// ProviderSummary is a small read model for the admin UI (no raw secrets).
type ProviderSummary struct {
	// KeyHint is a short summary for compact UI (first key in display order, or count).
	KeyHint string `json:"key_hint"`
	// KeyConfigured is true when any key appears configured (direct value or env-backed).
	KeyConfigured bool `json:"key_configured"`
	// OllamaBaseURL is set for the ollama provider from network_config.base_url.
	OllamaBaseURL string `json:"ollama_base_url,omitempty"`
}

// KeyEntrySummary is one API key row for the admin UI (no raw secrets).
type KeyEntrySummary struct {
	Name          string `json:"name"`
	KeyHint       string `json:"key_hint"`
	KeyConfigured bool   `json:"key_configured"`
}

type keySortRec struct {
	summary KeyEntrySummary
	// chimeraSeq is the numeric suffix for chimera-<provider>-key-<n>; 0 means not that pattern.
	chimeraSeq int
	isChimera  bool
}

// SummarizeProvider parses GET /api/providers/{p} JSON into a ProviderSummary.
// Key "value" may be a string (file / store inline or env.GROQ_API_KEY) or an object (e.g. {"value":"***"} from API).
func SummarizeProvider(providerName string, body []byte) (ProviderSummary, error) {
	var out ProviderSummary
	if len(body) == 0 {
		return out, nil
	}
	var root map[string]any
	if err := json.Unmarshal(body, &root); err != nil {
		return out, err
	}
	if strings.EqualFold(strings.TrimSpace(providerName), "ollama") {
		if nc, ok := root["network_config"].(map[string]any); ok {
			if u, _ := nc["base_url"].(string); strings.TrimSpace(u) != "" {
				out.OllamaBaseURL = strings.TrimSpace(u)
			}
		}
	}
	sorted, err := SummarizeProviderKeys(providerName, body)
	if err != nil {
		return out, err
	}
	if len(sorted) == 0 {
		out.KeyHint = "not set"
		return out, nil
	}
	anyCfg := false
	for _, e := range sorted {
		if e.KeyConfigured {
			anyCfg = true
		}
	}
	out.KeyConfigured = anyCfg
	if len(sorted) == 1 {
		out.KeyHint = sorted[0].KeyHint
		return out, nil
	}
	out.KeyHint = fmt.Sprintf("%d keys", len(sorted))
	return out, nil
}

// SummarizeProviderKeys returns key rows sorted for display: chimera-<provider>-key-<n> by n ascending,
// then any other keys by name ascending.
func SummarizeProviderKeys(providerName string, body []byte) ([]KeyEntrySummary, error) {
	if len(body) == 0 {
		return nil, nil
	}
	var root map[string]any
	if err := json.Unmarshal(body, &root); err != nil {
		return nil, err
	}
	keys, _ := root["keys"].([]any)
	if len(keys) == 0 {
		return nil, nil
	}
	p := strings.TrimSpace(providerName)
	recs := make([]keySortRec, 0, len(keys))
	for _, raw := range keys {
		km, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		name, _ := km["name"].(string)
		name = strings.TrimSpace(name)
		hint, cfg := summarizeKeyValueField(km["value"])
		s := KeyEntrySummary{Name: name, KeyHint: hint, KeyConfigured: cfg}
		var seq int
		var isCh bool
		if idx, ok := parseChimeraKeyIndex(p, name); ok {
			seq = idx
			isCh = true
		}
		recs = append(recs, keySortRec{summary: s, chimeraSeq: seq, isChimera: isCh})
	}
	sort.Slice(recs, func(i, j int) bool {
		a, b := recs[i], recs[j]
		switch {
		case a.isChimera && b.isChimera:
			return a.chimeraSeq < b.chimeraSeq
		case a.isChimera && !b.isChimera:
			return true
		case !a.isChimera && b.isChimera:
			return false
		default:
			return strings.ToLower(a.summary.Name) < strings.ToLower(b.summary.Name)
		}
	})
	out := make([]KeyEntrySummary, len(recs))
	for i := range recs {
		out[i] = recs[i].summary
	}
	return out, nil
}

func summarizeKeyValueField(raw any) (hint string, configured bool) {
	if raw == nil {
		return "not set", false
	}
	switch v := raw.(type) {
	case string:
		s := strings.TrimSpace(v)
		if s == "" {
			return "not set", false
		}
		if strings.HasPrefix(s, "env.") {
			return "env:" + strings.TrimPrefix(s, "env."), true
		}
		if s == "***" || strings.Contains(s, "*") {
			return maskRedactedKey(s), true
		}
		return maskPlainKey(s), true
	case map[string]any:
		fromEnv := false
		switch x := v["from_env"].(type) {
		case bool:
			fromEnv = x
		case float64:
			fromEnv = x != 0
		}
		envVar, _ := v["env_var"].(string)
		if fromEnv && strings.TrimSpace(envVar) != "" {
			return "env:" + strings.TrimSpace(envVar), true
		}
		inner, _ := v["value"].(string)
		inner = strings.TrimSpace(inner)
		if inner == "" || inner == "***" || strings.Contains(inner, "*") {
			if inner != "" {
				return maskRedactedKey(inner), true
			}
			return "not set", false
		}
		return maskPlainKey(inner), true
	default:
		return "not set", false
	}
}

func maskPlainKey(s string) string {
	if len(s) <= 8 {
		return "••••"
	}
	return "••••" + s[len(s)-4:]
}

func maskRedactedKey(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "configured"
	}
	return s
}
