package indexer

import (
	"os"
	"path/filepath"
)

// SupervisedConfigTemplate is the initial single-file YAML when the supervised
// indexer config path does not exist yet (v0.5).
const SupervisedConfigTemplate = `# chimera-indexer supervised config (single --config file; highest merge precedence).
# Watch directories are managed in the gateway operator store (logs UI); this file is indexer tuning only.
# Token: set CHIMERA_GATEWAY_TOKEN in the environment (same as gateway / Continue).
gateway_url: "http://127.0.0.1:3000"
roots: []
`

// EnsureSupervisedConfigFile creates the parent directory and a starter YAML
// when path is missing. Existing files are left unchanged.
func EnsureSupervisedConfigFile(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(SupervisedConfigTemplate), 0o644)
}
