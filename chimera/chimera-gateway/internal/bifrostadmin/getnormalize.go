package bifrostadmin

import (
	"encoding/json"
	"net/http"
	"strings"
)

// NormalizeProviderGETForMerge returns JSON to pass into AppendProviderAPIKey / RemoveProviderKeyByName / MergeOllamaBaseURL
// after GET /api/providers/{name}. BiFrost may answer “not found” as HTTP 404 or as HTTP 2xx
// with an error envelope (status_code 404 in JSON).
func NormalizeProviderGETForMerge(st int, body []byte) (forMerge []byte, ok bool) {
	if IsProviderMissingGET(st, body) {
		return []byte("{}"), true
	}
	if st >= 200 && st < 300 {
		return body, true
	}
	return body, false
}

func providerNotFoundEnvelope(b []byte) bool {
	if len(b) == 0 {
		return false
	}
	var top map[string]any
	if err := json.Unmarshal(b, &top); err != nil {
		return false
	}
	sc, hasSC := top["status_code"]
	if !hasSC {
		return false
	}
	code, ok := statusCodeAsInt(sc)
	if !ok || code != http.StatusNotFound {
		return false
	}
	if _, has := top["is_chimera_broker_error"]; has {
		return true
	}
	if errObj, ok := top["error"].(map[string]any); ok {
		msg, _ := errObj["message"].(string)
		if strings.Contains(strings.ToLower(msg), "provider not found") {
			return true
		}
	}
	return false
}

// IsProviderMissingGET is true when GET /api/providers/{name} indicates the provider does not exist yet
// (HTTP 404 or a 2xx JSON error envelope with status_code 404).
func IsProviderMissingGET(st int, body []byte) bool {
	if st == http.StatusNotFound {
		return true
	}
	return st >= 200 && st < 300 && providerNotFoundEnvelope(body)
}

func statusCodeAsInt(v any) (int, bool) {
	switch x := v.(type) {
	case float64:
		return int(x), true
	case int:
		return x, true
	case json.Number:
		i, err := x.Int64()
		if err != nil {
			return 0, false
		}
		return int(i), true
	default:
		return 0, false
	}
}
