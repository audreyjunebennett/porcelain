package providerlimits

import (
	"testing"
	"time"
)

func TestResolve_modelOverridesProviderOverridesDefaults(t *testing.T) {
	cfg, err := Parse([]byte(`
defaults:
  rpm: 5
  tpm: 100
  usage_day_timezone: UTC
providers:
  groq:
    usage_day_timezone: UTC
    rpm: 30
    rpd: 1000
    tpm: 6000
    models:
      groq/fast:
        tpm: 12000
`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	e := cfg.Resolve("groq/fast")
	if e.RPM == nil || *e.RPM != 30 {
		t.Fatalf("rpm inherited from provider: got %v", e.RPM)
	}
	if e.TPM == nil || *e.TPM != 12000 {
		t.Fatalf("tpm from model override: got %v", e.TPM)
	}
	if e.RPD == nil || *e.RPD != 1000 {
		t.Fatalf("rpd from provider: got %v", e.RPD)
	}
	if e.TPD != nil {
		t.Fatalf("tpd should be nil (no layer set): got %v", e.TPD)
	}
	if e.UsageDayTimezone != "UTC" {
		t.Fatalf("tz: %q", e.UsageDayTimezone)
	}

	// Unknown provider/model -> defaults only.
	e2 := cfg.Resolve("openai/gpt-4o-mini")
	if e2.RPM == nil || *e2.RPM != 5 || e2.TPM == nil || *e2.TPM != 100 {
		t.Fatalf("expected default limits, got %+v", e2)
	}
	if e2.HasAnyDayLimit() {
		t.Fatalf("unknown provider should have no day limits")
	}
	if e2.UsageDayTimezone != "" {
		t.Fatalf("tz should be pruned when no day limits, got %q", e2.UsageDayTimezone)
	}
}

func TestResolve_providerTZInheritsFromDefaultsWhenOnlyDefaultsSet(t *testing.T) {
	cfg, err := Parse([]byte(`
defaults:
  usage_day_timezone: America/Los_Angeles
providers:
  google:
    rpd: 250
`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	e := cfg.Resolve("google/gemini-2.0-flash")
	if e.RPD == nil || *e.RPD != 250 {
		t.Fatalf("rpd: %v", e.RPD)
	}
	if e.UsageDayTimezone != "America/Los_Angeles" {
		t.Fatalf("tz from defaults: %q", e.UsageDayTimezone)
	}
}

func TestMinuteKey_isUTC(t *testing.T) {
	loc, _ := time.LoadLocation("America/Los_Angeles")
	at := time.Date(2026, 4, 16, 23, 30, 0, 0, loc) // 06:30 UTC next day
	got := MinuteKey(at)
	want := "2026-04-17T06:30"
	if got != want {
		t.Fatalf("MinuteKey = %q, want %q", got, want)
	}
}

func TestDayKey_vendorLocal(t *testing.T) {
	// 23:30 PT on 2026-04-16 = 06:30 UTC on 2026-04-17.
	loc, _ := time.LoadLocation("America/Los_Angeles")
	at := time.Date(2026, 4, 16, 23, 30, 0, 0, loc)

	utcKey, err := DayKey(at, "UTC")
	if err != nil {
		t.Fatalf("utc: %v", err)
	}
	if utcKey != "2026-04-17" {
		t.Fatalf("utc day: %q", utcKey)
	}

	ptKey, err := DayKey(at, "America/Los_Angeles")
	if err != nil {
		t.Fatalf("pt: %v", err)
	}
	if ptKey != "2026-04-16" {
		t.Fatalf("pt day: %q", ptKey)
	}
}

func TestDayKey_emptyOrBadTZ_error(t *testing.T) {
	if _, err := DayKey(time.Now(), ""); err == nil {
		t.Fatalf("want empty-tz err")
	}
	if _, err := DayKey(time.Now(), "Not/A_Zone"); err == nil {
		t.Fatalf("want bad-tz err")
	}
}

func TestDayWindow_isExclusiveEndInUTC(t *testing.T) {
	// DST spring-forward in America/Los_Angeles on 2026-03-08: local 02:00 -> 03:00.
	// The local day should still be 24 wall-clock hours but only 23 elapsed UTC hours.
	loc, _ := time.LoadLocation("America/Los_Angeles")
	at := time.Date(2026, 3, 8, 10, 0, 0, 0, loc) // mid-day local
	start, end, err := DayWindow(at, "America/Los_Angeles")
	if err != nil {
		t.Fatalf("DayWindow: %v", err)
	}
	wantStart := time.Date(2026, 3, 8, 8, 0, 0, 0, time.UTC) // PST->PDT means 00:00 PST = 08:00 UTC
	if !start.Equal(wantStart) {
		t.Fatalf("start = %s, want %s", start, wantStart)
	}
	// End = next local midnight = 00:00 PDT = 07:00 UTC => 23h after start.
	wantEnd := time.Date(2026, 3, 9, 7, 0, 0, 0, time.UTC)
	if !end.Equal(wantEnd) {
		t.Fatalf("end = %s, want %s", end, wantEnd)
	}
}

func TestSplitProviderModel(t *testing.T) {
	cases := []struct {
		in, p, m string
	}{
		{"groq/llama-3.3-70b", "groq", "groq/llama-3.3-70b"},
		{"openai/gpt-4o", "openai", "openai/gpt-4o"},
		{"noSlash", "", "noSlash"},
		{"", "", ""},
	}
	for _, c := range cases {
		p, m := SplitProviderModel(c.in)
		if p != c.p || m != c.m {
			t.Fatalf("%q -> (%q,%q), want (%q,%q)", c.in, p, m, c.p, c.m)
		}
	}
}
