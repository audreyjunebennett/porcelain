// Package cataloglimits seeds context_window values in provider-model-limits.yaml from
// catalog-available.snapshot.yaml (context_length) with optional Ollama static defaults.
package cataloglimits

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/lynn/porcelain/chimera/internal/providerlimits"
	"gopkg.in/yaml.v3"
)

const schemaVersion = 2

// OllamaContextDefaults supplies context_window when the catalog snapshot omits context_length
// (common for local Ollama models). Keys are full BiFrost ids (e.g. ollama/llama3.2:3b).
var OllamaContextDefaults = map[string]int64{
	"ollama/llama3.2:3b": 131072,
	"ollama/qwen3.5:9b":  131072,
}

// PromptTokenOverrides are operator-maintained stricter caps applied on every seed run.
var PromptTokenOverrides = map[string]int64{
	"groq/groq/compound-mini": 8192,
}

// ApplyOptions controls catalog seeding behavior.
type ApplyOptions struct {
	Force bool
}

// ApplyReport summarizes a seed run.
type ApplyReport struct {
	Updated []string
	Added   []string
	Skipped []string
}

// LoadCatalogContextLengths reads catalog-available.snapshot.yaml and returns model id → context length.
func LoadCatalogContextLengths(path string) (map[string]int64, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read catalog %s: %w", path, err)
	}
	var doc struct {
		Data []map[string]any `yaml:"data"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parse catalog %s: %w", path, err)
	}
	out := make(map[string]int64, len(doc.Data))
	for _, row := range doc.Data {
		id, _ := row["id"].(string)
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if n, ok := int64FromAny(row["context_length"]); ok && n > 0 {
			out[id] = n
		}
	}
	return out, nil
}

// LoadFallbackChain reads routing.fallback_chain from gateway.yaml when present.
func LoadFallbackChain(path string) ([]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read gateway %s: %w", path, err)
	}
	var doc struct {
		Routing struct {
			FallbackChain []string `yaml:"fallback_chain"`
		} `yaml:"routing"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parse gateway %s: %w", path, err)
	}
	var out []string
	for _, id := range doc.Routing.FallbackChain {
		if id = strings.TrimSpace(id); id != "" {
			out = append(out, id)
		}
	}
	return out, nil
}

// ApplyContextWindows merges context_window (and known prompt overrides) into cfg.
// ensureModels lists ids that must exist after seeding (e.g. gateway fallback_chain entries).
func ApplyContextWindows(cfg *providerlimits.Config, catalog map[string]int64, ensureModels []string, opts ApplyOptions) ApplyReport {
	if cfg == nil {
		return ApplyReport{}
	}
	if cfg.SchemaVersion < schemaVersion {
		cfg.SchemaVersion = schemaVersion
	}
	ensureSchemaDefaults(cfg)

	var rep ApplyReport
	seen := map[string]struct{}{}
	for _, id := range ensureModels {
		seen[id] = struct{}{}
	}
	for prov, p := range cfg.Providers {
		for mid := range p.Models {
			seen[mid] = struct{}{}
		}
		_ = prov
	}

	for id := range seen {
		ctxLen, source := resolveContextLength(id, catalog, OllamaContextDefaults)
		if ctxLen <= 0 {
			rep.Skipped = append(rep.Skipped, id)
			continue
		}
		provider, _ := providerlimits.SplitProviderModel(id)
		if provider == "" {
			rep.Skipped = append(rep.Skipped, id)
			continue
		}
		added := ensureModelLayer(cfg, provider, id)
		layer := cfg.Providers[provider].Models[id]
		if opts.Force || layer.ContextWindow == nil {
			v := ctxLen
			layer.ContextWindow = &v
			if added {
				rep.Added = append(rep.Added, id+"("+source+")")
			} else {
				rep.Updated = append(rep.Updated, id+"("+source+")")
			}
		} else if added {
			rep.Added = append(rep.Added, id+"(existing)")
		}
		if override, ok := PromptTokenOverrides[id]; ok {
			v := override
			layer.MaxPromptTokens = &v
		}
		cfg.Providers[provider].Models[id] = layer
	}
	sort.Strings(rep.Updated)
	sort.Strings(rep.Added)
	sort.Strings(rep.Skipped)
	return rep
}

func ensureSchemaDefaults(cfg *providerlimits.Config) {
	if cfg.Defaults.ContextSafetyFactor == nil {
		v := 0.9
		cfg.Defaults.ContextSafetyFactor = &v
	}
	if cfg.Defaults.MaxBodyBytes == nil {
		v := int64(3500000)
		cfg.Defaults.MaxBodyBytes = &v
	}
	if strings.TrimSpace(cfg.Defaults.UsageDayTimezone) == "" {
		cfg.Defaults.UsageDayTimezone = "UTC"
	}
}

func ensureModelLayer(cfg *providerlimits.Config, provider, modelID string) bool {
	if cfg.Providers == nil {
		cfg.Providers = map[string]providerlimits.Provider{}
	}
	p, ok := cfg.Providers[provider]
	if !ok {
		p = providerlimits.Provider{Models: map[string]providerlimits.Layer{}}
	}
	if p.Models == nil {
		p.Models = map[string]providerlimits.Layer{}
	}
	_, exists := p.Models[modelID]
	if !exists {
		p.Models[modelID] = providerlimits.Layer{}
	}
	cfg.Providers[provider] = p
	return !exists
}

func resolveContextLength(id string, catalog, ollamaDefaults map[string]int64) (int64, string) {
	if n, ok := catalog[id]; ok && n > 0 {
		return n, "catalog"
	}
	if strings.HasPrefix(id, "ollama/") {
		if n, ok := ollamaDefaults[id]; ok && n > 0 {
			return n, "ollama-default"
		}
	}
	return 0, ""
}

func int64FromAny(v any) (int64, bool) {
	switch n := v.(type) {
	case int:
		return int64(n), true
	case int64:
		return n, true
	case float64:
		return int64(n), true
	case uint64:
		return int64(n), true
	default:
		return 0, false
	}
}

const limitsFileHeader = `# Provider/model usage limits from vendor tables (schema_version 2: rpm/rpd/tpm/tpd + context caps).
# Groq: temp/groq-limits.txt (Whisper ASH/ASD not modeled). Gemini: temp/gemini-limits.txt
# ("-" / "unlimited" → dimension omitted). Gemini day buckets use America/Los_Angeles.
# groq: https://console.groq.com/docs/rate-limits
# gemini: https://ai.google.dev/gemini-api/docs/pricing
#
# context_window values seeded from catalog-available.snapshot.yaml via: make catalog-limits
# Effective token cap: floor(min(context_window, max_prompt_tokens_if_set) × context_safety_factor)
`

// WriteLimitsFile writes cfg to path with the standard operator header.
func WriteLimitsFile(path string, cfg *providerlimits.Config) error {
	return providerlimits.WriteFile(path, cfg, limitsFileHeader)
}
