// Package operatorcopy holds the operator message registry (canonical slugs, aliases, copy).
//
// Registry YAML is embedded for go generate validation; Phases 2–4 migrate JS switches here.
package operatorcopy

import (
	"fmt"
	"strings"
)

// Registry is the top-level messages.yaml document.
type Registry struct {
	Version       int                  `yaml:"version"`
	Locale        string               `yaml:"locale"`
	Formatters    map[string]Formatter `yaml:"formatters,omitempty"`
	IndexerStates map[string]string    `yaml:"indexer_states,omitempty"`
	Messages      []Message            `yaml:"messages"`
}

// Formatter documents a shared dynamic summary builder (implemented in JS/Go in later phases).
type Formatter struct {
	Description string `yaml:"description"`
}

// Message is one canonical slug and how the logs UI presents it to operators.
type Message struct {
	Slug           string       `yaml:"slug"`
	Summary        string       `yaml:"summary,omitempty"`
	Formatter      string       `yaml:"formatter,omitempty"`
	Append         []Append     `yaml:"append,omitempty"`
	Aliases        []string     `yaml:"aliases,omitempty"`
	MatchFields    *MatchFields `yaml:"match_fields,omitempty"`
	MatchPrefix    bool         `yaml:"match_prefix,omitempty"`
	Shape          string       `yaml:"shape,omitempty"`
	MetricsCounter string       `yaml:"metrics_counter,omitempty"`
	TimelineKind   string       `yaml:"timeline_kind,omitempty"`
	GalleryPreview string       `yaml:"gallery_preview"`
}

// Gateway card counter keys (metrics_counter in messages.yaml must be one of these or empty).
var allowedMetricsCounters = map[string]struct{}{
	"chatReq": {}, "chatResp": {}, "chatErr": {},
	"ragQuery": {}, "ragHit": {},
	"ingestOk": {}, "ingestFail": {},
}

// MatchFields gates alias resolution when slog JSON carries duplicate "msg" keys (human title vs slug).
// All lists are ANDed; require_any and require_any_bool are OR branches inside the any-group.
type MatchFields struct {
	RequireAll     []string `yaml:"require_all,omitempty"`
	RequireAny     []string `yaml:"require_any,omitempty"`
	RequireAnyBool []string `yaml:"require_any_bool,omitempty"`
}

// Append adds a dynamic tail from a log field (rendered in Phase 2+).
type Append struct {
	Field  string `yaml:"field"`
	Fmt    string `yaml:"fmt"`
	OmitIn string `yaml:"omit_in,omitempty"`
}

// Validate checks registry invariants for Phase 1+.
func (r *Registry) Validate() error {
	if r == nil {
		return fmt.Errorf("operatorcopy: nil registry")
	}
	if r.Version != 1 {
		return fmt.Errorf("operatorcopy: unsupported version %d (want 1)", r.Version)
	}
	if strings.TrimSpace(r.Locale) == "" {
		return fmt.Errorf("operatorcopy: locale is required")
	}
	if len(r.Messages) == 0 {
		return fmt.Errorf("operatorcopy: messages list is empty")
	}
	seen := make(map[string]string, len(r.Messages)*2)
	for i, m := range r.Messages {
		if err := m.validate(i, r.Formatters); err != nil {
			return err
		}
		for _, key := range m.allKeys() {
			if prev, ok := seen[key]; ok {
				return fmt.Errorf("operatorcopy: duplicate key %q (messages %q and %q)", key, prev, m.Slug)
			}
			seen[key] = m.Slug
		}
	}
	return nil
}

func (m *Message) allKeys() []string {
	keys := []string{m.Slug}
	keys = append(keys, m.Aliases...)
	return keys
}

func (m *Message) validate(index int, formatters map[string]Formatter) error {
	prefix := fmt.Sprintf("operatorcopy: messages[%d]", index)
	if strings.TrimSpace(m.Slug) == "" {
		return fmt.Errorf("%s: slug is required", prefix)
	}
	if strings.TrimSpace(m.GalleryPreview) == "" {
		return fmt.Errorf("%s (%s): gallery_preview is required", prefix, m.Slug)
	}
	hasSummary := strings.TrimSpace(m.Summary) != ""
	hasFormatter := strings.TrimSpace(m.Formatter) != ""
	if hasSummary == hasFormatter {
		return fmt.Errorf("%s (%s): exactly one of summary or formatter is required", prefix, m.Slug)
	}
	if hasFormatter {
		if formatters == nil {
			return fmt.Errorf("%s (%s): unknown formatter %q (no formatters catalog)", prefix, m.Slug, m.Formatter)
		}
		if _, ok := formatters[m.Formatter]; !ok {
			return fmt.Errorf("%s (%s): unknown formatter %q", prefix, m.Slug, m.Formatter)
		}
	}
	for j, a := range m.Append {
		if strings.TrimSpace(a.Field) == "" {
			return fmt.Errorf("%s (%s): append[%d]: field is required", prefix, m.Slug, j)
		}
		if strings.TrimSpace(a.Fmt) == "" {
			return fmt.Errorf("%s (%s): append[%d]: fmt is required", prefix, m.Slug, j)
		}
	}
	if mc := strings.TrimSpace(m.MetricsCounter); mc != "" {
		if _, ok := allowedMetricsCounters[mc]; !ok {
			return fmt.Errorf("%s (%s): unknown metrics_counter %q", prefix, m.Slug, mc)
		}
	}
	return nil
}

// CanonicalSlugs returns every canonical slug in registry order.
func (r *Registry) CanonicalSlugs() []string {
	out := make([]string, len(r.Messages))
	for i, m := range r.Messages {
		out[i] = m.Slug
	}
	return out
}

// ResolveCanonical maps a flat log msg (slug or alias) to the canonical slug, or "" if unknown.
func (r *Registry) ResolveCanonical(flatMsg string) string {
	msg := strings.TrimSpace(flatMsg)
	if msg == "" {
		return ""
	}
	for _, m := range r.Messages {
		if m.Slug == msg {
			return m.Slug
		}
		for _, a := range m.Aliases {
			if a == msg {
				return m.Slug
			}
		}
	}
	return ""
}
