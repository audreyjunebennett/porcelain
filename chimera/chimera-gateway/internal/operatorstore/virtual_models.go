package operatorstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const (
	VisibilityPublic  = "public"
	VisibilityPrivate = "private"
)

// VirtualModel is one operator-managed virtual model with routing attachments.
type VirtualModel struct {
	ID                   int64
	ModelID              string
	Name                 string
	Version              string
	Description          string
	Enabled              bool
	Visibility           string
	CreatedByPrincipalID string
	TenantID             string
	CreatedAt            time.Time
	UpdatedAt            time.Time
	FallbackChain        []string
	RoutingPolicyYAML    string
	RoutingPolicyEnabled bool
	ToolRouterEnabled    bool
	RouterModels         []string
	ToolRouterConfidence float64
}

// RoutingRuleDefinition is a reusable routing rule catalog entry.
type RoutingRuleDefinition struct {
	ID                int64
	Name              string
	Slug              string
	DefaultConfigJSON string
	Description       string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// CreateVirtualModelInput is metadata for a new virtual model (routing filled separately).
type CreateVirtualModelInput struct {
	ModelID              string
	Name                 string
	Version              string
	Description          string
	Visibility           string
	CreatedByPrincipalID string
	TenantID             string
	Enabled              bool
}

func normalizeVisibility(v string) string {
	v = strings.TrimSpace(strings.ToLower(v))
	if v == VisibilityPrivate {
		return VisibilityPrivate
	}
	return VisibilityPublic
}

func (s *Store) countVirtualModels(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM virtual_models`).Scan(&n)
	return n, err
}

// HasVirtualModels reports whether any virtual model rows exist.
func (s *Store) HasVirtualModels(ctx context.Context) (bool, error) {
	n, err := s.countVirtualModels(ctx)
	return n > 0, err
}

// ListVirtualModels returns models visible to principalID within tenantID.
func (s *Store) ListVirtualModels(ctx context.Context, tenantID, principalID string) ([]VirtualModel, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("operator store unavailable")
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, model_id, name, version, description, enabled, visibility,
       created_by_principal_id, tenant_id, created_at, updated_at
FROM virtual_models
WHERE tenant_id = ? OR tenant_id = ''
ORDER BY id`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []VirtualModel
	for rows.Next() {
		vm, err := scanVirtualModelRow(rows)
		if err != nil {
			return nil, err
		}
		if vm.Visibility == VisibilityPrivate && vm.CreatedByPrincipalID != "" &&
			principalID != "" && vm.CreatedByPrincipalID != principalID {
			continue
		}
		out = append(out, vm)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		if err := s.loadVirtualModelRouting(ctx, &out[i]); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// ListEnabledVirtualModels returns enabled models for runtime resolution (all tenants merged).
func (s *Store) ListEnabledVirtualModels(ctx context.Context) ([]VirtualModel, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("operator store unavailable")
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, model_id, name, version, description, enabled, visibility,
       created_by_principal_id, tenant_id, created_at, updated_at
FROM virtual_models
WHERE enabled = 1
ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []VirtualModel
	for rows.Next() {
		vm, err := scanVirtualModelRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, vm)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		if err := s.loadVirtualModelRouting(ctx, &out[i]); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func scanVirtualModelRow(rows *sql.Rows) (VirtualModel, error) {
	var vm VirtualModel
	var desc, vis, creator, tenant, ca, ua string
	var enabled int
	if err := rows.Scan(&vm.ID, &vm.ModelID, &vm.Name, &vm.Version, &desc, &enabled, &vis,
		&creator, &tenant, &ca, &ua); err != nil {
		return VirtualModel{}, err
	}
	vm.Description = desc
	vm.Enabled = enabled != 0
	vm.Visibility = vis
	vm.CreatedByPrincipalID = creator
	vm.TenantID = tenant
	vm.CreatedAt, _ = time.Parse(time.RFC3339Nano, ca)
	vm.UpdatedAt, _ = time.Parse(time.RFC3339Nano, ua)
	return vm, nil
}

func (s *Store) loadVirtualModelRouting(ctx context.Context, vm *VirtualModel) error {
	var chainJSON string
	err := s.db.QueryRowContext(ctx, `
SELECT chain_json FROM virtual_model_fallback WHERE virtual_model_id = ?`, vm.ID).Scan(&chainJSON)
	if err == sql.ErrNoRows {
		vm.FallbackChain = nil
	} else if err != nil {
		return err
	} else if chainJSON != "" {
		_ = json.Unmarshal([]byte(chainJSON), &vm.FallbackChain)
	}

	var polEnabled int
	var polYAML string
	err = s.db.QueryRowContext(ctx, `
SELECT enabled, policy_yaml FROM virtual_model_routing_policy WHERE virtual_model_id = ?`, vm.ID).
		Scan(&polEnabled, &polYAML)
	if err == sql.ErrNoRows {
		vm.RoutingPolicyEnabled = false
		vm.RoutingPolicyYAML = ""
	} else if err != nil {
		return err
	} else {
		vm.RoutingPolicyEnabled = polEnabled != 0
		vm.RoutingPolicyYAML = polYAML
	}

	var trEnabled int
	var routerJSON string
	var threshold float64
	err = s.db.QueryRowContext(ctx, `
SELECT enabled, router_models_json, confidence_threshold
FROM virtual_model_tool_router WHERE virtual_model_id = ?`, vm.ID).
		Scan(&trEnabled, &routerJSON, &threshold)
	if err == sql.ErrNoRows {
		vm.ToolRouterEnabled = false
		vm.RouterModels = nil
		vm.ToolRouterConfidence = 0.5
	} else if err != nil {
		return err
	} else {
		vm.ToolRouterEnabled = trEnabled != 0
		vm.ToolRouterConfidence = threshold
		if routerJSON != "" {
			_ = json.Unmarshal([]byte(routerJSON), &vm.RouterModels)
		}
	}
	return nil
}

// GetVirtualModelByID loads one model by row id and tenant scope.
func (s *Store) GetVirtualModelByID(ctx context.Context, tenantID string, id int64) (*VirtualModel, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("operator store unavailable")
	}
	var vm VirtualModel
	var desc, vis, creator, tid, ca, ua string
	var enabled int
	err := s.db.QueryRowContext(ctx, `
SELECT id, model_id, name, version, description, enabled, visibility,
       created_by_principal_id, tenant_id, created_at, updated_at
FROM virtual_models WHERE id = ? AND (tenant_id = ? OR tenant_id = '')`, id, tenantID).
		Scan(&vm.ID, &vm.ModelID, &vm.Name, &vm.Version, &desc, &enabled, &vis,
			&creator, &tid, &ca, &ua)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	vm.Description = desc
	vm.Enabled = enabled != 0
	vm.Visibility = vis
	vm.CreatedByPrincipalID = creator
	vm.TenantID = tid
	vm.CreatedAt, _ = time.Parse(time.RFC3339Nano, ca)
	vm.UpdatedAt, _ = time.Parse(time.RFC3339Nano, ua)
	if err := s.loadVirtualModelRouting(ctx, &vm); err != nil {
		return nil, err
	}
	return &vm, nil
}

// GetVirtualModelByModelID loads by client-facing model id.
func (s *Store) GetVirtualModelByModelID(ctx context.Context, modelID string) (*VirtualModel, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("operator store unavailable")
	}
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return nil, nil
	}
	var vm VirtualModel
	var desc, vis, creator, tenant, ca, ua string
	var enabled int
	err := s.db.QueryRowContext(ctx, `
SELECT id, model_id, name, version, description, enabled, visibility,
       created_by_principal_id, tenant_id, created_at, updated_at
FROM virtual_models WHERE model_id = ?`, modelID).
		Scan(&vm.ID, &vm.ModelID, &vm.Name, &vm.Version, &desc, &enabled, &vis,
			&creator, &tenant, &ca, &ua)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	vm.Description = desc
	vm.Enabled = enabled != 0
	vm.Visibility = vis
	vm.CreatedByPrincipalID = creator
	vm.TenantID = tenant
	vm.CreatedAt, _ = time.Parse(time.RFC3339Nano, ca)
	vm.UpdatedAt, _ = time.Parse(time.RFC3339Nano, ua)
	if err := s.loadVirtualModelRouting(ctx, &vm); err != nil {
		return nil, err
	}
	return &vm, nil
}

// CreateVirtualModel inserts metadata and empty routing rows.
func (s *Store) CreateVirtualModel(ctx context.Context, in CreateVirtualModelInput) (*VirtualModel, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("operator store unavailable")
	}
	in.ModelID = strings.TrimSpace(in.ModelID)
	in.Name = strings.TrimSpace(in.Name)
	in.Version = strings.TrimSpace(in.Version)
	if in.Name == "" || in.Version == "" {
		return nil, fmt.Errorf("name and version required")
	}
	if in.ModelID == "" {
		in.ModelID = in.Name + "-" + in.Version
	}
	now := s.nowRFC3339()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	enabled := 1
	if !in.Enabled {
		enabled = 0
	}
	res, err := tx.ExecContext(ctx, `
INSERT INTO virtual_models (model_id, name, version, description, enabled, visibility,
	created_by_principal_id, tenant_id, created_at, updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?)`,
		in.ModelID, in.Name, in.Version, strings.TrimSpace(in.Description), enabled,
		normalizeVisibility(in.Visibility), strings.TrimSpace(in.CreatedByPrincipalID),
		strings.TrimSpace(in.TenantID), now, now)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO virtual_model_fallback (virtual_model_id, chain_json, updated_at) VALUES (?,?,?)`,
		id, "[]", now); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO virtual_model_routing_policy (virtual_model_id, enabled, policy_yaml, updated_at) VALUES (?,?,?,?)`,
		id, 0, "", now); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO virtual_model_tool_router (virtual_model_id, enabled, router_models_json, confidence_threshold, updated_at)
VALUES (?,?,?,?,?)`, id, 0, "[]", 0.5, now); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetVirtualModelByID(ctx, in.TenantID, id)
}

// UpdateVirtualModelMetadata updates name, version, description, enabled, visibility.
func (s *Store) UpdateVirtualModelMetadata(ctx context.Context, tenantID string, id int64, name, version, description string, enabled *bool, visibility *string) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("operator store unavailable")
	}
	w, err := s.GetVirtualModelByID(ctx, tenantID, id)
	if err != nil {
		return err
	}
	if w == nil {
		return fmt.Errorf("virtual model not found")
	}
	if strings.TrimSpace(name) != "" {
		w.Name = strings.TrimSpace(name)
	}
	if strings.TrimSpace(version) != "" {
		w.Version = strings.TrimSpace(version)
	}
	w.Description = strings.TrimSpace(description)
	en := w.Enabled
	if enabled != nil {
		en = *enabled
	}
	vis := w.Visibility
	if visibility != nil {
		vis = normalizeVisibility(*visibility)
	}
	now := s.nowRFC3339()
	res, err := s.db.ExecContext(ctx, `
UPDATE virtual_models SET name = ?, version = ?, description = ?, enabled = ?, visibility = ?, updated_at = ?
WHERE id = ? AND (tenant_id = ? OR tenant_id = '')`,
		w.Name, w.Version, w.Description, boolToInt(en), vis, now, id, tenantID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("virtual model not found")
	}
	return nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// DeleteVirtualModel removes a virtual model and cascaded routing rows.
func (s *Store) DeleteVirtualModel(ctx context.Context, tenantID string, id int64) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("operator store unavailable")
	}
	res, err := s.db.ExecContext(ctx, `
DELETE FROM virtual_models WHERE id = ? AND (tenant_id = ? OR tenant_id = '')`, id, tenantID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("virtual model not found")
	}
	return nil
}

// SetVirtualModelFallback saves the ordered fallback chain.
func (s *Store) SetVirtualModelFallback(ctx context.Context, tenantID string, id int64, chain []string) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("operator store unavailable")
	}
	if len(chain) == 0 {
		return fmt.Errorf("fallback chain must be non-empty")
	}
	for _, m := range chain {
		if strings.TrimSpace(m) == "" {
			return fmt.Errorf("fallback chain contains empty model id")
		}
	}
	w, err := s.GetVirtualModelByID(ctx, tenantID, id)
	if err != nil {
		return err
	}
	if w == nil {
		return fmt.Errorf("virtual model not found")
	}
	b, err := json.Marshal(chain)
	if err != nil {
		return err
	}
	now := s.nowRFC3339()
	_, err = s.db.ExecContext(ctx, `
INSERT INTO virtual_model_fallback (virtual_model_id, chain_json, updated_at) VALUES (?,?,?)
ON CONFLICT(virtual_model_id) DO UPDATE SET chain_json = excluded.chain_json, updated_at = excluded.updated_at`,
		id, string(b), now)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `UPDATE virtual_models SET updated_at = ? WHERE id = ?`, now, id)
	return err
}

// SetVirtualModelRoutingPolicy saves policy YAML and enabled toggle.
func (s *Store) SetVirtualModelRoutingPolicy(ctx context.Context, tenantID string, id int64, enabled bool, policyYAML string) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("operator store unavailable")
	}
	w, err := s.GetVirtualModelByID(ctx, tenantID, id)
	if err != nil {
		return err
	}
	if w == nil {
		return fmt.Errorf("virtual model not found")
	}
	now := s.nowRFC3339()
	_, err = s.db.ExecContext(ctx, `
INSERT INTO virtual_model_routing_policy (virtual_model_id, enabled, policy_yaml, updated_at) VALUES (?,?,?,?)
ON CONFLICT(virtual_model_id) DO UPDATE SET enabled = excluded.enabled, policy_yaml = excluded.policy_yaml, updated_at = excluded.updated_at`,
		id, boolToInt(enabled), policyYAML, now)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `UPDATE virtual_models SET updated_at = ? WHERE id = ?`, now, id)
	return err
}

// SetVirtualModelToolRouter saves tool-router settings for a virtual model.
func (s *Store) SetVirtualModelToolRouter(ctx context.Context, tenantID string, id int64, enabled bool, routerModels []string, threshold float64) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("operator store unavailable")
	}
	w, err := s.GetVirtualModelByID(ctx, tenantID, id)
	if err != nil {
		return err
	}
	if w == nil {
		return fmt.Errorf("virtual model not found")
	}
	if routerModels == nil {
		routerModels = []string{}
	}
	b, err := json.Marshal(routerModels)
	if err != nil {
		return err
	}
	now := s.nowRFC3339()
	_, err = s.db.ExecContext(ctx, `
INSERT INTO virtual_model_tool_router (virtual_model_id, enabled, router_models_json, confidence_threshold, updated_at)
VALUES (?,?,?,?,?)
ON CONFLICT(virtual_model_id) DO UPDATE SET enabled = excluded.enabled,
	router_models_json = excluded.router_models_json, confidence_threshold = excluded.confidence_threshold,
	updated_at = excluded.updated_at`,
		id, boolToInt(enabled), string(b), threshold, now)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `UPDATE virtual_models SET updated_at = ? WHERE id = ?`, now, id)
	return err
}

// UpsertRoutingRuleDefinition inserts or updates a catalog rule by slug.
func (s *Store) UpsertRoutingRuleDefinition(ctx context.Context, name, slug, defaultConfigJSON, description string) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("operator store unavailable")
	}
	name = strings.TrimSpace(name)
	slug = strings.TrimSpace(slug)
	if name == "" || slug == "" {
		return fmt.Errorf("name and slug required")
	}
	if defaultConfigJSON == "" {
		defaultConfigJSON = "{}"
	}
	now := s.nowRFC3339()
	_, err := s.db.ExecContext(ctx, `
INSERT INTO routing_rule_definitions (name, slug, default_config_json, description, created_at, updated_at)
VALUES (?,?,?,?,?,?)
ON CONFLICT(slug) DO UPDATE SET name = excluded.name, default_config_json = excluded.default_config_json,
	description = excluded.description, updated_at = excluded.updated_at`,
		name, slug, defaultConfigJSON, strings.TrimSpace(description), now, now)
	return err
}

// ListRoutingRuleDefinitions returns all catalog entries.
func (s *Store) ListRoutingRuleDefinitions(ctx context.Context) ([]RoutingRuleDefinition, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("operator store unavailable")
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, name, slug, default_config_json, description, created_at, updated_at
FROM routing_rule_definitions ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RoutingRuleDefinition
	for rows.Next() {
		var d RoutingRuleDefinition
		var ca, ua string
		if err := rows.Scan(&d.ID, &d.Name, &d.Slug, &d.DefaultConfigJSON, &d.Description, &ca, &ua); err != nil {
			return nil, err
		}
		d.CreatedAt, _ = time.Parse(time.RFC3339Nano, ca)
		d.UpdatedAt, _ = time.Parse(time.RFC3339Nano, ua)
		out = append(out, d)
	}
	return out, rows.Err()
}

// InsertVirtualModelFull inserts a complete virtual model in one transaction (bootstrap/tests).
func (s *Store) InsertVirtualModelFull(ctx context.Context, vm VirtualModel) (*VirtualModel, error) {
	in := CreateVirtualModelInput{
		ModelID:              vm.ModelID,
		Name:                 vm.Name,
		Version:              vm.Version,
		Description:          vm.Description,
		Visibility:           vm.Visibility,
		CreatedByPrincipalID: vm.CreatedByPrincipalID,
		TenantID:             vm.TenantID,
		Enabled:              vm.Enabled,
	}
	created, err := s.CreateVirtualModel(ctx, in)
	if err != nil {
		return nil, err
	}
	if len(vm.FallbackChain) > 0 {
		if err := s.SetVirtualModelFallback(ctx, vm.TenantID, created.ID, vm.FallbackChain); err != nil {
			return nil, err
		}
	}
	if vm.RoutingPolicyYAML != "" || vm.RoutingPolicyEnabled {
		if err := s.SetVirtualModelRoutingPolicy(ctx, vm.TenantID, created.ID, vm.RoutingPolicyEnabled, vm.RoutingPolicyYAML); err != nil {
			return nil, err
		}
	}
	if vm.ToolRouterEnabled || len(vm.RouterModels) > 0 {
		th := vm.ToolRouterConfidence
		if th <= 0 {
			th = 0.5
		}
		if err := s.SetVirtualModelToolRouter(ctx, vm.TenantID, created.ID, vm.ToolRouterEnabled, vm.RouterModels, th); err != nil {
			return nil, err
		}
	}
	return s.GetVirtualModelByID(ctx, vm.TenantID, created.ID)
}
