package routing

import (
	"encoding/json"
	"log/slog"
	"sync"
)

// InMemoryPolicy holds a compiled routing policy document (no disk reload).
type InMemoryPolicy struct {
	mu               sync.Mutex
	ambiguousDefault string
	rules            []policyRule
}

// NewInMemoryPolicy compiles policy YAML bytes; empty YAML yields chain-only routing.
func NewInMemoryPolicy(raw []byte) (*InMemoryPolicy, error) {
	p := &InMemoryPolicy{}
	if len(raw) == 0 {
		return p, nil
	}
	if err := ValidatePolicyYAML(raw); err != nil {
		return nil, err
	}
	ambiguous, rules, err := parsePolicyDocument(raw)
	if err != nil {
		return nil, err
	}
	p.ambiguousDefault = ambiguous
	p.rules = rules
	return p, nil
}

// PickInitialModel mirrors Policy.PickInitialModel without disk reads.
func (p *InMemoryPolicy) PickInitialModel(body map[string]json.RawMessage, fallbackChain []string, virtualModelID string, log *slog.Logger) (model string, via Via) {
	if p == nil {
		first := ""
		if len(fallbackChain) > 0 {
			first = fallbackChain[0]
		}
		return first, ViaChainOnly
	}
	var clientModel string
	if m, ok := body["model"]; ok {
		_ = json.Unmarshal(m, &clientModel)
	}
	if clientModel != virtualModelID {
		return clientModel, ViaChainOnly
	}
	lastUser := lastUserMessageCharCount(body)
	p.mu.Lock()
	defer p.mu.Unlock()
	return pickFromRules(p.ambiguousDefault, p.rules, lastUser, fallbackChain, log)
}
