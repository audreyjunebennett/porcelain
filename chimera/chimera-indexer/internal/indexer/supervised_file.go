package indexer

import (
	"os"
	"path/filepath"
)

// SupervisedConfigTemplate is the initial single-file YAML when the supervised
// indexer config path does not exist yet (v0.5).
const SupervisedConfigTemplate = `# chimera-indexer supervised config (single --config file; highest merge precedence).
# Watch directories are managed in the gateway operator store (/ui/settings); this file is indexer tuning only.
# Token: set CHIMERA_GATEWAY_TOKEN in the environment (same as gateway / Continue).
# Prefer make chimera-indexer-configure from config/indexer.example.yaml for the full documented template.
gateway_url: "http://127.0.0.1:3000"
roots: []

# Operator profile: quiet per-file INFO; batched summaries while draining.
job_skip_log: debug
job_ingest_log: debug
scope_status_poll_ms: -1
storage_stats_poll_ms: -1
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
