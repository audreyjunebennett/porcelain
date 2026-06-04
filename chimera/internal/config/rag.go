package config

import (
	"fmt"
	"strings"
)

// RAG holds resolved retrieval-augmented-generation settings (gateway v0.2).
//
// All fields are populated by LoadGatewayYAML from the optional "rag" block in
// gateway.yaml. When RAG is disabled the rest of the gateway must continue to
// behave exactly as v0.1; ingest, indexer REST, retrieval, and the /health
// Qdrant probe are all gated on Enabled.
type RAG struct {
	Enabled bool

	// Vector store (Qdrant HTTP REST in v0.2; one adapter per scheme).
	QdrantURL      string
	QdrantAPIKey   string
	QdrantLogLevel string // supervised native Qdrant: QDRANT__LOGGER__LOG_LEVEL when non-empty; `-qdrant-log-level` overrides.

	// Embedding configuration. EmbeddingBaseURL falls back to broker.base_url
	// when empty; EmbeddingPath is appended (default "/v1/embeddings").
	EmbeddingBaseURL string
	EmbeddingPath    string
	EmbeddingModel   string
	EmbeddingDim     int

	// Chunking knobs (UTF-8 code units; see indexer plan).
	ChunkSize    int
	ChunkOverlap int

	// Retrieval knobs.
	TopK              int
	ScoreThreshold    float64
	MaxIngestBytes    int64
	MaxWholeFileBytes int64 // single POST /v1/ingest body limit hint; 0 = same as MaxIngestBytes
	DefaultProject    string
	DefaultFlavor     string
	CollectionScope   string // reserved for future scope keys (currently always "tenant_project_flavor")

	// CoherenceMode controls stale-index warnings: off | warn | strict (default warn).
	CoherenceMode string
	// ToolingEnabled gates /v1/rag/* expansion APIs.
	ToolingEnabled bool
	// ExpansionCacheTTLSeconds is LRU TTL for merged context slices (0 = 300).
	ExpansionCacheTTLSeconds int
	// ExpansionCacheMaxEntries caps hot context cache size (0 = 256).
	ExpansionCacheMaxEntries int
}

const (
	defaultEmbeddingPath  = "/v1/embeddings"
	defaultEmbeddingModel = "text-embedding-3-small"
	defaultEmbeddingDim   = 1536
	defaultChunkSize      = 512
	defaultChunkOverlap   = 128
	defaultTopK           = 8
	defaultScoreThreshold = 0.72
	defaultMaxIngestBytes = 10 * 1024 * 1024
	defaultQdrantURL      = "http://127.0.0.1:6333"
)

// vectorstoreDoc is the YAML shape for chimera-vectorstore (top-level in gateway.yaml).
type vectorstoreDoc struct {
	URL      string `yaml:"url"`
	APIKey   string `yaml:"api_key"`
	LogLevel string `yaml:"log_level"`
}

// effective returns a RAG with defaults filled. When rag.enabled is false the
// gateway stays on the v0.1 path; vectorstore fields are still resolved for health URLs.
func (d ragDoc) effective(vs vectorstoreDoc) RAG {
	r := RAG{
		Enabled:           d.Enabled != nil && *d.Enabled,
		QdrantURL:         strings.TrimSpace(vs.URL),
		QdrantAPIKey:      strings.TrimSpace(vs.APIKey),
		QdrantLogLevel:    strings.TrimSpace(vs.LogLevel),
		EmbeddingBaseURL:  strings.TrimSpace(d.Embedding.BaseURL),
		EmbeddingPath:     strings.TrimSpace(d.Embedding.Path),
		EmbeddingModel:    strings.TrimSpace(d.Embedding.Model),
		EmbeddingDim:      d.Embedding.Dim,
		ChunkSize:         d.Chunking.Size,
		ChunkOverlap:      d.Chunking.Overlap,
		TopK:              d.Retrieval.TopK,
		ScoreThreshold:    d.Retrieval.ScoreThreshold,
		MaxIngestBytes:    d.Ingest.MaxBytes,
		MaxWholeFileBytes: d.Ingest.MaxWholeFileBytes,
		DefaultProject:    strings.TrimSpace(d.Defaults.ProjectID),
		DefaultFlavor:     strings.TrimSpace(d.Defaults.FlavorID),
		CollectionScope:   "tenant_project_flavor",
		CoherenceMode:     normalizeCoherenceMode(d.Coherence.Mode),
		ToolingEnabled:    d.Tooling.Enabled == nil || *d.Tooling.Enabled,
		ExpansionCacheTTLSeconds: d.Tooling.CacheTTLSeconds,
		ExpansionCacheMaxEntries: d.Tooling.CacheMaxEntries,
	}
	if r.ExpansionCacheTTLSeconds <= 0 {
		r.ExpansionCacheTTLSeconds = 300
	}
	if r.ExpansionCacheMaxEntries <= 0 {
		r.ExpansionCacheMaxEntries = 256
	}
	if r.QdrantURL == "" {
		r.QdrantURL = defaultQdrantURL
	}
	r.QdrantURL = strings.TrimSuffix(r.QdrantURL, "/")
	if r.EmbeddingPath == "" {
		r.EmbeddingPath = defaultEmbeddingPath
	}
	if !strings.HasPrefix(r.EmbeddingPath, "/") {
		r.EmbeddingPath = "/" + r.EmbeddingPath
	}
	if r.EmbeddingModel == "" {
		r.EmbeddingModel = defaultEmbeddingModel
	}
	if r.EmbeddingDim <= 0 {
		r.EmbeddingDim = defaultEmbeddingDim
	}
	if r.ChunkSize <= 0 {
		r.ChunkSize = defaultChunkSize
	}
	if r.ChunkOverlap <= 0 {
		r.ChunkOverlap = defaultChunkOverlap
	}
	if r.ChunkOverlap >= r.ChunkSize {
		r.ChunkOverlap = r.ChunkSize / 4
	}
	if r.TopK <= 0 {
		r.TopK = defaultTopK
	}
	if r.ScoreThreshold <= 0 {
		r.ScoreThreshold = defaultScoreThreshold
	}
	if r.MaxIngestBytes <= 0 {
		r.MaxIngestBytes = defaultMaxIngestBytes
	}
	if r.MaxWholeFileBytes <= 0 || r.MaxWholeFileBytes > r.MaxIngestBytes {
		r.MaxWholeFileBytes = r.MaxIngestBytes
	}
	return r
}

// EmbeddingURL returns the absolute embedding endpoint for the resolved RAG
// config. upstreamBaseURL is used as a fallback when EmbeddingBaseURL is empty.
func (r RAG) EmbeddingURL(upstreamBaseURL string) string {
	base := r.EmbeddingBaseURL
	if base == "" {
		base = upstreamBaseURL
	}
	base = strings.TrimSuffix(strings.TrimSpace(base), "/")
	if base == "" {
		return r.EmbeddingPath
	}
	return base + r.EmbeddingPath
}

// Validate returns an error when Enabled is true but settings are inconsistent
// (e.g. invalid Qdrant URL). Disabled RAG always validates.
func (r RAG) Validate() error {
	if !r.Enabled {
		return nil
	}
	if r.QdrantURL == "" {
		return fmt.Errorf("vectorstore.url is required when rag.enabled=true")
	}
	if !strings.HasPrefix(r.QdrantURL, "http://") && !strings.HasPrefix(r.QdrantURL, "https://") {
		return fmt.Errorf("vectorstore.url must be http:// or https://, got %q", r.QdrantURL)
	}
	if r.EmbeddingDim <= 0 {
		return fmt.Errorf("rag.embedding.dim must be > 0")
	}
	if r.ChunkSize <= 0 || r.ChunkOverlap < 0 || r.ChunkOverlap >= r.ChunkSize {
		return fmt.Errorf("rag.chunking: size=%d overlap=%d invalid", r.ChunkSize, r.ChunkOverlap)
	}
	if r.MaxWholeFileBytes > r.MaxIngestBytes {
		return fmt.Errorf("rag.ingest.max_whole_file_bytes (%d) cannot exceed max_bytes (%d)", r.MaxWholeFileBytes, r.MaxIngestBytes)
	}
	return nil
}

// ragDoc is the YAML shape parsed out of gateway.yaml's "rag" block (orchestration only).
type ragDoc struct {
	Enabled   *bool `yaml:"enabled"`
	Embedding struct {
		BaseURL string `yaml:"base_url"`
		Path    string `yaml:"path"`
		Model   string `yaml:"model"`
		Dim     int    `yaml:"dim"`
	} `yaml:"embedding"`
	Chunking struct {
		Size    int `yaml:"size"`
		Overlap int `yaml:"overlap"`
	} `yaml:"chunking"`
	Retrieval struct {
		TopK           int     `yaml:"top_k"`
		ScoreThreshold float64 `yaml:"score_threshold"`
	} `yaml:"retrieval"`
	Ingest struct {
		MaxBytes          int64 `yaml:"max_bytes"`
		MaxWholeFileBytes int64 `yaml:"max_whole_file_bytes"`
	} `yaml:"ingest"`
	Defaults struct {
		ProjectID string `yaml:"project_id"`
		FlavorID  string `yaml:"flavor_id"`
	} `yaml:"defaults"`
	Coherence struct {
		Mode string `yaml:"mode"`
	} `yaml:"coherence"`
	Tooling struct {
		Enabled          *bool `yaml:"enabled"`
		CacheTTLSeconds  int   `yaml:"expansion_cache_ttl_seconds"`
		CacheMaxEntries  int   `yaml:"expansion_cache_max_entries"`
	} `yaml:"tooling"`
}

func normalizeCoherenceMode(m string) string {
	switch strings.ToLower(strings.TrimSpace(m)) {
	case "off", "strict":
		return strings.ToLower(strings.TrimSpace(m))
	default:
		return "warn"
	}
}
