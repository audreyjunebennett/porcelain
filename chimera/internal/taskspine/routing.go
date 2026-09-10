package taskspine

import (
	"strings"

	"github.com/lynn/porcelain/chimera/internal/modelcatalog"
	"github.com/lynn/porcelain/chimera/internal/providerlimits"
)

type ExecutionLocation string

const (
	LocationLocal ExecutionLocation = "local"
	LocationCloud ExecutionLocation = "cloud"
)

type Candidate struct {
	ModelID        string
	Capabilities   []string
	CatalogPresent bool
	Location       ExecutionLocation
	Limits         providerlimits.Effective
	Usage          providerlimits.Usage
}

type CandidateDecision struct {
	ModelID             string
	Eligible            bool
	EffectiveContextCap int64
	Reasons             []string
}

func (d CandidateDecision) PrimaryReason() string {
	if len(d.Reasons) == 0 {
		return ""
	}
	return d.Reasons[0]
}

type RouteResult struct {
	SelectedModel string
	Decisions     []CandidateDecision
}

func Route(task Task, summary ContextSummary, candidates []Candidate) RouteResult {
	ordered := orderForPrivacy(task.Privacy, candidates)
	result := RouteResult{Decisions: make([]CandidateDecision, 0, len(ordered))}
	request := providerlimits.RequestAdmission{
		EstPromptTokens: summary.TotalTokens,
		MaxTokens:       task.MaxOutputTokens,
		BodyBytes:       summary.BodyBytes,
	}
	for _, candidate := range ordered {
		decision := evaluateCandidate(task.Privacy, request, candidate)
		result.Decisions = append(result.Decisions, decision)
		if result.SelectedModel == "" && decision.Eligible {
			result.SelectedModel = candidate.ModelID
		}
	}
	return result
}

func evaluateCandidate(policy PrivacyPolicy, request providerlimits.RequestAdmission, candidate Candidate) CandidateDecision {
	decision := CandidateDecision{ModelID: strings.TrimSpace(candidate.ModelID), Eligible: true}
	reject := func(reason string) {
		decision.Eligible = false
		decision.Reasons = append(decision.Reasons, reason)
	}

	capability := modelcatalog.Classify(candidate.ModelID, candidate.Capabilities)
	if capability.Status != modelcatalog.StatusIncluded {
		reject("capability: " + capability.Reason)
	}
	if !candidate.CatalogPresent {
		reject("availability: absent from live catalog evidence")
	}
	if candidate.Location != LocationLocal && candidate.Location != LocationCloud {
		reject("location: execution location is unknown")
	}
	if policy == PrivacyLocalOnly && candidate.Location != LocationLocal {
		reject("privacy: cloud execution is disabled")
	}

	contextCap, hasContextCap := candidate.Limits.EffectiveContextCap()
	decision.EffectiveContextCap = contextCap
	if !hasContextCap {
		reject("context: effective context window is unknown")
	} else if contextDecision := providerlimits.DecideContext(candidate.Limits, request); !contextDecision.Allowed {
		reject("context: " + contextDecision.Detail)
	}
	if quotaDecision := providerlimits.Decide(candidate.Limits, candidate.Usage, request.EstPromptTokens); !quotaDecision.Allowed {
		reject("quota: " + quotaDecision.Detail)
	}
	if decision.Eligible {
		decision.Reasons = append(decision.Reasons,
			"eligible: general text capability, live catalog evidence, policy, context, and configured quota checks passed")
	}
	return decision
}

func orderForPrivacy(policy PrivacyPolicy, candidates []Candidate) []Candidate {
	ordered := append([]Candidate(nil), candidates...)
	if policy != PrivacyLocalFirst {
		return ordered
	}
	local := make([]Candidate, 0, len(ordered))
	cloud := make([]Candidate, 0, len(ordered))
	for _, candidate := range ordered {
		if candidate.Location == LocationLocal {
			local = append(local, candidate)
		} else {
			cloud = append(cloud, candidate)
		}
	}
	return append(local, cloud...)
}
