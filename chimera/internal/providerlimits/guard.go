package providerlimits

import (
	"context"
	"time"
)

// UsageSource is the minimal interface Guard needs from the metrics store. A thin adapter in
// the server package wraps gatewaymetrics.Store to satisfy it; keeping the interface here
// prevents a hard dependency from providerlimits onto the SQLite package.
type UsageSource interface {
	UsageForModelWindow(ctx context.Context, modelID string, start, end time.Time) (calls, estTokens int64, err error)
}

// Guard composes a limits Config with a live metrics store to answer: can this request proceed?
// Zero-value Guard (nil Cfg) always allows. Context checks run without metrics; RPM/TPM need Usage.
type Guard struct {
	Cfg   *Config
	Usage UsageSource
	// Catalog optionally overlays context_window from the live broker catalog when YAML omits it.
	Catalog ContextCatalog
	// Now returns the current instant; injected for tests. Defaults to time.Now when nil.
	Now func() time.Time
}

func (g *Guard) resolveEffective(upstreamID string) Effective {
	if g == nil || g.Cfg == nil {
		return Effective{ModelID: upstreamID}
	}
	return g.Cfg.ResolveWithCatalog(upstreamID, g.Catalog)
}

// EffectiveFor returns fully resolved limits for upstreamID, including live catalog context overlay.
func (g *Guard) EffectiveFor(upstreamID string) Effective {
	if g == nil {
		return Effective{ModelID: upstreamID}
	}
	return g.resolveEffective(upstreamID)
}

// Allow returns a Decision for sending req to upstreamID right now. Context/body checks run
// first without metrics; RPM/TPM/RPD/TPD require a Usage source. Usage lookup errors are
// non-fatal for quota checks: the guard allows the call and reports the error so callers can
// log. Rationale: the gateway must degrade to "no quota enforcement" when metrics are unreadable.
func (g *Guard) Allow(ctx context.Context, upstreamID string, req RequestAdmission) (Decision, error) {
	if g == nil || g.Cfg == nil {
		return Decision{Allowed: true}, nil
	}
	eff := g.resolveEffective(upstreamID)
	if cd := DecideContext(eff, req); !cd.Allowed {
		return cd, nil
	}
	if g.Usage == nil {
		return Decision{Allowed: true}, nil
	}
	if !eff.HasAnyMinuteLimit() && !eff.HasAnyDayLimit() {
		return Decision{Allowed: true}, nil
	}
	now := time.Now
	if g.Now != nil {
		now = g.Now
	}
	at := now()

	var usage Usage
	// Minute window (UTC).
	if eff.HasAnyMinuteLimit() {
		minStart := at.UTC().Truncate(time.Minute)
		minEnd := minStart.Add(time.Minute)
		calls, tok, err := g.Usage.UsageForModelWindow(ctx, upstreamID, minStart, minEnd)
		if err != nil {
			return Decision{Allowed: true}, err
		}
		usage.MinuteCalls = calls
		usage.MinuteEstTokens = tok
	}
	// Day window in provider local tz.
	if eff.HasAnyDayLimit() {
		if eff.UsageDayTimezone == "" {
			// Validated at Parse time; defensively allow if it ever happens.
			return Decision{Allowed: true}, nil
		}
		start, end, err := DayWindow(at, eff.UsageDayTimezone)
		if err != nil {
			return Decision{Allowed: true}, err
		}
		calls, tok, err := g.Usage.UsageForModelWindow(ctx, upstreamID, start, end)
		if err != nil {
			return Decision{Allowed: true}, err
		}
		usage.DayCalls = calls
		usage.DayEstTokens = tok
	}
	return Decide(eff, usage, req.EstPromptTokens), nil
}
