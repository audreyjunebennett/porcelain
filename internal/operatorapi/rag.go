package operatorapi

// RAGSearchRequest is POST /api/ui/rag/search body.
type RAGSearchRequest struct {
	Query          string   `json:"query"`
	ProjectID      string   `json:"project_id"`
	FlavorID       string   `json:"flavor_id"`
	ScoreThreshold *float64 `json:"score_threshold,omitempty"`
	TopK           *int     `json:"top_k,omitempty"`
}

// RAGSearchHit is one retrieval row in POST /api/ui/rag/search response.
type RAGSearchHit struct {
	Source      string  `json:"source"`
	Score       float32 `json:"score"`
	TextExcerpt string  `json:"text_excerpt"`
	PointID     string  `json:"point_id"`
}

// RAGSearchResponse is POST /api/ui/rag/search success JSON.
type RAGSearchResponse struct {
	Hits           []RAGSearchHit `json:"hits"`
	Collection     string         `json:"collection"`
	TopK           int            `json:"top_k"`
	ScoreThreshold float32        `json:"score_threshold"`
	IndexerHint    string         `json:"indexer_hint,omitempty"`
}

// RAGEmbeddingCandidate is one catalog model entry for the embedding selector.
type RAGEmbeddingCandidate struct {
	ID              string `json:"id"`
	EmbeddingLikely bool   `json:"embedding_likely"`
	KnownDim        int    `json:"known_dim,omitempty"`
}

// RAGEmbeddingGetResponse is GET /api/ui/rag/embedding success JSON.
type RAGEmbeddingGetResponse struct {
	Model          string                  `json:"model"`
	Dim            int                     `json:"dim"`
	Status         string                  `json:"status"`
	ModelInCatalog bool                    `json:"model_in_catalog"`
	CatalogStale   bool                    `json:"catalog_stale,omitempty"`
	Candidates     []RAGEmbeddingCandidate `json:"candidates"`
}

// RAGEmbeddingPutRequest is PUT /api/ui/rag/embedding body.
type RAGEmbeddingPutRequest struct {
	Model string `json:"model"`
}

// RAGEmbeddingPutResponse is PUT /api/ui/rag/embedding success JSON.
type RAGEmbeddingPutResponse struct {
	OK    bool   `json:"ok"`
	Model string `json:"model"`
	Dim   int    `json:"dim"`
}
