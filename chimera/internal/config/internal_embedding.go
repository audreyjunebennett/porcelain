package config

import (
	"fmt"
	"strings"

	"github.com/lynn/porcelain/internal/naming"
)

// InternalEmbedding configures a supervised local embedding runtime (chimera-embed + llama-server).
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

const defaultInternalEmbedBaseURL = "http://127.0.0.1:8090"

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

func (d internalEmbeddingDoc) effective() InternalEmbedding {
	out := InternalEmbedding{
		Enabled:   d.Enabled != nil && *d.Enabled,
		Provider:  strings.TrimSpace(d.Provider),
		Model:     strings.TrimSpace(d.Model),
		Dim:       d.Dim,
		BaseURL:   strings.TrimSuffix(strings.TrimSpace(d.BaseURL), "/"),
		ModelPath: strings.TrimSpace(d.ModelPath),
		CacheDir:  strings.TrimSpace(d.CacheDir),
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
		out.BaseURL = defaultInternalEmbedBaseURL
	}
	if out.ModelPath == "" {
		out.ModelPath = naming.DefaultEmbedModelPath
	}
	if out.CacheDir == "" {
		out.CacheDir = naming.DefaultEmbedCacheDir
	}
	if out.LogLevel == "" {
		out.LogLevel = naming.DefaultEmbedLogLevel
	}
	return out
}

func (ie InternalEmbedding) Validate() error {
	if !ie.Enabled {
		return nil
	}
	if ie.Provider == "" {
		return fmt.Errorf("internal_embedding.provider is required when enabled")
	}
	if ie.Model == "" {
		return fmt.Errorf("internal_embedding.model is required when enabled")
	}
	if ie.Dim <= 0 {
		return fmt.Errorf("internal_embedding.dim must be > 0 when enabled")
	}
	if ie.BaseURL == "" {
		return fmt.Errorf("internal_embedding.base_url is required when enabled")
	}
	if !strings.HasPrefix(ie.BaseURL, "http://") && !strings.HasPrefix(ie.BaseURL, "https://") {
		return fmt.Errorf("internal_embedding.base_url must be http(s), got %q", ie.BaseURL)
	}
	if ie.ModelPath == "" {
		return fmt.Errorf("internal_embedding.model_path is required when enabled")
	}
	return nil
}

// ProviderPrefix returns the model id prefix before the first slash (e.g. "internal").
func (ie InternalEmbedding) ProviderPrefix() string {
	return providerPrefixFromModel(ie.Model)
}

func providerPrefixFromModel(modelID string) string {
	modelID = strings.TrimSpace(modelID)
	if slash := strings.Index(modelID, "/"); slash > 0 {
		return strings.ToLower(modelID[:slash])
	}
	return ""
}

// UsesInternalProvider reports whether modelID is served by the internal embedding runtime.
func UsesInternalProvider(modelID string, ie InternalEmbedding) bool {
	if !ie.Enabled {
		return false
	}
	prefix := providerPrefixFromModel(modelID)
	if prefix == "" {
		return false
	}
	return strings.EqualFold(prefix, ie.Provider)
}

// applyInternalEmbeddingToRAG rewires rag.embedding to the local runtime when enabled.
func applyInternalEmbeddingToRAG(rag *RAG, ie InternalEmbedding, brokerBaseURL string) {
	if rag == nil || !ie.Enabled || !rag.Enabled {
		return
	}
	brokerBase := strings.TrimSuffix(strings.TrimSpace(brokerBaseURL), "/")
	embBase := strings.TrimSuffix(strings.TrimSpace(rag.EmbeddingBaseURL), "/")
	model := strings.TrimSpace(rag.EmbeddingModel)

	switch {
	case embBase == "" || embBase == brokerBase:
		rag.EmbeddingBaseURL = ie.BaseURL
	case strings.HasPrefix(strings.ToLower(model), "ollama/"):
		rag.EmbeddingBaseURL = ie.BaseURL
	}

	if model == "" || strings.HasPrefix(strings.ToLower(model), "ollama/") {
		rag.EmbeddingModel = ie.Model
	}
	if rag.EmbeddingDim <= 0 || rag.EmbeddingDim == defaultEmbeddingDim {
		rag.EmbeddingDim = ie.Dim
	}
}
