package indexer

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// IndexerKey returns a compact stable fingerprint of the authenticated user
// (tenant) plus the default RAG ingest scope **project_id** and **flavor_id**.
// Workspace is deliberately excluded so /ui/logs groups indexers by the same
// identity triple operators use when scoping ingestion, not indexer YAML nesting.
//
// All arguments should already be trimmed / resolved (e.g. effective project after
// Ingest-style fallbacks + gateway defaults when headers are omitted).
func IndexerKey(tenantID, projectID, flavorID string) string {
	norm := strings.Join([]string{
		strings.TrimSpace(tenantID),
		strings.TrimSpace(projectID),
		strings.TrimSpace(flavorID),
	}, "\x1e")
	sum := sha256.Sum256([]byte(norm))
	return "ik_" + hex.EncodeToString(sum[:12])
}
