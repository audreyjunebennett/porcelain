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

func TestDecideContext_noLimits_allows(t *testing.T) {
	d := DecideContext(Effective{}, RequestAdmission{EstPromptTokens: 100_000, MaxTokens: 4096, BodyBytes: 5_000_000})
	if !d.Allowed {
		t.Fatalf("expected allow with no context limits: %+v", d)
	}
}

func TestDecideContext_deniesWhenPromptPlusMaxTokensExceedCap(t *testing.T) {
	window := int64(1000)
	factor := 1.0
	eff := Effective{ContextWindow: &window, ContextSafetyFactor: &factor}
	d := DecideContext(eff, RequestAdmission{EstPromptTokens: 900, MaxTokens: 200})
	if d.Allowed || d.Reason != ReasonContext {
		t.Fatalf("want context deny, got %+v", d)
	}
	d2 := DecideContext(eff, RequestAdmission{EstPromptTokens: 800, MaxTokens: 200})
	if !d2.Allowed {
		t.Fatalf("exact fit should allow: %+v", d2)
	}
}

func TestDecideContext_maxPromptTokensTighterThanContextWindow(t *testing.T) {
	window := int64(131072)
	promptCap := int64(8192)
	factor := 1.0
	eff := Effective{ContextWindow: &window, MaxPromptTokens: &promptCap, ContextSafetyFactor: &factor}
	d := DecideContext(eff, RequestAdmission{EstPromptTokens: 7500, MaxTokens: 500})
	if !d.Allowed {
		t.Fatalf("under max_prompt_tokens should allow: %+v", d)
	}
	d2 := DecideContext(eff, RequestAdmission{EstPromptTokens: 8000, MaxTokens: 1000})
	if d2.Allowed || d2.Reason != ReasonContext {
		t.Fatalf("max_prompt_tokens should win over context_window: %+v", d2)
	}
}

func TestDecideContext_appliesSafetyFactor(t *testing.T) {
	window := int64(1000)
	factor := 0.9
	eff := Effective{ContextWindow: &window, ContextSafetyFactor: &factor}
	d := DecideContext(eff, RequestAdmission{EstPromptTokens: 900})
	if !d.Allowed {
		t.Fatalf("900 <= floor(1000*0.9)=900 should allow: %+v", d)
	}
	d2 := DecideContext(eff, RequestAdmission{EstPromptTokens: 901})
	if d2.Allowed || d2.Reason != ReasonContext {
		t.Fatalf("901 > 900 effective cap should deny: %+v", d2)
	}
}

func TestDecideContext_deniesBodyBytes(t *testing.T) {
	maxBody := int64(1000)
	eff := Effective{MaxBodyBytes: &maxBody}
	d := DecideContext(eff, RequestAdmission{BodyBytes: 1001})
	if d.Allowed || d.Reason != ReasonBodySize {
		t.Fatalf("want body deny, got %+v", d)
	}
	d2 := DecideContext(eff, RequestAdmission{BodyBytes: 1000})
	if !d2.Allowed {
		t.Fatalf("at cap should allow: %+v", d2)
	}
}
