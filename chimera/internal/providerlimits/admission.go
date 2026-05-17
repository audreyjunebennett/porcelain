package providerlimits

import "fmt"

// Usage is the observed usage for a provider/model over the relevant windows. Callers compute
// these from the metrics store (gatewaymetrics). Counts are independent of HTTP status today —
// limits apply to all billable attempts; see plan §3.7.5.
type Usage struct {
	MinuteCalls     int64
	MinuteEstTokens int64
	DayCalls        int64
	DayEstTokens    int64
}

// Reason identifies the dimension that caused a deny (empty when Allowed).
type Reason string

const (
	ReasonNone Reason = ""
	ReasonRPM  Reason = "rpm"
	ReasonRPD  Reason = "rpd"
	ReasonTPM  Reason = "tpm"
	ReasonTPD  Reason = "tpd"
)

// Decision is the result of Decide.
type Decision struct {
	Allowed bool
	Reason  Reason
	// Detail is a human-readable message safe to log; never contains prompt content.
	Detail string
}

// Decide returns Allowed=true when sending one more request of estForThisRequest tokens to the
// resolved provider/model would still stay under every configured limit. If any configured
// ceiling would be exceeded the decision is not allowed and Reason identifies the dimension.
//
// Unset dimensions (nil limit) are never enforced. Pure function — no I/O.
func Decide(eff Effective, usage Usage, estForThisRequest int64) Decision {
	if eff.RPM != nil && usage.MinuteCalls+1 > *eff.RPM {
		return deny(ReasonRPM, fmt.Sprintf("RPM %d would be exceeded (used=%d)", *eff.RPM, usage.MinuteCalls))
	}
	if eff.TPM != nil && usage.MinuteEstTokens+estForThisRequest > *eff.TPM {
		return deny(ReasonTPM, fmt.Sprintf("TPM %d would be exceeded (used=%d, req=%d)", *eff.TPM, usage.MinuteEstTokens, estForThisRequest))
	}
	if eff.RPD != nil && usage.DayCalls+1 > *eff.RPD {
		return deny(ReasonRPD, fmt.Sprintf("RPD %d would be exceeded (used=%d)", *eff.RPD, usage.DayCalls))
	}
	if eff.TPD != nil && usage.DayEstTokens+estForThisRequest > *eff.TPD {
		return deny(ReasonTPD, fmt.Sprintf("TPD %d would be exceeded (used=%d, req=%d)", *eff.TPD, usage.DayEstTokens, estForThisRequest))
	}
	return Decision{Allowed: true}
}

func deny(r Reason, detail string) Decision {
	return Decision{Allowed: false, Reason: r, Detail: detail}
}
