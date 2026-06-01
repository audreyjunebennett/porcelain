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
