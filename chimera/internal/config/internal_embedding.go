package config

import (
	"fmt"
	"net"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/lynn/porcelain/internal/naming"
)

// InternalEmbedding configures the optional supervised chimera-embed runtime.
type InternalEmbedding struct {
	Enabled   bool
	Provider  string
	Model     string
	Dim       int
	BaseURL   string
	ModelPath string
	CacheDir  string
	LogLevel  string
}

type internalEmbeddingDoc struct {
	Enabled   *bool  `yaml:"enabled"`
	Provider  string `yaml:"provider"`
	Model     string `yaml:"model"`
	Dim       int    `yaml:"dim"`
	BaseURL   string `yaml:"base_url"`
	ModelPath string `yaml:"model_path"`
	CacheDir  string `yaml:"cache_dir"`
	LogLevel  string `yaml:"log_level"`
}

func (d internalEmbeddingDoc) effective(configDir string) InternalEmbedding {
	out := InternalEmbedding{
		Enabled:   d.Enabled != nil && *d.Enabled,
		Provider:  strings.TrimSpace(d.Provider),
		Model:     strings.TrimSpace(d.Model),
		Dim:       d.Dim,
		BaseURL:   strings.TrimRight(strings.TrimSpace(d.BaseURL), "/"),
		ModelPath: resolveInternalEmbeddingPath(configDir, d.ModelPath, filepath.Join("..", naming.DefaultEmbedModelPath)),
		CacheDir:  resolveInternalEmbeddingPath(configDir, d.CacheDir, filepath.Join("..", naming.DefaultEmbedCacheDir)),
		LogLevel:  strings.TrimSpace(d.LogLevel),
	}
	if out.Provider == "" {
		out.Provider = naming.InternalEmbeddingProvider
	}
	if out.Model == "" {
		out.Model = naming.DefaultInternalEmbedModel
	}
	if out.Dim <= 0 {
		out.Dim = naming.DefaultInternalEmbedDim
	}
	if out.BaseURL == "" {
		out.BaseURL = "http://" + naming.DefaultEmbedEndpoint
	}
	if out.LogLevel == "" {
		out.LogLevel = naming.DefaultEmbedLogLevel
	}
	return out
}

func resolveInternalEmbeddingPath(configDir, configured, fallback string) string {
	p := strings.TrimSpace(configured)
	if p == "" {
		p = fallback
	}
	if filepath.IsAbs(p) {
		return filepath.Clean(p)
	}
	return filepath.Clean(filepath.Join(configDir, p))
}

// Validate checks semantic settings without requiring the model file to exist yet.
func (ie InternalEmbedding) Validate() error {
	if !ie.Enabled {
		return nil
	}
	if ie.Provider == "" || ie.Model == "" {
		return fmt.Errorf("internal_embedding provider and model are required when enabled")
	}
	if ie.Dim <= 0 {
		return fmt.Errorf("internal_embedding.dim must be > 0 when enabled")
	}
	u, err := url.Parse(ie.BaseURL)
	if err != nil || u.Scheme != "http" || u.Host == "" {
		return fmt.Errorf("internal_embedding.base_url must be an http URL, got %q", ie.BaseURL)
	}
	if ie.ModelPath == "" {
		return fmt.Errorf("internal_embedding.model_path is required when enabled")
	}
	return nil
}

// Endpoint returns the host:port used to launch the supervised llama-server.
func (ie InternalEmbedding) Endpoint() (string, error) {
	u, err := url.Parse(strings.TrimSpace(ie.BaseURL))
	if err != nil || u.Scheme != "http" || u.Host == "" {
		return "", fmt.Errorf("invalid internal embedding base URL %q", ie.BaseURL)
	}
	host := u.Hostname()
	port := u.Port()
	if port == "" {
		port = "80"
	}
	return net.JoinHostPort(host, port), nil
}

// UsesInternalProvider reports whether modelID belongs to the configured internal provider.
func UsesInternalProvider(modelID string, ie InternalEmbedding) bool {
	if !ie.Enabled {
		return false
	}
	modelID = strings.TrimSpace(modelID)
	if strings.EqualFold(modelID, ie.Model) {
		return true
	}
	slash := strings.IndexByte(modelID, '/')
	return slash > 0 && strings.EqualFold(modelID[:slash], ie.Provider)
}

// applyInternalEmbeddingToRAG makes enabling the internal provider deterministic:
// the gateway embeds directly through chimera-embed rather than the broker catalog.
func applyInternalEmbeddingToRAG(rag *RAG, ie InternalEmbedding) {
	if rag == nil || !rag.Enabled || !ie.Enabled {
		return
	}
	rag.EmbeddingBaseURL = ie.BaseURL
	rag.EmbeddingModel = ie.Model
	rag.EmbeddingDim = ie.Dim
}
