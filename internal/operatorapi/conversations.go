package operatorapi

// ConversationSummary is one row in GET /api/ui/conversations.
type ConversationSummary struct {
	ConversationID     string `json:"conversation_id"`
	Title              string `json:"title"`
	PreviewText        string `json:"preview_text"`
	Flagged            bool   `json:"flagged"`
	WorkspaceProjectID string `json:"workspace_project_id"`
	WorkspaceFlavorID  string `json:"workspace_flavor_id"`
	WorkspaceRowID     *int64 `json:"workspace_row_id,omitempty"`
	CreatedAt          string `json:"created_at"`
	UpdatedAt          string `json:"updated_at"`
}

// ConversationListResponse is GET /api/ui/conversations.
type ConversationListResponse struct {
	Conversations []ConversationSummary `json:"conversations"`
}

// ConversationRAGHit is a retrieval snippet on an assistant turn.
type ConversationRAGHit struct {
	Source        string  `json:"source"`
	Text          string  `json:"text"`
	Score         float32 `json:"score"`
	Language      string  `json:"language,omitempty"`
	VectorPointID string  `json:"vector_point_id,omitempty"`
	ContentSHA256 string  `json:"content_sha256,omitempty"`
}

// ConversationTurn is one message in a transcript.
type ConversationTurn struct {
	TurnID           string               `json:"turn_id"`
	TurnIndex        int                  `json:"turn_index"`
	Role             string               `json:"role"`
	Content          string               `json:"content"`
	SelectedModel    string               `json:"selected_model,omitempty"`
	ResolvedModel    string               `json:"resolved_model,omitempty"`
	ErrorDetail      string               `json:"error_detail,omitempty"`
	RetryUserText    string               `json:"retryUserText,omitempty"`
	PromptTokens     *int                 `json:"prompt_tokens,omitempty"`
	CompletionTokens *int                 `json:"completion_tokens,omitempty"`
	TotalTokens      *int                 `json:"total_tokens,omitempty"`
	RagHits          []ConversationRAGHit `json:"ragHits,omitempty"`
	CreatedAt        string               `json:"created_at"`
}

// ConversationDetailResponse is GET /api/ui/conversations/{id}.
type ConversationDetailResponse struct {
	ConversationID     string             `json:"conversation_id"`
	Title              string             `json:"title"`
	PreviewText        string             `json:"preview_text"`
	Flagged            bool               `json:"flagged"`
	WorkspaceProjectID string             `json:"workspace_project_id"`
	WorkspaceFlavorID  string             `json:"workspace_flavor_id"`
	WorkspaceRowID     *int64             `json:"workspace_row_id,omitempty"`
	CreatedAt          string             `json:"created_at"`
	UpdatedAt          string             `json:"updated_at"`
	Turns              []ConversationTurn `json:"turns"`
}

// ConversationTitlePatch is PATCH /api/ui/conversations/{id}.
type ConversationTitlePatch struct {
	Title string `json:"title"`
}

// ConversationFlagPatch is POST /api/ui/conversations/{id}/flag.
type ConversationFlagPatch struct {
	Flagged bool `json:"flagged"`
}
