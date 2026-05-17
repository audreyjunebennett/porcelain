package providerfreetier

import (
	"fmt"
	"os"
	"path"
	"strings"

	"gopkg.in/yaml.v3"
)

// Spec is an operator-maintained allowlist of upstream model ids (BiFrost style: provider/model).
type Spec struct {
	FormatVersion int      `yaml:"format_version"`
	EffectiveDate string   `yaml:"effective_date"`
	Models        []string `yaml:"models"`   // exact ids and/or globs (see Match)
	Patterns      []string `yaml:"patterns"` // shell-style globs; provider/* = whole provider
}

// Load reads and parses provider-free-tier YAML. An empty or missing models/patterns list is valid.
func Load(filePath string) (*Spec, error) {
	raw, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	var s Spec
	if err := yaml.Unmarshal(raw, &s); err != nil {
		return nil, fmt.Errorf("parse provider free tier yaml: %w", err)
	}
	if s.FormatVersion != 1 {
		return nil, fmt.Errorf("provider free tier: unsupported format_version %d (supported: 1)", s.FormatVersion)
	}
	return &s, nil
}

// Empty reports whether there are no exact entries and no patterns.
func (s *Spec) Empty() bool {
	if s == nil {
		return true
	}
	return len(s.Models) == 0 && len(s.Patterns) == 0
}

// Match returns true if id is allowed by models (exact or glob) or patterns.
//
// Entries in models are normally full catalog ids. An entry containing *, ?, or [
// is matched with the same rules as patterns. Additionally, a pattern of the form
// "provider/*" where provider contains no "/" matches any id with prefix "provider/"
// (all catalog models for that BiFrost provider), including nested paths such as
// ollama/library/llama3.
func (s *Spec) Match(id string) bool {
	if s == nil {
		return false
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return false
	}
	for _, m := range s.Models {
		if matchListedModel(strings.TrimSpace(m), id) {
			return true
		}
	}
	for _, p := range s.Patterns {
		if matchPattern(strings.TrimSpace(p), id) {
			return true
		}
	}
	return false
}

func matchListedModel(m, id string) bool {
	if m == "" {
		return false
	}
	if !strings.ContainsAny(m, "*?[") {
		return m == id
	}
	return matchPattern(m, id)
}

func matchPattern(p, id string) bool {
	if p == "" {
		return false
	}
	if prefix, ok := providerWildcardPrefix(p); ok {
		return strings.HasPrefix(id, prefix)
	}
	ok, err := path.Match(p, id)
	return err == nil && ok
}

// providerWildcardPrefix reports whether p is "provider/*" with a single path
// segment before the slash; the match prefix is "provider/".
func providerWildcardPrefix(p string) (prefix string, ok bool) {
	if !strings.HasSuffix(p, "/*") {
		return "", false
	}
	base := strings.TrimSuffix(p, "/*")
	if base == "" || strings.Contains(base, "/") {
		return "", false
	}
	return base + "/", true
}

// Filter returns ids that match the spec, preserving input order.
func (s *Spec) Filter(ids []string) []string {
	if s == nil {
		return nil
	}
	var out []string
	seen := make(map[string]struct{})
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || !s.Match(id) {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}
