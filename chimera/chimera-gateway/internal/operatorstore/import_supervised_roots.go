package operatorstore

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type supervisedRootsYAML struct {
	Defaults *struct {
		ProjectID   string `yaml:"project_id"`
		FlavorID    string `yaml:"flavor_id"`
		WorkspaceID string `yaml:"workspace_id"`
	} `yaml:"defaults"`
	Roots []supervisedRootEntry `yaml:"roots"`
}

type supervisedRootEntry struct {
	Path        string `yaml:"path"`
	ProjectID   string `yaml:"project_id"`
	FlavorID    string `yaml:"flavor_id"`
	WorkspaceID string `yaml:"workspace_id"`
}

// ImportSupervisedYAMLRootsIfEmpty copies legacy roots: from indexer.supervised.yaml
// into operator SQLite when the tenant has no workspace rows yet.
func ImportSupervisedYAMLRootsIfEmpty(ctx context.Context, s *Store, tenantID, yamlPath string, log *slog.Logger) error {
	if s == nil || s.db == nil {
		return nil
	}
	yamlPath = strings.TrimSpace(yamlPath)
	if yamlPath == "" {
		return nil
	}
	existing, err := s.ListWorkspaces(ctx, tenantID)
	if err != nil {
		return err
	}
	if len(existing) > 0 {
		return nil
	}
	b, err := os.ReadFile(yamlPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read supervised indexer config: %w", err)
	}
	var doc supervisedRootsYAML
	if err := yaml.Unmarshal(b, &doc); err != nil {
		return fmt.Errorf("parse supervised indexer config: %w", err)
	}
	if len(doc.Roots) == 0 {
		return nil
	}
	defProj, defFlav := "", ""
	if doc.Defaults != nil {
		defProj = strings.TrimSpace(doc.Defaults.ProjectID)
		defFlav = strings.TrimSpace(doc.Defaults.FlavorID)
	}
	imported := 0
	for _, entry := range doc.Roots {
		p := strings.TrimSpace(entry.Path)
		if p == "" {
			continue
		}
		abs, err := filepath.Abs(p)
		if err != nil {
			continue
		}
		abs = filepath.Clean(abs)
		st, err := os.Stat(abs)
		if err != nil || !st.IsDir() {
			if log != nil {
				log.Warn("skip legacy supervised root (not a directory)",
					"msg", "gateway.operator.workspaces_import_skipped",
					"path", abs,
					"err", err,
				)
			}
			continue
		}
		proj := strings.TrimSpace(entry.ProjectID)
		if proj == "" {
			proj = defProj
		}
		if proj == "" {
			proj = "default"
		}
		flav := strings.TrimSpace(entry.FlavorID)
		if flav == "" {
			flav = defFlav
		}
		if flav == "" {
			flav = "default"
		}
		ws, err := s.CreateWorkspace(ctx, tenantID, proj, flav, []string{abs})
		if err != nil {
			return fmt.Errorf("import legacy root %q: %w", abs, err)
		}
		imported++
		if log != nil {
			log.Info("imported legacy supervised YAML root into operator sqlite",
				"msg", "gateway.operator.workspaces_imported",
				"type", "gateway.operator.workspaces_imported",
				"workspace_id", ws.ID,
				"path", abs,
				"project_id", proj,
				"flavor_id", flav,
			)
		}
	}
	if imported > 0 && log != nil {
		log.Info("legacy supervised roots migration complete",
			"msg", "gateway.operator.workspaces_import_done",
			"imported", imported,
			"yaml", yamlPath,
		)
	}
	return nil
}
