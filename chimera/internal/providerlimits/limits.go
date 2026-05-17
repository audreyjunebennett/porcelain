// Package providerlimits parses config/provider-model-limits.yaml (plan §3.7) and resolves
// effective (RPM/RPD/TPM/TPD + day-reset timezone) ceilings for a given provider/model id.
// Values that are unset at all layers mean "no enforceable cap" for that dimension.
package providerlimits

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Dimension keys used across the package.
const (
	DimRPM = "rpm"
	DimRPD = "rpd"
	DimTPM = "tpm"
	DimTPD = "tpd"
)

// limitsBlock mirrors the YAML shape: any of rpm/rpd/tpm/tpd (non-negative ints) plus optional tz.
type limitsBlock struct {
	RPM *int64 `yaml:"rpm"`
	RPD *int64 `yaml:"rpd"`
	TPM *int64 `yaml:"tpm"`
	TPD *int64 `yaml:"tpd"`

	UsageDayTimezone *string `yaml:"usage_day_timezone"`
}

// providerBlock is a limitsBlock plus a models map keyed by full "provider/model" id.
type providerBlock struct {
	limitsBlock `yaml:",inline"`
	Models      map[string]limitsBlock `yaml:"models"`
}

type document struct {
	SchemaVersion int                      `yaml:"schema_version"`
	Defaults      limitsBlock              `yaml:"defaults"`
	Providers     map[string]providerBlock `yaml:"providers"`
}

// Config is the parsed + validated in-memory form.
type Config struct {
	SchemaVersion int
	Defaults      Layer
	Providers     map[string]Provider
}

// Layer holds numeric limits and optional tz for one layer (defaults / provider / model).
type Layer struct {
	RPM *int64
	RPD *int64
	TPM *int64
	TPD *int64
	// UsageDayTimezone is the IANA name (e.g. "UTC", "America/Los_Angeles") or "" when unset.
	UsageDayTimezone string
}

// Provider carries the merged provider layer and per-model overrides.
type Provider struct {
	Layer
	Models map[string]Layer
}

// Load parses provider-model-limits.yaml at path. Missing file is NOT an error here — callers
// decide whether to treat that as "no limits configured" (see LoadOrEmpty).
func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return Parse(raw)
}

// LoadOrEmpty returns an empty (no enforcement) Config when the file does not exist, otherwise
// behaves like Load.
func LoadOrEmpty(path string) (*Config, error) {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return &Config{SchemaVersion: 0, Providers: map[string]Provider{}}, nil
		}
		return nil, err
	}
	return Load(path)
}

// Parse decodes YAML bytes into a validated Config (unknown fields are rejected).
// An empty or whitespace-only document yields an empty Config with no providers.
func Parse(raw []byte) (*Config, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return &Config{Providers: map[string]Provider{}}, nil
	}
	var doc document
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(&doc); err != nil && err != io.EOF {
		return nil, fmt.Errorf("parse provider-model-limits: %w", err)
	}

	cfg := &Config{
		SchemaVersion: doc.SchemaVersion,
		Defaults:      toLayer(doc.Defaults),
		Providers:     make(map[string]Provider, len(doc.Providers)),
	}
	if err := validateLayer("defaults", cfg.Defaults); err != nil {
		return nil, err
	}
	for name, pb := range doc.Providers {
		if strings.TrimSpace(name) == "" {
			return nil, fmt.Errorf("provider key must be non-empty")
		}
		p := Provider{Layer: toLayer(pb.limitsBlock)}
		if err := validateLayer("providers."+name, p.Layer); err != nil {
			return nil, err
		}
		if len(pb.Models) > 0 {
			p.Models = make(map[string]Layer, len(pb.Models))
			for mid, mb := range pb.Models {
				if strings.TrimSpace(mid) == "" {
					return nil, fmt.Errorf("provider %q: model key must be non-empty", name)
				}
				if !strings.HasPrefix(mid, name+"/") {
					return nil, fmt.Errorf("provider %q: model id %q must start with %q/", name, mid, name)
				}
				ml := toLayer(mb)
				if err := validateLayer(fmt.Sprintf("providers.%s.models[%s]", name, mid), ml); err != nil {
					return nil, err
				}
				p.Models[mid] = ml
			}
		}
		effectiveTZ := p.UsageDayTimezone
		if effectiveTZ == "" {
			effectiveTZ = cfg.Defaults.UsageDayTimezone
		}
		if (dayNeedsTZ(p.Layer) || anyModelDayNeedsTZ(p.Models)) && strings.TrimSpace(effectiveTZ) == "" {
			return nil, fmt.Errorf("provider %q: rpd/tpd set but no usage_day_timezone on provider or defaults", name)
		}
		cfg.Providers[name] = p
	}
	return cfg, nil
}

func anyModelDayNeedsTZ(models map[string]Layer) bool {
	for _, l := range models {
		if dayNeedsTZ(l) {
			return true
		}
	}
	return false
}

func toLayer(b limitsBlock) Layer {
	l := Layer{RPM: b.RPM, RPD: b.RPD, TPM: b.TPM, TPD: b.TPD}
	if b.UsageDayTimezone != nil {
		l.UsageDayTimezone = strings.TrimSpace(*b.UsageDayTimezone)
	}
	return l
}

func validateLayer(where string, l Layer) error {
	for _, p := range []struct {
		name string
		v    *int64
	}{{"rpm", l.RPM}, {"rpd", l.RPD}, {"tpm", l.TPM}, {"tpd", l.TPD}} {
		if p.v == nil {
			continue
		}
		if *p.v < 0 {
			return fmt.Errorf("%s: %s must be >= 0 (got %d)", where, p.name, *p.v)
		}
	}
	if l.UsageDayTimezone != "" {
		if _, err := time.LoadLocation(l.UsageDayTimezone); err != nil {
			return fmt.Errorf("%s: usage_day_timezone %q: %w", where, l.UsageDayTimezone, err)
		}
	}
	return nil
}

func dayNeedsTZ(l Layer) bool {
	return l.RPD != nil || l.TPD != nil
}
