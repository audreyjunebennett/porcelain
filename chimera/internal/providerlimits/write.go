package providerlimits

import (
	"bytes"
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type writeDocument struct {
	SchemaVersion int                      `yaml:"schema_version"`
	Defaults      writeDefaultsLayer       `yaml:"defaults"`
	Providers     map[string]writeProvider `yaml:"providers"`
}

type writeProvider struct {
	writeLayer `yaml:",inline"`
	Models     map[string]writeLayer `yaml:"models,omitempty"`
}

type writeDefaultsLayer struct {
	RPM                 *int64   `yaml:"rpm"`
	RPD                 *int64   `yaml:"rpd"`
	TPM                 *int64   `yaml:"tpm"`
	TPD                 *int64   `yaml:"tpd"`
	ContextWindow       *int64   `yaml:"context_window,omitempty"`
	MaxPromptTokens     *int64   `yaml:"max_prompt_tokens,omitempty"`
	MaxBodyBytes        *int64   `yaml:"max_body_bytes,omitempty"`
	ContextSafetyFactor *float64 `yaml:"context_safety_factor,omitempty"`
	UsageDayTimezone    *string  `yaml:"usage_day_timezone,omitempty"`
}

type writeLayer struct {
	RPM                 *int64   `yaml:"rpm,omitempty"`
	RPD                 *int64   `yaml:"rpd,omitempty"`
	TPM                 *int64   `yaml:"tpm,omitempty"`
	TPD                 *int64   `yaml:"tpd,omitempty"`
	ContextWindow       *int64   `yaml:"context_window,omitempty"`
	MaxPromptTokens     *int64   `yaml:"max_prompt_tokens,omitempty"`
	MaxBodyBytes        *int64   `yaml:"max_body_bytes,omitempty"`
	ContextSafetyFactor *float64 `yaml:"context_safety_factor,omitempty"`
	UsageDayTimezone    *string  `yaml:"usage_day_timezone,omitempty"`
}

// Write encodes cfg as provider-model-limits.yaml bytes (sorted providers/models).
func Write(cfg *Config) ([]byte, error) {
	if cfg == nil {
		return nil, fmt.Errorf("Write: nil config")
	}
	doc := writeDocument{
		SchemaVersion: cfg.SchemaVersion,
		Defaults:      layerToWriteDefaults(cfg.Defaults),
		Providers:     make(map[string]writeProvider, len(cfg.Providers)),
	}
	for _, name := range sortedProviderKeys(cfg.Providers) {
		p := cfg.Providers[name]
		wp := writeProvider{writeLayer: layerToWriteLayer(p.Layer)}
		if len(p.Models) > 0 {
			wp.Models = make(map[string]writeLayer, len(p.Models))
			for _, mid := range sortedModelKeys(p.Models) {
				wp.Models[mid] = layerToWriteLayer(p.Models[mid])
			}
		}
		doc.Providers[name] = wp
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		return nil, fmt.Errorf("encode provider-model-limits: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("close yaml encoder: %w", err)
	}
	return buf.Bytes(), nil
}

// WriteFile writes cfg to path with an optional header comment block.
func WriteFile(path string, cfg *Config, header string) error {
	body, err := Write(cfg)
	if err != nil {
		return err
	}
	var out bytes.Buffer
	if header != "" {
		out.WriteString(header)
		if header[len(header)-1] != '\n' {
			out.WriteByte('\n')
		}
		out.WriteByte('\n')
	}
	out.Write(body)
	return os.WriteFile(path, out.Bytes(), 0o644)
}

func layerToWriteDefaults(l Layer) writeDefaultsLayer {
	wl := writeDefaultsLayer{
		RPM:                 copyInt64(l.RPM),
		RPD:                 copyInt64(l.RPD),
		TPM:                 copyInt64(l.TPM),
		TPD:                 copyInt64(l.TPD),
		ContextWindow:       copyInt64(l.ContextWindow),
		MaxPromptTokens:     copyInt64(l.MaxPromptTokens),
		MaxBodyBytes:        copyInt64(l.MaxBodyBytes),
		ContextSafetyFactor: copyFloat64(l.ContextSafetyFactor),
	}
	if tz := strings.TrimSpace(l.UsageDayTimezone); tz != "" {
		wl.UsageDayTimezone = &tz
	}
	return wl
}

func layerToWriteLayer(l Layer) writeLayer {
	wl := writeLayer{
		RPM:                 copyInt64(l.RPM),
		RPD:                 copyInt64(l.RPD),
		TPM:                 copyInt64(l.TPM),
		TPD:                 copyInt64(l.TPD),
		ContextWindow:       copyInt64(l.ContextWindow),
		MaxPromptTokens:     copyInt64(l.MaxPromptTokens),
		MaxBodyBytes:        copyInt64(l.MaxBodyBytes),
		ContextSafetyFactor: copyFloat64(l.ContextSafetyFactor),
	}
	if tz := strings.TrimSpace(l.UsageDayTimezone); tz != "" {
		wl.UsageDayTimezone = &tz
	}
	return wl
}

func copyInt64(v *int64) *int64 {
	if v == nil {
		return nil
	}
	x := *v
	return &x
}

func copyFloat64(v *float64) *float64 {
	if v == nil {
		return nil
	}
	x := *v
	return &x
}

func sortedProviderKeys(m map[string]Provider) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedModelKeys(m map[string]Layer) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
