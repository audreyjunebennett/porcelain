package config

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// ReplaceFile writes data to path by creating a temp file in the same directory and renaming.
func ReplaceFile(path string, data []byte, perm fs.FileMode) error {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, ".chimera-wr-*")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmpName := f.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return fmt.Errorf("write temp: %w", err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return fmt.Errorf("sync temp: %w", err)
	}
	if perm == 0 {
		perm = 0o644
	}
	if err := f.Chmod(perm); err != nil {
		_ = f.Close()
		return fmt.Errorf("chmod temp: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close temp: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("rename to %s: %w", path, err)
	}
	cleanup = false
	return nil
}

// CommitRoutingAndGateway writes routing policy then gateway.yaml. If the gateway write fails,
// the routing file is restored to its previous contents (or removed if it did not exist).
func CommitRoutingAndGateway(routePath string, routeData []byte, routePerm fs.FileMode, gwPath string, gwData []byte, gwPerm fs.FileMode) error {
	oldRoute, errR := os.ReadFile(routePath)
	routeExisted := errR == nil

	if err := ReplaceFile(routePath, routeData, routePerm); err != nil {
		return fmt.Errorf("write routing policy: %w", err)
	}
	if err := ReplaceFile(gwPath, gwData, gwPerm); err != nil {
		if routeExisted {
			if rerr := ReplaceFile(routePath, oldRoute, routePerm); rerr != nil {
				return fmt.Errorf("gateway write failed and routing rollback failed: gw=%v; rollback=%v", err, rerr)
			}
		} else {
			_ = os.Remove(routePath)
		}
		return fmt.Errorf("write gateway.yaml (routing reverted): %w", err)
	}
	return nil
}
