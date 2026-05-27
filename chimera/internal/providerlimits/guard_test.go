package providerlimits

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeUsage struct {
	// calls indexed by (modelID, start, end)
	minute map[string][2]int64
	day    map[string][2]int64
	err    error
}

func (f *fakeUsage) UsageForModelWindow(_ context.Context, modelID string, start, end time.Time) (int64, int64, error) {
	if f.err != nil {
		return 0, 0, f.err
	}
	// Minute windows are always exactly 1 minute in length; day windows are > 1 hour.
	dur := end.Sub(start)
	if dur == time.Minute {
		v := f.minute[modelID]
		return v[0], v[1], nil
	}
	v := f.day[modelID]
	return v[0], v[1], nil
}

func mustCfg(t *testing.T, src string) *Config {
	t.Helper()
	c, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return c
}

func TestGuard_NilCfg_allows(t *testing.T) {
	g := &Guard{}
	d, err := g.Allow(context.Background(), "groq/fast", RequestAdmission{EstPromptTokens: 1000})
	if err != nil || !d.Allowed {
		t.Fatalf("want allow, got %+v err=%v", d, err)
	}
}

func TestGuard_NoLimits_allows(t *testing.T) {
	cfg := mustCfg(t, `providers: {}`)
	g := &Guard{Cfg: cfg, Usage: &fakeUsage{}}
	d, err := g.Allow(context.Background(), "unknown/one", RequestAdmission{EstPromptTokens: 1_000_000})
	if err != nil || !d.Allowed {
		t.Fatalf("want allow, got %+v err=%v", d, err)
	}
}

func TestGuard_DeniesRPMUsingMinuteWindow(t *testing.T) {
	cfg := mustCfg(t, `
providers:
  groq:
    usage_day_timezone: UTC
    rpm: 3
`)
	now := time.Date(2026, 4, 16, 18, 0, 30, 0, time.UTC)
	fu := &fakeUsage{minute: map[string][2]int64{"groq/fast": {3, 0}}}
	g := &Guard{Cfg: cfg, Usage: fu, Now: func() time.Time { return now }}

	d, err := g.Allow(context.Background(), "groq/fast", RequestAdmission{EstPromptTokens: 100})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if d.Allowed || d.Reason != ReasonRPM {
		t.Fatalf("want RPM deny, got %+v", d)
	}
}

func TestGuard_DeniesRPDUsingVendorLocalDay(t *testing.T) {
	cfg := mustCfg(t, `
providers:
  google:
    usage_day_timezone: America/Los_Angeles
    rpd: 50
`)
	// 06:30 UTC on 2026-04-17 is still 2026-04-16 in LA.
	now := time.Date(2026, 4, 17, 6, 30, 0, 0, time.UTC)
	fu := &fakeUsage{day: map[string][2]int64{"google/gemini": {50, 0}}}
	g := &Guard{Cfg: cfg, Usage: fu, Now: func() time.Time { return now }}

	d, err := g.Allow(context.Background(), "google/gemini", RequestAdmission{})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if d.Allowed || d.Reason != ReasonRPD {
		t.Fatalf("want RPD deny, got %+v", d)
	}
}

func TestGuard_UsageError_degradesToAllow(t *testing.T) {
	cfg := mustCfg(t, `
providers:
  groq:
    usage_day_timezone: UTC
    rpm: 1
`)
	fu := &fakeUsage{err: errors.New("boom")}
	g := &Guard{Cfg: cfg, Usage: fu, Now: func() time.Time { return time.Unix(0, 0) }}

	d, err := g.Allow(context.Background(), "groq/fast", RequestAdmission{EstPromptTokens: 10})
	if !d.Allowed {
		t.Fatalf("should allow on error, got %+v", d)
	}
	if err == nil || err.Error() != "boom" {
		t.Fatalf("want error propagated, got %v", err)
	}
}

func TestGuard_AllowsWhenHeadroomRemains(t *testing.T) {
	cfg := mustCfg(t, `
providers:
  groq:
    usage_day_timezone: UTC
    rpm: 30
    tpm: 6000
    rpd: 1000
`)
	fu := &fakeUsage{
		minute: map[string][2]int64{"groq/fast": {5, 500}},
		day:    map[string][2]int64{"groq/fast": {100, 50_000}},
	}
	g := &Guard{Cfg: cfg, Usage: fu, Now: func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }}
	d, err := g.Allow(context.Background(), "groq/fast", RequestAdmission{EstPromptTokens: 200})
	if err != nil || !d.Allowed {
		t.Fatalf("want allow, got %+v err=%v", d, err)
	}
}

func TestGuard_EnforcesContextWithoutUsageStore(t *testing.T) {
	cfg := mustCfg(t, `
defaults:
  context_safety_factor: 1.0
providers:
  groq:
    models:
      groq/groq/compound-mini:
        max_prompt_tokens: 8192
`)
	g := &Guard{Cfg: cfg, Usage: nil}
	d, err := g.Allow(context.Background(), "groq/groq/compound-mini", RequestAdmission{EstPromptTokens: 9000, MaxTokens: 500})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if d.Allowed || d.Reason != ReasonContext {
		t.Fatalf("want context deny without metrics, got %+v", d)
	}
	d2, err := g.Allow(context.Background(), "groq/groq/compound-mini", RequestAdmission{EstPromptTokens: 7000, MaxTokens: 500})
	if err != nil || !d2.Allowed {
		t.Fatalf("want allow under cap, got %+v err=%v", d2, err)
	}
}

func TestGuard_ContextCheckedBeforeQuotaDeny(t *testing.T) {
	cfg := mustCfg(t, `
providers:
  groq:
    usage_day_timezone: UTC
    tpm: 100
    models:
      groq/small:
        max_prompt_tokens: 500
        context_safety_factor: 1.0
`)
	fu := &fakeUsage{minute: map[string][2]int64{"groq/small": {0, 0}}}
	g := &Guard{Cfg: cfg, Usage: fu, Now: func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }}
	d, err := g.Allow(context.Background(), "groq/small", RequestAdmission{EstPromptTokens: 600})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if d.Allowed || d.Reason != ReasonContext {
		t.Fatalf("context should deny before TPM, got %+v", d)
	}
}
