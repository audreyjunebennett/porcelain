package operatorstore

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// Workspace is one logical indexer workspace (project + flavor) with watched paths.
type Workspace struct {
	ID        int64
	TenantID  string
	ProjectID string
	FlavorID  string
	CreatedAt time.Time
	UpdatedAt time.Time
	Paths     []WorkspacePath
}

// WorkspacePath is one watched directory belonging to a workspace.
type WorkspacePath struct {
	ID             int64
	WorkspaceRowID int64
	Path           string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// Store is SQLite-backed operator data (workspaces). Safe for concurrent HTTP handlers.
type Store struct {
	db *sql.DB
}

func sqliteDSN(absPath string) string {
	p := filepath.ToSlash(absPath)
	return "file:" + p + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
}

// Open creates parent dirs, opens SQLite, applies migrations.
func Open(sqlitePath, migrationsDir string, log *slog.Logger) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(sqlitePath), 0o755); err != nil {
		return nil, fmt.Errorf("operator sqlite mkdir: %w", err)
	}
	abs, err := filepath.Abs(sqlitePath)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", sqliteDSN(abs))
	if err != nil {
		return nil, fmt.Errorf("operator sqlite open: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetConnMaxLifetime(0)
	if err := ApplyMigrations(db, migrationsDir, log); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

// Close releases the database handle.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// DB returns the pool for tests.
func (s *Store) DB() *sql.DB {
	if s == nil {
		return nil
	}
	return s.db
}

func (s *Store) nowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

// ListWorkspaces returns all workspaces for a tenant with paths ordered by path id.
func (s *Store) ListWorkspaces(ctx context.Context, tenantID string) ([]Workspace, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("operator store unavailable")
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, tenant_id, project_id, flavor_id, created_at, updated_at
FROM workspaces
WHERE tenant_id = ?
ORDER BY id`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Workspace
	for rows.Next() {
		var w Workspace
		var ca, ua string
		if err := rows.Scan(&w.ID, &w.TenantID, &w.ProjectID, &w.FlavorID, &ca, &ua); err != nil {
			return nil, err
		}
		w.CreatedAt, _ = time.Parse(time.RFC3339Nano, ca)
		if t, err := time.Parse(time.RFC3339Nano, ua); err == nil {
			w.UpdatedAt = t
		}
		out = append(out, w)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		paths, err := s.listPathsForWorkspace(ctx, out[i].ID)
		if err != nil {
			return nil, err
		}
		out[i].Paths = paths
	}
	return out, nil
}

func (s *Store) listPathsForWorkspace(ctx context.Context, workspaceID int64) ([]WorkspacePath, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, workspace_row_id, path, created_at, updated_at
FROM workspace_paths
WHERE workspace_row_id = ?
ORDER BY id`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []WorkspacePath
	for rows.Next() {
		var p WorkspacePath
		var ca, ua string
		if err := rows.Scan(&p.ID, &p.WorkspaceRowID, &p.Path, &ca, &ua); err != nil {
			return nil, err
		}
		p.CreatedAt, _ = time.Parse(time.RFC3339Nano, ca)
		if t, err := time.Parse(time.RFC3339Nano, ua); err == nil {
			p.UpdatedAt = t
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// CreateWorkspace inserts a workspace and paths in one transaction.
func (s *Store) CreateWorkspace(ctx context.Context, tenantID, projectID, flavorID string, absPaths []string) (*Workspace, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("operator store unavailable")
	}
	if projectID == "" {
		return nil, fmt.Errorf("project_id required")
	}
	if len(absPaths) == 0 {
		return nil, fmt.Errorf("at least one path required")
	}
	now := s.nowRFC3339()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx, `
INSERT INTO workspaces (tenant_id, project_id, flavor_id, created_at, updated_at)
VALUES (?,?,?,?,?)`, tenantID, projectID, flavorID, now, now)
	if err != nil {
		return nil, err
	}
	wid, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	for _, p := range absPaths {
		if p == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO workspace_paths (workspace_row_id, path, created_at, updated_at)
VALUES (?,?,?,?)`, wid, p, now, now); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	ws, err := s.GetWorkspace(ctx, tenantID, wid)
	if err != nil {
		return nil, err
	}
	return ws, nil
}

// GetWorkspace returns one workspace or nil if missing / wrong tenant.
func (s *Store) GetWorkspace(ctx context.Context, tenantID string, id int64) (*Workspace, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("operator store unavailable")
	}
	var w Workspace
	var ca, ua string
	err := s.db.QueryRowContext(ctx, `
SELECT id, tenant_id, project_id, flavor_id, created_at, updated_at
FROM workspaces WHERE id = ? AND tenant_id = ?`, id, tenantID).Scan(
		&w.ID, &w.TenantID, &w.ProjectID, &w.FlavorID, &ca, &ua)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	w.CreatedAt, _ = time.Parse(time.RFC3339Nano, ca)
	w.UpdatedAt, _ = time.Parse(time.RFC3339Nano, ua)
	paths, err := s.listPathsForWorkspace(ctx, w.ID)
	if err != nil {
		return nil, err
	}
	w.Paths = paths
	return &w, nil
}

// UpdateWorkspaceProjectFlavor updates scope fields for a workspace.
func (s *Store) UpdateWorkspaceProjectFlavor(ctx context.Context, tenantID string, id int64, projectID, flavorID string) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("operator store unavailable")
	}
	if projectID == "" {
		return fmt.Errorf("project_id required")
	}
	now := s.nowRFC3339()
	res, err := s.db.ExecContext(ctx, `
UPDATE workspaces SET project_id = ?, flavor_id = ?, updated_at = ?
WHERE id = ? AND tenant_id = ?`, projectID, flavorID, now, id, tenantID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("workspace not found")
	}
	return nil
}

// DeleteWorkspace removes a workspace and its paths (CASCADE).
func (s *Store) DeleteWorkspace(ctx context.Context, tenantID string, id int64) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("operator store unavailable")
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM workspaces WHERE id = ? AND tenant_id = ?`, id, tenantID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("workspace not found")
	}
	return nil
}

// AddPath appends a watched path to a workspace.
func (s *Store) AddPath(ctx context.Context, tenantID string, workspaceID int64, absPath string) (*WorkspacePath, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("operator store unavailable")
	}
	if absPath == "" {
		return nil, fmt.Errorf("path required")
	}
	w, err := s.GetWorkspace(ctx, tenantID, workspaceID)
	if err != nil {
		return nil, err
	}
	if w == nil {
		return nil, fmt.Errorf("workspace not found")
	}
	now := s.nowRFC3339()
	res, err := s.db.ExecContext(ctx, `
INSERT INTO workspace_paths (workspace_row_id, path, created_at, updated_at)
VALUES (?,?,?,?)`, workspaceID, absPath, now, now)
	if err != nil {
		return nil, err
	}
	pid, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	_, err = s.db.ExecContext(ctx, `UPDATE workspaces SET updated_at = ? WHERE id = ? AND tenant_id = ?`, now, workspaceID, tenantID)
	if err != nil {
		return nil, err
	}
	return &WorkspacePath{
		ID:             pid,
		WorkspaceRowID: workspaceID,
		Path:           absPath,
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	}, nil
}

// GetPath returns a path row if it exists and belongs to tenant (via workspace join).
func (s *Store) GetPath(ctx context.Context, tenantID string, pathID int64) (*WorkspacePath, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("operator store unavailable")
	}
	var p WorkspacePath
	var ca, ua string
	err := s.db.QueryRowContext(ctx, `
SELECT wp.id, wp.workspace_row_id, wp.path, wp.created_at, wp.updated_at
FROM workspace_paths wp
JOIN workspaces w ON w.id = wp.workspace_row_id
WHERE wp.id = ? AND w.tenant_id = ?`, pathID, tenantID).Scan(
		&p.ID, &p.WorkspaceRowID, &p.Path, &ca, &ua)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	p.CreatedAt, _ = time.Parse(time.RFC3339Nano, ca)
	p.UpdatedAt, _ = time.Parse(time.RFC3339Nano, ua)
	return &p, nil
}

// UpdatePath updates absolute path and/or parent workspace project+flavor.
func (s *Store) UpdatePath(ctx context.Context, tenantID string, pathID int64, newAbsPath *string, projectID, flavorID *string) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("operator store unavailable")
	}
	p, err := s.GetPath(ctx, tenantID, pathID)
	if err != nil {
		return err
	}
	if p == nil {
		return fmt.Errorf("path not found")
	}
	now := s.nowRFC3339()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if projectID != nil || flavorID != nil {
		w, err := s.GetWorkspace(ctx, tenantID, p.WorkspaceRowID)
		if err != nil {
			return err
		}
		if w == nil {
			return fmt.Errorf("workspace not found")
		}
		pj := w.ProjectID
		fv := w.FlavorID
		if projectID != nil {
			if *projectID == "" {
				return fmt.Errorf("project_id required")
			}
			pj = *projectID
		}
		if flavorID != nil {
			fv = *flavorID
		}
		if _, err := tx.ExecContext(ctx, `
UPDATE workspaces SET project_id = ?, flavor_id = ?, updated_at = ? WHERE id = ? AND tenant_id = ?`,
			pj, fv, now, p.WorkspaceRowID, tenantID); err != nil {
			return err
		}
	}
	if newAbsPath != nil {
		if *newAbsPath == "" {
			return fmt.Errorf("path required")
		}
		if _, err := tx.ExecContext(ctx, `
UPDATE workspace_paths SET path = ?, updated_at = ? WHERE id = ?`, *newAbsPath, now, pathID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// DeletePath removes one watched path row.
func (s *Store) DeletePath(ctx context.Context, tenantID string, pathID int64) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("operator store unavailable")
	}
	p, err := s.GetPath(ctx, tenantID, pathID)
	if err != nil {
		return err
	}
	if p == nil {
		return fmt.Errorf("path not found")
	}
	now := s.nowRFC3339()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx, `DELETE FROM workspace_paths WHERE id = ?`, pathID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("path not found")
	}
	if _, err := tx.ExecContext(ctx, `UPDATE workspaces SET updated_at = ? WHERE id = ? AND tenant_id = ?`, now, p.WorkspaceRowID, tenantID); err != nil {
		return err
	}
	return tx.Commit()
}
