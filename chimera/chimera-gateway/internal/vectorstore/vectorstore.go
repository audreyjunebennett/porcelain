// Package vectorstore defines the storage interface used by gateway v0.2 RAG.
//
// Implementations live in subpackages (e.g. vectorstore/qdrant for the Qdrant driver).
// Callers must treat Store as the only contract — payload field names and
// collection naming rules are documented here so swapping backends does not
// change wire behavior visible to clients.
package vectorstore

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
)

// Coords identifies a single corpus (one collection per tenant/project/flavor triple in v0.2).
type Coords struct {
	TenantID  string
	ProjectID string
	FlavorID  string
}

// Point is a single vector with its payload.
type Point struct {
	ID      string
	Vector  []float32
	Payload Payload
}

// Payload is the minimum set of fields stored per point (see docs/plans/version-v0.2.md).
// Extra fields may be added by callers but the gateway only filters/reads on these names.
type Payload struct {
	TenantID  string `json:"tenant_id"`
	ProjectID string `json:"project_id"`
	FlavorID  string `json:"flavor_id,omitempty"`
	Text      string `json:"text"`
	Source    string `json:"source"`
	CreatedAt int64  `json:"created_at,omitempty"`
	// ContentSHA256 is the server digest over UTF-8 ingested document bytes
	// (same as ingest response content_sha256).
	ContentSHA256 string `json:"content_sha256,omitempty"`
	// ClientContentHash is the optional indexer-supplied digest echoed at ingest.
	ClientContentHash string `json:"client_content_hash,omitempty"`
}

// PointPayload is a scroll/search row without an embedded vector.
type PointPayload struct {
	ID      string
	Payload Payload
}

// ScrollBatch is one page from ScrollPoints.
type ScrollBatch struct {
	Points     []PointPayload
	NextCursor string // empty when no further pages
}

// Hit is a single retrieval result.
type Hit struct {
	ID      string
	Score   float32
	Payload Payload
}

// Stats reports point counts / vector dim for a collection.
type Stats struct {
	Collection string
	Points     int64
	VectorDim  int
}

// Store is the v0.2 vector-store contract.
type Store interface {
	EnsureCollection(ctx context.Context, name string, dim int) error
	Upsert(ctx context.Context, collection string, points []Point) error
	Search(ctx context.Context, collection string, vector []float32, topK int, scoreThreshold float32, filter *Coords) ([]Hit, error)
	Health(ctx context.Context) error
	Stats(ctx context.Context, collection string) (Stats, error)
	DeleteBySource(ctx context.Context, collection, source string) error
	// DeleteCollection removes an entire collection. Implementations should treat a
	// missing collection as success so purge is idempotent.
	DeleteCollection(ctx context.Context, collection string) error
	// ScrollPoints lists points in collection order for corpus inventory. filter
	// scopes to tenant/project/flavor when non-nil. cursor is empty on the first
	// page; subsequent calls pass ScrollBatch.NextCursor from the prior page.
	ScrollPoints(ctx context.Context, collection string, filter *Coords, limit int, cursor string) (ScrollBatch, error)
}

// CollectionName derives a deterministic collection name from coords.
//
// Rules (per docs/plans/version-v0.2.md): lowercase, slug-safe, hyphen-separated
// triple, with a deterministic short hash suffix to disambiguate inputs that
// would otherwise collide after sanitization. Empty FlavorID is allowed.
func CollectionName(c Coords) string {
	parts := []string{slug(c.TenantID), slug(c.ProjectID), slug(c.FlavorID)}
	for i, p := range parts {
		if p == "" {
			parts[i] = "_"
		}
	}
	prefix := strings.Join(parts, "-")
	h := sha1.Sum([]byte(c.TenantID + "\x00" + c.ProjectID + "\x00" + c.FlavorID))
	suffix := hex.EncodeToString(h[:4])
	full := "chimera-" + prefix + "-" + suffix
	if len(full) > 200 { // backend collection name limit is generous; keep us safe.
		full = full[:200]
	}
	return full
}

var slugInvalid = regexp.MustCompile(`[^a-z0-9]+`)

func slug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = slugInvalid.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		return ""
	}
	return s
}

// PointID returns a deterministic id (UUIDv5-ish hex) for a chunk; same source +
// chunk index always maps to the same id so re-ingest acts as upsert.
func PointID(c Coords, source string, chunkIdx int) string {
	h := sha1.Sum(fmt.Appendf(nil, "%s\x00%s\x00%s\x00%s\x00%d",
		c.TenantID, c.ProjectID, c.FlavorID, source, chunkIdx))
	// Format as 8-4-4-4-12 hex (UUID v4 shape).
	hexs := hex.EncodeToString(h[:16])
	return hexs[:8] + "-" + hexs[8:12] + "-" + hexs[12:16] + "-" + hexs[16:20] + "-" + hexs[20:32]
}
