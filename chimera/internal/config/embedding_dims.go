package config

import "strings"

// KnownEmbeddingDim returns the vector dimension for well-known embedding model ids.
// The second return is false when the model is not recognized and callers should probe
// the live embeddings endpoint or reject the change.
func KnownEmbeddingDim(modelID string) (int, bool) {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return 0, false
	}
	if dim, ok := exactEmbeddingModelDims[modelID]; ok {
		return dim, true
	}
	lower := strings.ToLower(modelID)
	for _, rule := range embeddingDimSubstrings {
		if strings.Contains(lower, rule.sub) {
			return rule.dim, true
		}
	}
	return 0, false
}

var exactEmbeddingModelDims = map[string]int{
	"ollama/nomic-embed-text:latest": 768,
	"internal/nomic-embed-text":      768,
	"text-embedding-3-small":         1536,
	"text-embedding-3-large":         3072,
	"text-embedding-ada-002":         1536,
}

type embeddingDimRule struct {
	sub string
	dim int
}

var embeddingDimSubstrings = []embeddingDimRule{
	{sub: "nomic-embed", dim: 768},
	{sub: "text-embedding-3-large", dim: 3072},
	{sub: "text-embedding-3-small", dim: 1536},
	{sub: "text-embedding-ada-002", dim: 1536},
	{sub: "bge-m3", dim: 1024},
	{sub: "bge-large", dim: 1024},
	{sub: "bge-base", dim: 768},
	{sub: "e5-large", dim: 1024},
	{sub: "e5-base", dim: 768},
	{sub: "e5-small", dim: 384},
	{sub: "jina-embeddings-v2", dim: 768},
}
