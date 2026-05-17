package providerlimits

import "testing"

func eff(rpm, rpd, tpm, tpd *int64) Effective {
	return Effective{Provider: "p", ModelID: "p/m", RPM: rpm, RPD: rpd, TPM: tpm, TPD: tpd, UsageDayTimezone: "UTC"}
}

func TestDecide_allLimitsNil_allows(t *testing.T) {
	d := Decide(eff(nil, nil, nil, nil), Usage{MinuteCalls: 9e9}, 9e9)
	if !d.Allowed {
		t.Fatalf("expected allow with no configured limits: %+v", d)
	}
}

func TestDecide_deniesRPMWhenOneMoreWouldExceed(t *testing.T) {
	rpm := int64(10)
	d := Decide(eff(&rpm, nil, nil, nil), Usage{MinuteCalls: 10}, 0)
	if d.Allowed || d.Reason != ReasonRPM {
		t.Fatalf("want RPM deny, got %+v", d)
	}
	// At 9 used -> +1 = 10 == limit, allowed.
	d2 := Decide(eff(&rpm, nil, nil, nil), Usage{MinuteCalls: 9}, 0)
	if !d2.Allowed {
		t.Fatalf("should allow at limit-1: %+v", d2)
	}
}

func TestDecide_deniesTPMBasedOnEstimatedRequest(t *testing.T) {
	tpm := int64(1000)
	d := Decide(eff(nil, nil, &tpm, nil), Usage{MinuteEstTokens: 900}, 200)
	if d.Allowed || d.Reason != ReasonTPM {
		t.Fatalf("want TPM deny, got %+v", d)
	}
	d2 := Decide(eff(nil, nil, &tpm, nil), Usage{MinuteEstTokens: 900}, 100)
	if !d2.Allowed {
		t.Fatalf("exact fit should allow: %+v", d2)
	}
}

func TestDecide_priorityMinuteBeforeDay(t *testing.T) {
	rpm := int64(1)
	rpd := int64(1)
	// Both would deny, but RPM should surface first (minute checked before day).
	d := Decide(eff(&rpm, &rpd, nil, nil), Usage{MinuteCalls: 1, DayCalls: 1}, 0)
	if d.Allowed || d.Reason != ReasonRPM {
		t.Fatalf("want RPM first, got %+v", d)
	}
}

func TestDecide_dayTokensDeny(t *testing.T) {
	tpd := int64(10_000)
	d := Decide(eff(nil, nil, nil, &tpd), Usage{DayEstTokens: 9_999}, 2)
	if d.Allowed || d.Reason != ReasonTPD {
		t.Fatalf("want TPD deny, got %+v", d)
	}
}
