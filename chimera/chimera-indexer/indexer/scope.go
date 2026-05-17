package indexer

import (
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// ScopeFragment holds optional project / flavor / workspace overrides from
// indexer YAML (v0.3). Empty fields mean “inherit from outer layers”.
type ScopeFragment struct {
	ProjectID   string
	FlavorID    string
	WorkspaceID string
}

// GlobOverride is one overrides[] entry: a glob matched against paths
// relative to a watch root (forward slashes).
type GlobOverride struct {
	Pattern string
	Scope   ScopeFragment
}

func mergeScopeFragment(base, over ScopeFragment) ScopeFragment {
	out := base
	if over.ProjectID != "" {
		out.ProjectID = over.ProjectID
	}
	if over.FlavorID != "" {
		out.FlavorID = over.FlavorID
	}
	if over.WorkspaceID != "" {
		out.WorkspaceID = over.WorkspaceID
	}
	return out
}

// IngestProject returns the value for X-Chimera-Project: explicit project_id,
// else workspace_id when project_id is empty (alias per indexer v0.3 plan).
func IngestProject(s ScopeFragment) string {
	if strings.TrimSpace(s.ProjectID) != "" {
		return strings.TrimSpace(s.ProjectID)
	}
	return strings.TrimSpace(s.WorkspaceID)
}

func (r *Resolved) scopeForRootPath(root Root, relPath string) ScopeFragment {
	s := mergeScopeFragment(r.DefaultScope, root.Scope)
	for _, o := range r.GlobOverrides {
		ok, err := doublestar.Match(o.Pattern, relPath)
		if err != nil || !ok {
			continue
		}
		s = mergeScopeFragment(s, o.Scope)
	}
	return s
}

// IngestHeaders returns X-Chimera-Project and X-Chimera-Flavor-Id for one file
// (root-relative path relPath uses forward slashes).
func (r *Resolved) IngestHeaders(root Root, relPath string) (project, flavor string) {
	s := r.scopeForRootPath(root, relPath)
	return IngestProject(s), strings.TrimSpace(s.FlavorID)
}

// DefaultIndexerHeaders returns optional headers for GET /v1/indexer/config
// using defaults only (per-root / per-glob scope may differ per file).
func (r *Resolved) DefaultIndexerHeaders() map[string]string {
	p, f := IngestProject(r.DefaultScope), strings.TrimSpace(r.DefaultScope.FlavorID)
	if p == "" && f == "" {
		return nil
	}
	m := map[string]string{}
	if p != "" {
		m["X-Chimera-Project"] = p
	}
	if f != "" {
		m["X-Chimera-Flavor-Id"] = f
	}
	return m
}
