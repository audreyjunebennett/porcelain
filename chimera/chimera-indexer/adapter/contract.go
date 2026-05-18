// Package adapter implements the chimera-indexer wrapper runtime and exposes a
// small stable surface for other Chimera binaries (gateway, supervisor).
package adapter

import (
	idx "github.com/lynn/porcelain/chimera/chimera-indexer/internal/indexer"
)

// FileConfig is the on-disk YAML schema for indexer configuration.
type FileConfig = idx.FileConfig

const (
	// EnvGatewayURL is the preferred environment variable for gateway base URL.
	EnvGatewayURL = idx.EnvGatewayURL
	// EnvGatewayToken is the preferred environment variable for gateway bearer token.
	EnvGatewayToken = idx.EnvGatewayToken
)

// EnsureSupervisedConfigFile creates the parent directory and starter YAML when missing.
func EnsureSupervisedConfigFile(path string) error {
	return idx.EnsureSupervisedConfigFile(path)
}
