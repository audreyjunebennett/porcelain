package tokens

import (
	"os"

	"gopkg.in/yaml.v3"
)

// IsBootstrapMode reports whether the gateway should run in bootstrap:
// missing credential file, unreadable, unparseable YAML, or zero valid rows
// (non-empty token and tenant_id), matching runtime validation in ReloadIfStale.
func IsBootstrapMode(path string) bool {
	st, err := os.Stat(path)
	if err != nil || st.IsDir() {
		return true
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return true
	}
	doc := defaultCredentialDoc()
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return true
	}
	for _, row := range doc.APIKeys {
		if rowIsValidSecret(row.Secret, row.TenantID) {
			return false
		}
	}
	return true
}
