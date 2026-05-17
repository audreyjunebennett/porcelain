package routing

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// ValidatePolicyYAML checks that YAML is parseable as a routing policy and contains at least
// one rule with a non-empty models list (matching what the live gateway loads).
func ValidatePolicyYAML(b []byte) error {
	if len(strings.TrimSpace(string(b))) == 0 {
		return fmt.Errorf("routing policy: empty")
	}
	var doc policyDoc
	if err := yaml.Unmarshal(b, &doc); err != nil {
		return fmt.Errorf("routing policy yaml: %w", err)
	}
	usable := 0
	for _, r := range doc.Rules {
		if len(r.Models) > 0 {
			usable++
		}
	}
	if usable == 0 {
		return fmt.Errorf("routing policy: need at least one rule with non-empty models")
	}
	return nil
}
