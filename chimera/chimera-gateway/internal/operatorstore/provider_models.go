package operatorstore

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// ProviderModelAvailabilityRow is one stored availability override for a tenant + provider + model.
type ProviderModelAvailabilityRow struct {
	ModelID      string
	Available    bool
	MetadataJSON string
	UpdatedAt    time.Time
}

// ProviderModelAvailabilityEntry is a merged broker model id with resolved availability.
type ProviderModelAvailabilityEntry struct {
	ModelID   string
	Available bool
	Explicit  bool // true when a row exists in provider_model_availability
}

// ProviderIDFromModelID returns the provider prefix from a unified catalog id (provider/model).
func ProviderIDFromModelID(modelID string) string {
	modelID = strings.TrimSpace(modelID)
	if slash := strings.Index(modelID, "/"); slash > 0 {
		return strings.ToLower(modelID[:slash])
	}
	return ""
}

// HasProviderModelAvailabilityRows reports whether any availability rows exist (bootstrap gate).
func (s *Store) HasProviderModelAvailabilityRows(ctx context.Context) (bool, error) {
	if s == nil || s.db == nil {
		return false, fmt.Errorf("operator store unavailable")
	}
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM provider_model_availability`).Scan(&n)
	return n > 0, err
}

// ListDistinctTenantIDs returns tenant ids referenced by workspaces, virtual models, or availability rows.
func (s *Store) ListDistinctTenantIDs(ctx context.Context) ([]string, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("operator store unavailable")
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT DISTINCT tenant_id FROM (
	SELECT tenant_id FROM workspaces
	UNION
	SELECT tenant_id FROM virtual_models
	UNION
	SELECT tenant_id FROM provider_model_availability
)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	seen := make(map[string]struct{})
	var out []string
	for rows.Next() {
		var tid string
		if err := rows.Scan(&tid); err != nil {
			return nil, err
		}
		if _, ok := seen[tid]; ok {
			continue
		}
		seen[tid] = struct{}{}
		out = append(out, tid)
	}
	return out, rows.Err()
}

// ListProviderModelAvailability returns stored rows for one tenant + provider.
func (s *Store) ListProviderModelAvailability(ctx context.Context, tenantID, providerID string) ([]ProviderModelAvailabilityRow, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("operator store unavailable")
	}
	providerID = strings.ToLower(strings.TrimSpace(providerID))
	rows, err := s.db.QueryContext(ctx, `
SELECT model_id, available, metadata_json, updated_at
FROM provider_model_availability
WHERE tenant_id = ? AND provider_id = ?
ORDER BY model_id`, tenantID, providerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ProviderModelAvailabilityRow
	for rows.Next() {
		var row ProviderModelAvailabilityRow
		var avail int
		var ua string
		if err := rows.Scan(&row.ModelID, &avail, &row.MetadataJSON, &ua); err != nil {
			return nil, err
		}
		row.Available = avail != 0
		row.UpdatedAt, _ = time.Parse(time.RFC3339Nano, ua)
		out = append(out, row)
	}
	return out, rows.Err()
}

// MergeProviderModelAvailability merges broker model ids with stored overrides (absent row => available).
func MergeProviderModelAvailability(brokerModelIDs []string, stored []ProviderModelAvailabilityRow) []ProviderModelAvailabilityEntry {
	overrides := make(map[string]bool, len(stored))
	for _, row := range stored {
		overrides[row.ModelID] = row.Available
	}
	seen := make(map[string]struct{}, len(brokerModelIDs))
	out := make([]ProviderModelAvailabilityEntry, 0, len(brokerModelIDs))
	for _, id := range brokerModelIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		key := strings.ToLower(id)
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		if avail, ok := overrides[id]; ok {
			out = append(out, ProviderModelAvailabilityEntry{ModelID: id, Available: avail, Explicit: true})
			continue
		}
		out = append(out, ProviderModelAvailabilityEntry{ModelID: id, Available: true, Explicit: false})
	}
	return out
}

// ReplaceProviderModelAvailability replaces all rows for tenant+provider and upserts config metadata.
func (s *Store) ReplaceProviderModelAvailability(ctx context.Context, tenantID, providerID string, models map[string]bool) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("operator store unavailable")
	}
	providerID = strings.ToLower(strings.TrimSpace(providerID))
	if providerID == "" {
		return fmt.Errorf("provider_id required")
	}
	now := s.nowRFC3339()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `
DELETE FROM provider_model_availability WHERE tenant_id = ? AND provider_id = ?`, tenantID, providerID); err != nil {
		return err
	}
	for modelID, available := range models {
		modelID = strings.TrimSpace(modelID)
		if modelID == "" {
			continue
		}
		avail := 0
		if available {
			avail = 1
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO provider_model_availability (tenant_id, provider_id, model_id, available, metadata_json, updated_at)
VALUES (?,?,?,?,?,?)`, tenantID, providerID, modelID, avail, "{}", now); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO provider_model_config (tenant_id, provider_id, updated_at, metadata_json)
VALUES (?,?,?,?)
ON CONFLICT(tenant_id, provider_id) DO UPDATE SET updated_at = excluded.updated_at`, tenantID, providerID, now, "{}"); err != nil {
		return err
	}
	return tx.Commit()
}

// ClearProviderModelAvailability removes all availability rows and config for tenant+provider.
func (s *Store) ClearProviderModelAvailability(ctx context.Context, tenantID, providerID string) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("operator store unavailable")
	}
	providerID = strings.ToLower(strings.TrimSpace(providerID))
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `
DELETE FROM provider_model_availability WHERE tenant_id = ? AND provider_id = ?`, tenantID, providerID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
DELETE FROM provider_model_config WHERE tenant_id = ? AND provider_id = ?`, tenantID, providerID); err != nil {
		return err
	}
	return tx.Commit()
}

// UpsertProviderModelAvailability writes one model row (bootstrap and internal use).
func (s *Store) UpsertProviderModelAvailability(ctx context.Context, tenantID, providerID, modelID string, available bool, metadataJSON string) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("operator store unavailable")
	}
	providerID = strings.ToLower(strings.TrimSpace(providerID))
	modelID = strings.TrimSpace(modelID)
	if providerID == "" || modelID == "" {
		return fmt.Errorf("provider_id and model_id required")
	}
	if strings.TrimSpace(metadataJSON) == "" {
		metadataJSON = "{}"
	}
	avail := 0
	if available {
		avail = 1
	}
	now := s.nowRFC3339()
	_, err := s.db.ExecContext(ctx, `
INSERT INTO provider_model_availability (tenant_id, provider_id, model_id, available, metadata_json, updated_at)
VALUES (?,?,?,?,?,?)
ON CONFLICT(tenant_id, provider_id, model_id) DO UPDATE SET
	available = excluded.available,
	metadata_json = excluded.metadata_json,
	updated_at = excluded.updated_at`, tenantID, providerID, modelID, avail, metadataJSON, now)
	return err
}

// ProviderModelConfigConfigured reports whether operator saved config for tenant+provider.
func (s *Store) ProviderModelConfigConfigured(ctx context.Context, tenantID, providerID string) (bool, error) {
	if s == nil || s.db == nil {
		return false, fmt.Errorf("operator store unavailable")
	}
	var n int
	err := s.db.QueryRowContext(ctx, `
SELECT COUNT(*) FROM provider_model_config WHERE tenant_id = ? AND provider_id = ?`,
		tenantID, strings.ToLower(strings.TrimSpace(providerID))).Scan(&n)
	return n > 0, err
}

// LoadProviderModelAvailabilityIndex loads explicit unavailable model ids grouped by tenant.
func (s *Store) LoadProviderModelAvailabilityIndex(ctx context.Context) (map[string]map[string]struct{}, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("operator store unavailable")
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT tenant_id, model_id FROM provider_model_availability WHERE available = 0`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]map[string]struct{})
	for rows.Next() {
		var tenantID, modelID string
		if err := rows.Scan(&tenantID, &modelID); err != nil {
			return nil, err
		}
		modelID = strings.TrimSpace(modelID)
		if modelID == "" {
			continue
		}
		if out[tenantID] == nil {
			out[tenantID] = make(map[string]struct{})
		}
		out[tenantID][modelID] = struct{}{}
	}
	return out, rows.Err()
}

// CountProviderModelAvailability returns available/unavailable counts for tenant+provider.
func (s *Store) CountProviderModelAvailability(ctx context.Context, tenantID, providerID string) (available, unavailable int, err error) {
	if s == nil || s.db == nil {
		return 0, 0, fmt.Errorf("operator store unavailable")
	}
	providerID = strings.ToLower(strings.TrimSpace(providerID))
	err = s.db.QueryRowContext(ctx, `
SELECT
	COALESCE(SUM(CASE WHEN available = 1 THEN 1 ELSE 0 END), 0),
	COALESCE(SUM(CASE WHEN available = 0 THEN 1 ELSE 0 END), 0)
FROM provider_model_availability
WHERE tenant_id = ? AND provider_id = ?`, tenantID, providerID).Scan(&available, &unavailable)
	return available, unavailable, err
}

// IsModelAvailableForTenant reports availability using stored overrides (absent row => available).
func (s *Store) IsModelAvailableForTenant(ctx context.Context, tenantID, modelID string) (bool, error) {
	if s == nil || s.db == nil {
		return true, fmt.Errorf("operator store unavailable")
	}
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return true, nil
	}
	var avail sql.NullInt64
	err := s.db.QueryRowContext(ctx, `
SELECT available FROM provider_model_availability
WHERE tenant_id = ? AND model_id = ?`, tenantID, modelID).Scan(&avail)
	if err == sql.ErrNoRows {
		return true, nil
	}
	if err != nil {
		return true, err
	}
	return avail.Valid && avail.Int64 != 0, nil
}
