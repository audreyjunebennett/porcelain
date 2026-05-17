package routinggen

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/lynn/porcelain/chimera/internal/providerlimits"
	"gopkg.in/yaml.v3"
)

// ExtractCatalogModelIDs returns model ids from OpenAI-style list JSON, excluding skipID (e.g. virtual Chimera id).
func ExtractCatalogModelIDs(listJSON []byte, skipID string) ([]string, error) {
	var wrap struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(listJSON, &wrap); err != nil {
		return nil, err
	}
	skipID = strings.TrimSpace(skipID)
	var out []string
	seen := make(map[string]struct{})
	for _, d := range wrap.Data {
		id := strings.TrimSpace(d.ID)
		if id == "" || id == skipID {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out, nil
}

// modelScore ranks ids for fallback ordering (higher = try earlier when hosted; ollama penalized).
func modelScore(id string) int {
	lower := strings.ToLower(id)
	s := 0
	if strings.HasPrefix(id, "ollama/") {
		s -= 100000
	}
	switch {
	case strings.Contains(lower, "405b"), strings.Contains(lower, "400b"):
		s += 800
	case strings.Contains(lower, "120b"), strings.Contains(lower, "70b"), strings.Contains(lower, "72b"):
		s += 600
	case strings.Contains(lower, "32b"):
		s += 400
	case strings.Contains(lower, "27b"):
		s += 350
	case strings.Contains(lower, "17b"), strings.Contains(lower, "20b"):
		s += 300
	case strings.Contains(lower, "13b"), strings.Contains(lower, "14b"):
		s += 200
	case strings.Contains(lower, "8b"), strings.Contains(lower, "7b"):
		s += 100
	}
	if strings.Contains(lower, "pro") && !strings.Contains(lower, "lite") {
		s += 80
	}
	if strings.Contains(lower, "flash") && !strings.Contains(lower, "lite") {
		s += 40
	}
	if strings.Contains(lower, "instant") || strings.Contains(lower, "fast") {
		s += 25
	}
	if strings.Contains(lower, "lite") || strings.Contains(lower, "mini") {
		s -= 15
	}
	return s
}

// OrderFallbackChain sorts ids: higher modelScore first; stable tie-break by id.
func OrderFallbackChain(ids []string) []string {
	if len(ids) == 0 {
		return nil
	}
	out := append([]string(nil), ids...)
	sort.SliceStable(out, func(i, j int) bool {
		si, sj := modelScore(out[i]), modelScore(out[j])
		if si != sj {
			return si > sj
		}
		return out[i] < out[j]
	})
	return out
}

const maxRecommendedRouterModels = 8

// OrderRouterModels sorts ids for the tool-router transformer: prefer **small / fast** models (so
// each scoring call is cheap), then **higher RPM** and **higher TPM** from provider limits when
// configured (more headroom for long tool JSON + router system prompt + user text).
// Hosted models are ranked ahead of ollama/ at equal composite score (more predictable JSON).
func OrderRouterModels(ids []string, limits *providerlimits.Config) []string {
	if len(ids) == 0 {
		return nil
	}
	out := append([]string(nil), ids...)
	sort.SliceStable(out, func(i, j int) bool {
		ri, rj := routerRank(out[i], limits), routerRank(out[j], limits)
		if ri != rj {
			return ri > rj
		}
		return out[i] < out[j]
	})
	if len(out) > maxRecommendedRouterModels {
		out = out[:maxRecommendedRouterModels]
	}
	return out
}

func routerRank(id string, limits *providerlimits.Config) float64 {
	ms := modelScore(id)
	// Strong preference for smaller/faster models (lower modelScore → higher rank term).
	sizeTerm := float64(5000 - ms)

	rpmBonus, tpmBonus := 0.0, 0.0
	if limits != nil {
		e := limits.Resolve(id)
		if e.RPM != nil {
			rpmBonus = math.Log1p(float64(*e.RPM)) * 80
		}
		if e.TPM != nil {
			tpmBonus = math.Log1p(float64(*e.TPM)) * 25
		}
	}
	// Slight preference for hosted APIs over local ollama for structured JSON scoring.
	hostedBonus := 0.0
	if strings.HasPrefix(id, "ollama/") {
		hostedBonus = -120
	}
	return sizeTerm + rpmBonus + tpmBonus + hostedBonus
}

// PickLongTurnModel chooses a rule target for long user messages (strongest hosted, else strongest overall).
func PickLongTurnModel(ids []string) string {
	if len(ids) == 0 {
		return ""
	}
	var hosted []string
	for _, id := range ids {
		if !strings.HasPrefix(id, "ollama/") {
			hosted = append(hosted, id)
		}
	}
	pool := hosted
	if len(pool) == 0 {
		pool = append([]string(nil), ids...)
	}
	best := pool[0]
	bestS := modelScore(best)
	for _, id := range pool[1:] {
		if s := modelScore(id); s > bestS {
			best, bestS = id, s
		}
	}
	return best
}

// PickAmbiguousDefault uses the highest-priority entry in the ordered chain (no cross-provider bias).
func PickAmbiguousDefault(chain []string) string {
	if len(chain) == 0 {
		return ""
	}
	return chain[0]
}

// BuildRoutingPolicyYAML returns routing-policy.yaml bytes for the virtual model path.
func BuildRoutingPolicyYAML(chain []string) ([]byte, error) {
	if len(chain) == 0 {
		return nil, fmt.Errorf("empty fallback chain")
	}
	long := PickLongTurnModel(chain)
	if long == "" {
		long = chain[0]
	}
	defaultModel := chain[0]
	doc := map[string]any{
		"ambiguous_default_model": PickAmbiguousDefault(chain),
		"rules": []any{
			map[string]any{
				"name":   "long-user-turn",
				"when":   map[string]any{"min_message_chars": 8000},
				"models": []string{long},
			},
			map[string]any{
				"name":   "default",
				"when":   map[string]any{},
				"models": []string{defaultModel},
			},
		},
	}
	var buf strings.Builder
	buf.WriteString("# Generated routing policy — virtual Chimera model only.\n")
	buf.WriteString("# Regenerated by the operator UI; see config/routing-policy.yaml in the repo for field semantics.\n\n")
	raw, err := yaml.Marshal(doc)
	if err != nil {
		return nil, err
	}
	buf.Write(raw)
	return []byte(buf.String()), nil
}
