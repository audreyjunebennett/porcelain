package operatorapi

import "time"

// ProviderHealthEntry is one row in the chimera-broker provider health strip.
//
// State values: "up", "down", "key_missing", "unknown", "not_configured".
type ProviderHealthEntry struct {
	ID            string   `json:"id"`
	State         string   `json:"state"`
	KeyConfigured bool     `json:"key_configured"`
	KeyCount      int      `json:"key_count"`
	KeyHint       string   `json:"key_hint,omitempty"`
	ModelIDs      []string `json:"model_ids,omitempty"`
	OllamaBaseURL string   `json:"ollama_base_url,omitempty"`
	HTTPStatus    int      `json:"http_status,omitempty"`
	Error         string   `json:"error,omitempty"`
}

// ProviderHealthResponse is GET /api/ui/chimera-broker/providers.
type ProviderHealthResponse struct {
	FetchedAt time.Time             `json:"fetched_at"`
	BrokerUp  bool                  `json:"chimera_broker_up"`
	Error     string                `json:"error,omitempty"`
	Providers []ProviderHealthEntry `json:"providers"`
}

// ProviderKeyEntry is one API key row in GET /api/ui/state provider entries.
type ProviderKeyEntry struct {
	Name          string `json:"name"`
	KeyHint       string `json:"key_hint"`
	KeyConfigured bool   `json:"key_configured"`
}

// StateProviderEntry is one provider block inside GET /api/ui/state (keyed by provider name).
type StateProviderEntry struct {
	Provider      string             `json:"provider"`
	OK            bool               `json:"ok"`
	Error         string             `json:"error,omitempty"`
	KeyConfigured bool               `json:"key_configured"`
	KeyHint       string             `json:"key_hint,omitempty"`
	Keys          []ProviderKeyEntry `json:"keys,omitempty"`
	OllamaBaseURL string             `json:"ollama_base_url,omitempty"`
	HTTPStatus    int                `json:"http_status,omitempty"`
}
