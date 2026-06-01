package operatorstore

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"github.com/lynn/porcelain/chimera/internal/config"
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

// BootstrapVirtualModels seeds operator SQLite on first open. Legacy gateway.yaml routing
// import was removed; operators create virtual models in settings (or tests seed rows explicitly).
func BootstrapVirtualModels(ctx context.Context, s *Store, res *config.Resolved, log *slog.Logger) error {
	if s == nil {
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
	return nil
}

// ChimeraSeed returns a Chimera-<semver> virtual model definition for tests and explicit seeding.
func ChimeraSeed(semver string, fallbackChain []string, policyDefaultModel string) VirtualModel {
	if semver == "" {
		semver = "0.1.0"
	}
	if len(fallbackChain) == 0 {
		fallbackChain = []string{"groq/a", "groq/b"}
	}
	if policyDefaultModel == "" {
		policyDefaultModel = fallbackChain[0]
	}
	policyYAML := fmt.Sprintf(`ambiguous_default_model: %s
rules:
  - name: default
    when: {}
    models:
      - %s
`, policyDefaultModel, policyDefaultModel)
	return VirtualModel{
		ModelID:              "Chimera-" + semver,
		Name:                 "Chimera",
		Version:              semver,
		Description:          "Test Chimera virtual model",
		Enabled:              true,
		Visibility:           VisibilityPublic,
		FallbackChain:        append([]string(nil), fallbackChain...),
		RoutingPolicyYAML:    policyYAML,
		RoutingPolicyEnabled: true,
		ToolRouterEnabled:    false,
		RouterModels:         nil,
		ToolRouterConfidence: 0.5,
	}
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
