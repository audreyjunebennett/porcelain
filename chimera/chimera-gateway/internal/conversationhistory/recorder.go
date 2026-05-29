package conversationhistory

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/chat"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/operatorstore"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/rag"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/vectorstore"
)

// TurnContext is workspace and identity metadata for one chat exchange.
type TurnContext struct {
	PrincipalID    string
	ConversationID string
	UserText       string
	SelectedModel  string
	ProjectID      string
	FlavorID       string
	WorkspaceRowID *int64
}

// Recorder persists operator chat turns to SQLite (best-effort).
type Recorder struct {
	store   *operatorstore.Store
	log     *slog.Logger
	ctx     context.Context
	turn    TurnContext
	ragHits []vectorstore.Hit
}

// NewRecorder returns a recorder when store is non-nil.
func NewRecorder(store *operatorstore.Store, log *slog.Logger, ctx context.Context, turn TurnContext) *Recorder {
	if store == nil || strings.TrimSpace(turn.ConversationID) == "" {
		return nil
	}
	return &Recorder{store: store, log: log, ctx: ctx, turn: turn}
}

// SetRAGHits attaches retrieval hits for the current exchange (virtual-model path).
func (r *Recorder) SetRAGHits(hits []vectorstore.Hit) {
	if r == nil {
		return
	}
	r.ragHits = hits
}

// Attach wires persistence hooks into chat.ProxyOpts.
func (r *Recorder) Attach(opts **chat.ProxyOpts) {
	if r == nil {
		return
	}
	capture := func(statusCode int, upstreamModel string, stream bool, body []byte) {
		r.onResponseCaptured(statusCode, upstreamModel, stream, body)
	}
	if *opts == nil {
		*opts = &chat.ProxyOpts{OnResponseCaptured: capture}
		return
	}
	prev := (*opts).OnResponseCaptured
	(*opts).OnResponseCaptured = func(statusCode int, upstreamModel string, stream bool, body []byte) {
		if prev != nil {
			prev(statusCode, upstreamModel, stream, body)
		}
		capture(statusCode, upstreamModel, stream, body)
	}
}

// PersistDedup records a merge dedup-cache response as a completed turn.
func (r *Recorder) PersistDedup(jsonBody []byte) {
	if r == nil {
		return
	}
	resolved := ResolvedModelFromJSON(jsonBody)
	if resolved == "" {
		resolved = r.turn.SelectedModel
	}
	content := AssistantContentFromResponse(false, jsonBody)
	r.persistSuccess(resolved, content, jsonBody, false)
}

// PersistGatewayError records user + error rows for inline gateway errors.
func (r *Recorder) PersistGatewayError(status int, errBody map[string]any) {
	if r == nil {
		return
	}
	msg := ""
	errType := ""
	if errBody != nil {
		if e, ok := errBody["error"].(map[string]any); ok {
			if m, ok := e["message"].(string); ok {
				msg = strings.TrimSpace(m)
			}
			if t, ok := e["type"].(string); ok {
				errType = strings.TrimSpace(t)
			}
		}
	}
	if msg == "" {
		msg = http.StatusText(status)
	}
	r.persistFailure(msg, errType)
}

func (r *Recorder) onResponseCaptured(statusCode int, upstreamModel string, stream bool, body []byte) {
	if r == nil {
		return
	}
	if statusCode >= 200 && statusCode < 300 {
		content := AssistantContentFromResponse(stream, body)
		resolved := strings.TrimSpace(upstreamModel)
		if resolved == "" {
			resolved = ResolvedModelFromJSON(body)
		}
		if resolved == "" {
			resolved = r.turn.SelectedModel
		}
		r.persistSuccess(resolved, content, body, stream)
		return
	}
	msg, errType := ErrorFromJSON(body)
	if msg == "" {
		msg = http.StatusText(statusCode)
	}
	r.persistFailure(msg, errType)
}

func (r *Recorder) persistSuccess(resolvedModel, assistantContent string, body []byte, stream bool) {
	if err := r.ensureConversation(); err != nil {
		r.warn("ensure conversation", err)
		return
	}
	if _, err := r.store.AppendTurn(r.ctx, r.turn.PrincipalID, r.turn.ConversationID, operatorstore.AppendTurnInput{
		Role: "user", Content: r.turn.UserText,
	}); err != nil {
		r.warn("append user turn", err)
		return
	}
	pt, ct, tot, hasUsage := UsageFromResponse(stream, body)
	var promptPtr, completionPtr, totalPtr *int
	if hasUsage {
		promptPtr, completionPtr, totalPtr = &pt, &ct, &tot
	}
	turnID, err := r.store.AppendTurn(r.ctx, r.turn.PrincipalID, r.turn.ConversationID, operatorstore.AppendTurnInput{
		Role:             "assistant",
		Content:          assistantContent,
		SelectedModel:    r.turn.SelectedModel,
		ResolvedModel:    resolvedModel,
		PromptTokens:     promptPtr,
		CompletionTokens: completionPtr,
		TotalTokens:      totalPtr,
	})
	if err != nil {
		r.warn("append assistant turn", err)
		return
	}
	hits := r.retrievalInputs()
	if len(hits) > 0 {
		if err := r.store.ReplaceTurnRetrievals(r.ctx, turnID, hits); err != nil {
			r.warn("replace retrievals", err)
		}
	}
}

func (r *Recorder) persistFailure(message, errType string) {
	if strings.TrimSpace(r.turn.UserText) == "" {
		return
	}
	if err := r.ensureConversation(); err != nil {
		r.warn("ensure conversation", err)
		return
	}
	if _, err := r.store.AppendTurn(r.ctx, r.turn.PrincipalID, r.turn.ConversationID, operatorstore.AppendTurnInput{
		Role: "user", Content: r.turn.UserText,
	}); err != nil {
		r.warn("append user turn", err)
		return
	}
	detail := errType
	if detail != "" && message != "" {
		detail = errType + ": " + message
	} else if detail == "" {
		detail = message
	}
	if _, err := r.store.AppendTurn(r.ctx, r.turn.PrincipalID, r.turn.ConversationID, operatorstore.AppendTurnInput{
		Role:          "error",
		Content:       message,
		ErrorDetail:   detail,
		RetryUserText: r.turn.UserText,
		SelectedModel: r.turn.SelectedModel,
	}); err != nil {
		r.warn("append error turn", err)
	}
}

func (r *Recorder) ensureConversation() error {
	ws := operatorstore.ConversationWorkspaceSnapshot{
		ProjectID: r.turn.ProjectID,
		FlavorID:  r.turn.FlavorID,
		RowID:     r.turn.WorkspaceRowID,
	}
	return r.store.EnsureConversation(r.ctx, r.turn.PrincipalID, r.turn.ConversationID, r.turn.UserText, ws)
}

func (r *Recorder) retrievalInputs() []operatorstore.RetrievalInput {
	if len(r.ragHits) == 0 {
		return nil
	}
	summaries := rag.SummarizeHits(r.ragHits)
	out := make([]operatorstore.RetrievalInput, 0, len(r.ragHits))
	for i, h := range r.ragHits {
		src := strings.TrimSpace(h.Payload.Source)
		if src == "" {
			src = "unknown"
		}
		snippet := ""
		lang := rag.LanguageFromSource(src)
		if i < len(summaries) {
			snippet = summaries[i].Text
			if summaries[i].Language != "" {
				lang = summaries[i].Language
			}
		}
		out = append(out, operatorstore.RetrievalInput{
			FilePath:      src,
			Score:         h.Score,
			SnippetText:   snippet,
			Language:      lang,
			VectorPointID: strings.TrimSpace(h.ID),
			ContentSHA256: strings.TrimSpace(h.Payload.ContentSHA256),
		})
	}
	return out
}

func (r *Recorder) warn(step string, err error) {
	if r == nil || err == nil {
		return
	}
	if r.log != nil {
		r.log.Warn("conversation history persist failed", "msg", "gateway.conversation_history.persist_failed",
			"step", step, "conversation_id", r.turn.ConversationID, "err", err)
	}
}

// PersistUpstreamErrorBody parses a JSON error body from upstream.
func (r *Recorder) PersistUpstreamErrorBody(status int, body []byte) {
	if r == nil {
		return
	}
	msg, errType := ErrorFromJSON(body)
	if msg == "" {
		var wrap map[string]any
		if json.Unmarshal(body, &wrap) == nil {
			r.PersistGatewayError(status, wrap)
			return
		}
		msg = http.StatusText(status)
	}
	r.persistFailure(msg, errType)
}
