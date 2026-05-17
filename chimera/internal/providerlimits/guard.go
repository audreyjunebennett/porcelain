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
// Zero-value Guard (nil Cfg or nil Store) always allows.
type Guard struct {
	Cfg   *Config
	Usage UsageSource
	// Now returns the current instant; injected for tests. Defaults to time.Now when nil.
	Now func() time.Time
}

// Allow returns a Decision for "send estTokens of request to upstreamID right now". Any error
// looking up usage is non-fatal: the guard allows the call and reports the error so callers can
// log. Rationale: the gateway must degrade to "no enforcement" when metrics are unreadable.
func (g *Guard) Allow(ctx context.Context, upstreamID string, estTokens int64) (Decision, error) {
	if g == nil || g.Cfg == nil || g.Usage == nil {
		return Decision{Allowed: true}, nil
	}
	eff := g.Cfg.Resolve(upstreamID)
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
	return Decide(eff, usage, estTokens), nil
}
