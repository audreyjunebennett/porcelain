package providerlimits

import (
	"fmt"
	"strings"
	"time"
)

// Effective is the fully-resolved limit set for a specific provider/model id, after layering
// model > provider > defaults. Nil fields mean "not enforced".
type Effective struct {
	Provider string
	ModelID  string

	RPM *int64
	RPD *int64
	TPM *int64
	TPD *int64

	// UsageDayTimezone is the IANA tz used to compute RPD/TPD day buckets for this provider.
	// Empty when no day-scoped limits are enforced (RPD and TPD are both nil).
	UsageDayTimezone string
}

// HasAnyMinuteLimit reports whether RPM or TPM are enforced.
func (e Effective) HasAnyMinuteLimit() bool { return e.RPM != nil || e.TPM != nil }

// HasAnyDayLimit reports whether RPD or TPD are enforced.
func (e Effective) HasAnyDayLimit() bool { return e.RPD != nil || e.TPD != nil }

// SplitProviderModel extracts the leading "<provider>/" segment from a BiFrost-style model id.
// Returns ("", modelID) when no slash is present.
func SplitProviderModel(modelID string) (provider, model string) {
	if i := strings.Index(modelID, "/"); i > 0 {
		return modelID[:i], modelID
	}
	return "", modelID
}

// Resolve returns the effective limits for the given upstream id. An empty Effective is returned
// for unknown providers/models (no enforcement).
func (c *Config) Resolve(upstreamID string) Effective {
	provider, _ := SplitProviderModel(upstreamID)
	eff := Effective{Provider: provider, ModelID: upstreamID}
	if c == nil {
		return eff
	}
	// Start from defaults.
	applyLayer(&eff, c.Defaults)
	if provider == "" {
		return eff
	}
	p, ok := c.Providers[provider]
	if !ok {
		// Keep defaults; tz only relevant when day limits are set — defaults must supply it.
		pruneDayTZIfNoDayLimits(&eff)
		return eff
	}
	applyLayer(&eff, p.Layer)
	if ml, ok := p.Models[upstreamID]; ok {
		applyLayer(&eff, ml)
	}
	pruneDayTZIfNoDayLimits(&eff)
	return eff
}

// applyLayer overlays any non-nil field from l onto eff. Timezone is overlaid when non-empty.
func applyLayer(eff *Effective, l Layer) {
	if l.RPM != nil {
		v := *l.RPM
		eff.RPM = &v
	}
	if l.RPD != nil {
		v := *l.RPD
		eff.RPD = &v
	}
	if l.TPM != nil {
		v := *l.TPM
		eff.TPM = &v
	}
	if l.TPD != nil {
		v := *l.TPD
		eff.TPD = &v
	}
	if strings.TrimSpace(l.UsageDayTimezone) != "" {
		eff.UsageDayTimezone = l.UsageDayTimezone
	}
}

func pruneDayTZIfNoDayLimits(eff *Effective) {
	if eff.RPD == nil && eff.TPD == nil {
		eff.UsageDayTimezone = ""
	}
}

// MinuteKey formats the UTC minute bucket key used in upstream_rollup_minute (YYYY-MM-DDTHH:MM).
// RPM/TPM buckets are always UTC-aligned in our metrics schema; providers do not reset by local
// minute so this key does not need a provider tz.
func MinuteKey(at time.Time) string { return at.UTC().Format("2006-01-02T15:04") }

// DayKey returns the calendar-day bucket key for a given instant in the provider's usage-day
// timezone. The returned string is "YYYY-MM-DD" using that local calendar date — this is the
// value to match against a provider's reset day when aggregating from upstream_call_events.
//
// tz must be a valid IANA name; empty or invalid tz returns ("", error).
func DayKey(at time.Time, tz string) (string, error) {
	if strings.TrimSpace(tz) == "" {
		return "", fmt.Errorf("DayKey: empty timezone")
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return "", fmt.Errorf("DayKey: load %q: %w", tz, err)
	}
	return at.In(loc).Format("2006-01-02"), nil
}

// DayWindow returns the [start, end) UTC instants that bound the local calendar day containing
// at in tz. Useful for summing upstream_call_events rows into a vendor-local day rollup.
func DayWindow(at time.Time, tz string) (start, end time.Time, err error) {
	if strings.TrimSpace(tz) == "" {
		return time.Time{}, time.Time{}, fmt.Errorf("DayWindow: empty timezone")
	}
	loc, e := time.LoadLocation(tz)
	if e != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("DayWindow: load %q: %w", tz, e)
	}
	local := at.In(loc)
	startLocal := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
	endLocal := startLocal.AddDate(0, 0, 1)
	return startLocal.UTC(), endLocal.UTC(), nil
}
