package providerlimits

import (
	"path/filepath"
	"strings"
	"testing"
)

func mustInt(v int64) *int64 { return &v }

func TestParse_EmptyDocument_yieldsEmptyConfig(t *testing.T) {
	cfg, err := Parse([]byte("   \n# just a comment\n"))
	if err != nil {
		t.Fatalf("parse empty: %v", err)
	}
	if len(cfg.Providers) != 0 {
		t.Fatalf("want no providers, got %d", len(cfg.Providers))
	}
}

func TestParse_Defaults_providersAndModels(t *testing.T) {
	src := `
schema_version: 1
defaults:
  rpm: 10
  rpd: 1000
  usage_day_timezone: UTC
providers:
  groq:
    usage_day_timezone: UTC
    rpm: 30
    rpd: 14400
    tpm: 6000
    models:
      groq/llama-3.3-70b-versatile:
        tpm: 12000
  google:
    usage_day_timezone: America/Los_Angeles
    rpd: 250
`
	cfg, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cfg.SchemaVersion != 1 {
		t.Fatalf("schema: %d", cfg.SchemaVersion)
	}
	if cfg.Defaults.UsageDayTimezone != "UTC" {
		t.Fatalf("defaults tz: %q", cfg.Defaults.UsageDayTimezone)
	}
	groq, ok := cfg.Providers["groq"]
	if !ok {
		t.Fatalf("missing groq")
	}
	if groq.RPM == nil || *groq.RPM != 30 {
		t.Fatalf("groq rpm: %v", groq.RPM)
	}
	if ml, ok := groq.Models["groq/llama-3.3-70b-versatile"]; !ok {
		t.Fatalf("missing model entry")
	} else if ml.TPM == nil || *ml.TPM != 12000 {
		t.Fatalf("model tpm: %v", ml.TPM)
	}
	if cfg.Providers["google"].UsageDayTimezone != "America/Los_Angeles" {
		t.Fatalf("google tz wrong")
	}
}

func TestParse_RejectsUnknownFields(t *testing.T) {
	src := `
schema_version: 1
defaults:
  rpm: 10
  bogus: 42
`
	if _, err := Parse([]byte(src)); err == nil {
		t.Fatalf("expected error for unknown field")
	}
}

func TestParse_NegativeLimits_error(t *testing.T) {
	src := `
providers:
  x:
    usage_day_timezone: UTC
    rpm: -1
`
	_, err := Parse([]byte(src))
	if err == nil || !strings.Contains(err.Error(), "rpm must be >= 0") {
		t.Fatalf("want negative-rpm error, got %v", err)
	}
}

func TestParse_BadTimezone_error(t *testing.T) {
	src := `
providers:
  x:
    usage_day_timezone: Not/A_Zone
`
	if _, err := Parse([]byte(src)); err == nil {
		t.Fatalf("expected bad tz error")
	}
}

func TestParse_DayLimitWithoutTZ_error(t *testing.T) {
	src := `
providers:
  groq:
    rpd: 1000
`
	_, err := Parse([]byte(src))
	if err == nil || !strings.Contains(err.Error(), "usage_day_timezone") {
		t.Fatalf("want tz-required error, got %v", err)
	}
}

func TestParse_DayLimitInheritsDefaultTZ(t *testing.T) {
	src := `
defaults:
  usage_day_timezone: UTC
providers:
  groq:
    rpd: 1000
`
	cfg, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	p := cfg.Providers["groq"]
	if p.UsageDayTimezone != "" {
		t.Fatalf("provider tz unexpectedly populated from defaults")
	}
	if cfg.Defaults.UsageDayTimezone != "UTC" {
		t.Fatalf("defaults tz lost")
	}
}

func TestParse_ModelKeyMustBePrefixedByProvider(t *testing.T) {
	src := `
providers:
  groq:
    usage_day_timezone: UTC
    models:
      openai/gpt-4o:
        tpm: 100
`
	_, err := Parse([]byte(src))
	if err == nil || !strings.Contains(err.Error(), "must start with") {
		t.Fatalf("want prefix error, got %v", err)
	}
}

func TestParse_NegativeContextLimits_error(t *testing.T) {
	src := `
providers:
  x:
    context_window: -1
`
	_, err := Parse([]byte(src))
	if err == nil || !strings.Contains(err.Error(), "context_window must be >= 0") {
		t.Fatalf("want negative context_window error, got %v", err)
	}
}

func TestParse_InvalidContextSafetyFactor_error(t *testing.T) {
	src := `
defaults:
  context_safety_factor: 0
`
	_, err := Parse([]byte(src))
	if err == nil || !strings.Contains(err.Error(), "context_safety_factor must be > 0") {
		t.Fatalf("want invalid safety factor error, got %v", err)
	}
}

func TestParse_SchemaV2ContextFields(t *testing.T) {
	src := `
schema_version: 2
defaults:
  context_safety_factor: 0.9
  max_body_bytes: 3500000
providers:
  groq:
    models:
      groq/groq/compound-mini:
        context_window: 131072
        max_prompt_tokens: 8192
`
	cfg, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cfg.SchemaVersion != 2 {
		t.Fatalf("schema: %d", cfg.SchemaVersion)
	}
	m := cfg.Providers["groq"].Models["groq/groq/compound-mini"]
	if m.ContextWindow == nil || *m.ContextWindow != 131072 {
		t.Fatalf("context_window: %v", m.ContextWindow)
	}
	if m.MaxPromptTokens == nil || *m.MaxPromptTokens != 8192 {
		t.Fatalf("max_prompt_tokens: %v", m.MaxPromptTokens)
	}
}

func TestLoadOrEmpty_missingFile_empty(t *testing.T) {
	cfg, err := LoadOrEmpty(filepath.Join(t.TempDir(), "does-not-exist.yaml"))
	if err != nil {
		t.Fatalf("LoadOrEmpty err: %v", err)
	}
	if cfg == nil || len(cfg.Providers) != 0 {
		t.Fatalf("want empty config")
	}
}
