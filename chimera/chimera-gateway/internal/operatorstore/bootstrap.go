package operatorstore

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"strings"

	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/routing"
	"github.com/lynn/porcelain/chimera/internal/config"
	"gopkg.in/yaml.v3"
)

var nonSlugRE = regexp.MustCompile(`[^a-z0-9]+`)

// RuleNameToSlug converts a policy rule name to a stable catalog slug.
func RuleNameToSlug(name string) string {
	name = strings.TrimSpace(strings.ToLower(name))
	name = nonSlugRE.ReplaceAllString(name, "_")
	name = strings.Trim(name, "_")
	if name == "" {
		return "routing.rule.unnamed"
	}
	return "routing.rule." + name
}

func seedRoutingRuleCatalog(ctx context.Context, s *Store) error {
	defaultCfg, _ := json.Marshal(map[string]any{
		"when":   map[string]any{},
		"models": []string{},
	})
	longCfg, _ := json.Marshal(map[string]any{
		"when":   map[string]any{"min_message_chars": 8000},
		"models": []string{},
	})
	if err := s.UpsertRoutingRuleDefinition(ctx, "default", "routing.rule.default", string(defaultCfg),
		"Default rule (empty when clause)"); err != nil {
		return err
	}
	return s.UpsertRoutingRuleDefinition(ctx, "long-user-turn", "routing.rule.long_user_turn", string(longCfg),
		"Route long user turns via min_message_chars")
}

func seedRulesFromPolicyYAML(ctx context.Context, s *Store, policyYAML []byte) error {
	var doc struct {
		Rules []struct {
			Name   string   `yaml:"name"`
			When   any      `yaml:"when"`
			Models []string `yaml:"models"`
		} `yaml:"rules"`
	}
	if err := yaml.Unmarshal(policyYAML, &doc); err != nil {
		return err
	}
	for _, r := range doc.Rules {
		if strings.TrimSpace(r.Name) == "" {
			continue
		}
		cfg, err := json.Marshal(map[string]any{"when": r.When, "models": r.Models})
		if err != nil {
			return err
		}
		slug := RuleNameToSlug(r.Name)
		if err := s.UpsertRoutingRuleDefinition(ctx, r.Name, slug, string(cfg), ""); err != nil {
			return err
		}
	}
	return nil
}

// BootstrapVirtualModels imports legacy gateway config into operator SQLite when empty.
func BootstrapVirtualModels(ctx context.Context, s *Store, res *config.Resolved, log *slog.Logger) error {
	if s == nil || res == nil {
		return nil
	}
	has, err := s.HasVirtualModels(ctx)
	if err != nil {
		return fmt.Errorf("bootstrap virtual models count: %w", err)
	}
	if has {
		return nil
	}
	if err := seedRoutingRuleCatalog(ctx, s); err != nil {
		return fmt.Errorf("bootstrap routing rule catalog: %w", err)
	}

	policyYAML := []byte("")
	if p := strings.TrimSpace(res.RoutingPolicyPath); p != "" {
		if b, err := os.ReadFile(p); err == nil {
			policyYAML = b
		}
	}
	if len(policyYAML) > 0 {
		if err := seedRulesFromPolicyYAML(ctx, s, policyYAML); err != nil && log != nil {
			log.Warn("bootstrap: seed rules from policy yaml failed", "err", err)
		}
	}

	policyEnabled := false
	if len(strings.TrimSpace(string(policyYAML))) > 0 {
		policyEnabled = routing.ValidatePolicyYAML(policyYAML) == nil
	}
	chimera := VirtualModel{
		ModelID:              res.VirtualModelID,
		Name:                 "Chimera",
		Version:              res.Semver,
		Description:          "Bootstrap virtual model imported from gateway.yaml and routing-policy.yaml",
		Enabled:              true,
		Visibility:           VisibilityPublic,
		CreatedByPrincipalID: "",
		TenantID:             "",
		FallbackChain:        append([]string(nil), res.FallbackChain...),
		RoutingPolicyYAML:    string(policyYAML),
		RoutingPolicyEnabled: policyEnabled,
		ToolRouterEnabled:    res.ToolRouterEnabled,
		RouterModels:         append([]string(nil), res.RouterModels...),
		ToolRouterConfidence: res.ToolRouterConfidenceThreshold,
	}
	if _, err := s.InsertVirtualModelFull(ctx, chimera); err != nil {
		return fmt.Errorf("bootstrap chimera virtual model: %w", err)
	}
	if log != nil {
		log.Info("bootstrap virtual model imported", "msg", "gateway.virtual_model.bootstrap_imported",
			"model_id", chimera.ModelID, "fallback_depth", len(chimera.FallbackChain))
	}
	return nil
}

// Gemini010Seed returns the Gemini-0.1.0 virtual model definition (gemini provider only).
func Gemini010Seed(geminiModels []string) VirtualModel {
	if len(geminiModels) == 0 {
		geminiModels = []string{
			"gemini/gemini-2.5-flash",
			"gemini/gemini-2.5-flash-lite",
		}
	}
	defaultModel := geminiModels[0]
	policyYAML := fmt.Sprintf(`ambiguous_default_model: %s
rules:
  - name: default
    when: {}
    models:
      - %s
  - name: long-user-turn
    when:
      min_message_chars: 4000
    models:
      - %s
`, defaultModel, defaultModel, defaultModel)
	return VirtualModel{
		ModelID:              "Gemini-0.1.0",
		Name:                 "Gemini",
		Version:              "0.1.0",
		Description:          "Gemini-only virtual model; routes exclusively through gemini provider models",
		Enabled:              true,
		Visibility:           VisibilityPublic,
		FallbackChain:        append([]string(nil), geminiModels...),
		RoutingPolicyYAML:    policyYAML,
		RoutingPolicyEnabled: true,
		ToolRouterEnabled:    false,
		RouterModels:         nil,
		ToolRouterConfidence: 0.5,
	}
}

// EnsureGeminiVirtualModel creates Gemini-0.1.0 when absent (used after bootstrap in dev/tests).
func EnsureGeminiVirtualModel(ctx context.Context, s *Store, geminiModels []string, log *slog.Logger) error {
	if s == nil {
		return nil
	}
	existing, err := s.GetVirtualModelByModelID(ctx, "Gemini-0.1.0")
	if err != nil {
		return err
	}
	if existing != nil {
		return nil
	}
	vm := Gemini010Seed(geminiModels)
	if _, err := s.InsertVirtualModelFull(ctx, vm); err != nil {
		return fmt.Errorf("seed gemini virtual model: %w", err)
	}
	if log != nil {
		log.Info("gemini virtual model seeded", "msg", "gateway.virtual_model.gemini_seeded", "model_id", vm.ModelID)
	}
	return nil
}
