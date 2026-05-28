package operatorstore

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/lynn/porcelain/chimera/internal/config"
	"github.com/lynn/porcelain/chimera/internal/providerfreetier"
)

const bootstrapProviderGroq = "groq"
const bootstrapProviderGemini = "gemini"
const bootstrapProviderOllama = "ollama"

// AvailabilityFromFreeTierSpec computes per-model availability for Groq/Gemini from the free-tier spec.
// Ollama is a no-op: every model is available and the returned map is empty (no explicit rows needed).
func AvailabilityFromFreeTierSpec(spec *providerfreetier.Spec, providerID string, modelIDs []string) map[string]bool {
	providerID = strings.ToLower(strings.TrimSpace(providerID))
	if providerID == bootstrapProviderOllama {
		return nil
	}
	if providerID != bootstrapProviderGroq && providerID != bootstrapProviderGemini {
		return nil
	}
	if spec == nil {
		out := make(map[string]bool, len(modelIDs))
		for _, id := range modelIDs {
			id = strings.TrimSpace(id)
			if id == "" || ProviderIDFromModelID(id) != providerID {
				continue
			}
			out[id] = false
		}
		return out
	}
	out := make(map[string]bool, len(modelIDs))
	for _, id := range modelIDs {
		id = strings.TrimSpace(id)
		if id == "" || ProviderIDFromModelID(id) != providerID {
			continue
		}
		out[id] = spec.Match(id)
	}
	return out
}

// BootstrapProviderModelAvailability seeds tenant availability from provider-free-tier.yaml and the
// live broker catalog. Idempotent: skips when any availability row already exists. Ollama models are
// left implicit (all available). Groq/Gemini non-free-tier models are stored as unavailable.
func BootstrapProviderModelAvailability(ctx context.Context, s *Store, res *config.Resolved, catalogModelIDs []string, log *slog.Logger) error {
	if s == nil || res == nil {
		return nil
	}
	has, err := s.HasProviderModelAvailabilityRows(ctx)
	if err != nil {
		return fmt.Errorf("bootstrap provider model availability count: %w", err)
	}
	if has {
		return nil
	}
	if len(catalogModelIDs) == 0 {
		return nil
	}

	var spec *providerfreetier.Spec
	ftPath := strings.TrimSpace(res.ProviderFreeTierPath)
	if ftPath != "" {
		loaded, loadErr := providerfreetier.Load(ftPath)
		if loadErr != nil {
			if log != nil {
				log.Warn("bootstrap: provider-free-tier.yaml unavailable; marking non-matching groq/gemini unavailable",
					"msg", "gateway.provider_models.bootstrap_free_tier_missing", "path", ftPath, "err", loadErr)
			}
		} else {
			spec = loaded
		}
	}

	tenants, err := s.ListDistinctTenantIDs(ctx)
	if err != nil {
		return fmt.Errorf("bootstrap provider model availability tenants: %w", err)
	}
	if len(tenants) == 0 {
		tenants = []string{""}
	}

	byProvider := make(map[string][]string)
	for _, id := range catalogModelIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		prov := ProviderIDFromModelID(id)
		if prov == bootstrapProviderOllama {
			continue
		}
		if prov != bootstrapProviderGroq && prov != bootstrapProviderGemini {
			continue
		}
		byProvider[prov] = append(byProvider[prov], id)
	}

	now := s.nowRFC3339()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	wrote := 0
	for _, tenantID := range tenants {
		for providerID, ids := range byProvider {
			availMap := AvailabilityFromFreeTierSpec(spec, providerID, ids)
			for modelID, available := range availMap {
				if available {
					continue
				}
				if _, err := tx.ExecContext(ctx, `
INSERT INTO provider_model_availability (tenant_id, provider_id, model_id, available, metadata_json, updated_at)
VALUES (?,?,?,?,?,?)`, tenantID, providerID, modelID, 0, "{}", now); err != nil {
					return fmt.Errorf("bootstrap insert %s: %w", modelID, err)
				}
				wrote++
			}
			if len(availMap) > 0 {
				if _, err := tx.ExecContext(ctx, `
INSERT INTO provider_model_config (tenant_id, provider_id, updated_at, metadata_json)
VALUES (?,?,?,?)
ON CONFLICT(tenant_id, provider_id) DO UPDATE SET updated_at = excluded.updated_at`,
					tenantID, providerID, now, `{"source":"bootstrap"}`); err != nil {
					return fmt.Errorf("bootstrap provider config %s: %w", providerID, err)
				}
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if log != nil && wrote > 0 {
		log.Info("bootstrap provider model availability seeded", "msg", "gateway.provider_models.bootstrap_seeded",
			"tenant_count", len(tenants), "unavailable_rows", wrote, "catalog_models", len(catalogModelIDs))
	}
	return nil
}
