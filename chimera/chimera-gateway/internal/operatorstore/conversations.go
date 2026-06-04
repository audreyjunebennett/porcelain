package operatorstore

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/conversationtitle"
)

// ConversationSummary is list metadata for one saved thread.
type ConversationSummary struct {
	ConversationID     string
	PrincipalID        string
	Title              sql.NullString
	PreviewText        string
	Flagged            bool
	WorkspaceProjectID string
	WorkspaceFlavorID  string
	WorkspaceRowID     sql.NullInt64
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// ConversationWorkspaceSnapshot records workspace scope at conversation start.
type ConversationWorkspaceSnapshot struct {
	ProjectID string
	FlavorID  string
	RowID     *int64
}

// ListConversationsFilter scopes conversation list queries.
type ListConversationsFilter struct {
	Limit       int
	Offset      int
	FlaggedOnly bool
}

// AppendTurnInput is one persisted message row.
type AppendTurnInput struct {
	Role             string
	Content          string
	SelectedModel    string
	ResolvedModel    string
	ErrorDetail      string
	RetryUserText    string
	PromptTokens     *int
	CompletionTokens *int
	TotalTokens      *int
}

// RetrievalInput is one RAG hit attached to an assistant turn.
type RetrievalInput struct {
	FilePath      string
	Score         float32
	SnippetText   string
	Language      string
	VectorPointID string
	ContentSHA256 string
	StartLine     int
	EndLine       int
	StartsMidLine bool
}

// ConversationTurn is a loaded transcript row.
type ConversationTurn struct {
	TurnID           string
	ConversationID   string
	TurnIndex        int
	Role             string
	Content          string
	SelectedModel    string
	ResolvedModel    string
	ErrorDetail      string
	RetryUserText    string
	PromptTokens     sql.NullInt64
	CompletionTokens sql.NullInt64
	TotalTokens      sql.NullInt64
	CreatedAt        time.Time
	Retrievals       []ConversationRetrieval
}

// ConversationRetrieval is a RAG hit on an assistant turn.
type ConversationRetrieval struct {
	RetrievalID   string
	TurnID        string
	SortOrder     int
	FilePath      string
	Score         float32
	SnippetText   string
	Language      string
	VectorPointID string
	ContentSHA256 string
	StartLine     int
	EndLine       int
	StartsMidLine bool
}

// ConversationTranscript is a full thread with turns and retrievals.
type ConversationTranscript struct {
	Summary ConversationSummary
	Turns   []ConversationTurn
}

func newTurnID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

func newRetrievalID() (string, error) {
	return newTurnID()
}

func parseTime(s string) time.Time {
	t, _ := time.Parse(time.RFC3339Nano, s)
	return t
}

// EnsureConversation inserts a conversation row when missing; sets preview/title from first user text.
func (s *Store) EnsureConversation(ctx context.Context, principalID, conversationID, firstUserText string, ws ConversationWorkspaceSnapshot) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("operator store unavailable")
	}
	principalID = strings.TrimSpace(principalID)
	conversationID = strings.TrimSpace(conversationID)
	if principalID == "" || conversationID == "" {
		return fmt.Errorf("principal_id and conversation_id required")
	}
	var exists int
	err := s.db.QueryRowContext(ctx, `SELECT 1 FROM conversations WHERE conversation_id = ? AND principal_id = ?`,
		conversationID, principalID).Scan(&exists)
	if err == nil {
		return nil
	}
	if err != sql.ErrNoRows {
		return err
	}
	preview := conversationtitle.FromFirstUserMessage(firstUserText, conversationtitle.PreviewMaxRunes)
	title := conversationtitle.TitleFromFirstUserMessage(firstUserText)
	now := s.nowRFC3339()
	var rowID any
	if ws.RowID != nil && *ws.RowID > 0 {
		rowID = *ws.RowID
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO conversations (
	conversation_id, principal_id, title, preview_text, flagged,
	workspace_project_id, workspace_flavor_id, workspace_row_id,
	created_at, updated_at
) VALUES (?,?,?,?,0,?,?,?,?,?)`,
		conversationID, principalID, nullIfEmpty(title), preview,
		strings.TrimSpace(ws.ProjectID), strings.TrimSpace(ws.FlavorID), rowID, now, now)
	return err
}

func nullIfEmpty(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}

func (s *Store) nextTurnIndex(ctx context.Context, conversationID string) (int, error) {
	var max sql.NullInt64
	err := s.db.QueryRowContext(ctx, `
SELECT MAX(turn_index) FROM conversation_turns WHERE conversation_id = ?`, conversationID).Scan(&max)
	if err != nil {
		return 0, err
	}
	if !max.Valid {
		return 0, nil
	}
	return int(max.Int64) + 1, nil
}

// AppendTurn appends one turn and bumps conversation updated_at.
func (s *Store) AppendTurn(ctx context.Context, principalID, conversationID string, in AppendTurnInput) (turnID string, err error) {
	if s == nil || s.db == nil {
		return "", fmt.Errorf("operator store unavailable")
	}
	role := strings.TrimSpace(in.Role)
	if role != "user" && role != "assistant" && role != "error" {
		return "", fmt.Errorf("invalid role %q", role)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()

	var owner string
	err = tx.QueryRowContext(ctx, `SELECT principal_id FROM conversations WHERE conversation_id = ?`, conversationID).Scan(&owner)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("conversation not found")
	}
	if err != nil {
		return "", err
	}
	if owner != strings.TrimSpace(principalID) {
		return "", fmt.Errorf("conversation not found")
	}

	turnID, err = newTurnID()
	if err != nil {
		return "", err
	}
	idx, err := s.nextTurnIndexTx(ctx, tx, conversationID)
	if err != nil {
		return "", err
	}
	now := s.nowRFC3339()
	_, err = tx.ExecContext(ctx, `
INSERT INTO conversation_turns (
	turn_id, conversation_id, turn_index, role, content,
	selected_model, resolved_model, error_detail, retry_user_text,
	prompt_tokens, completion_tokens, total_tokens, created_at
) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		turnID, conversationID, idx, role, in.Content,
		strings.TrimSpace(in.SelectedModel), strings.TrimSpace(in.ResolvedModel),
		strings.TrimSpace(in.ErrorDetail), strings.TrimSpace(in.RetryUserText),
		nullInt(in.PromptTokens), nullInt(in.CompletionTokens), nullInt(in.TotalTokens), now)
	if err != nil {
		return "", err
	}
	_, err = tx.ExecContext(ctx, `UPDATE conversations SET updated_at = ? WHERE conversation_id = ? AND principal_id = ?`,
		now, conversationID, principalID)
	if err != nil {
		return "", err
	}
	return turnID, tx.Commit()
}

func (s *Store) nextTurnIndexTx(ctx context.Context, tx *sql.Tx, conversationID string) (int, error) {
	var max sql.NullInt64
	err := tx.QueryRowContext(ctx, `
SELECT MAX(turn_index) FROM conversation_turns WHERE conversation_id = ?`, conversationID).Scan(&max)
	if err != nil {
		return 0, err
	}
	if !max.Valid {
		return 0, nil
	}
	return int(max.Int64) + 1, nil
}

func nullInt(v *int) any {
	if v == nil {
		return nil
	}
	return *v
}

// ReplaceTurnRetrievals replaces all retrieval rows for a turn.
func (s *Store) ReplaceTurnRetrievals(ctx context.Context, turnID string, hits []RetrievalInput) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("operator store unavailable")
	}
	turnID = strings.TrimSpace(turnID)
	if turnID == "" {
		return fmt.Errorf("turn_id required")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM conversation_retrievals WHERE turn_id = ?`, turnID); err != nil {
		return err
	}
	for i, h := range hits {
		rid, err := newRetrievalID()
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `
INSERT INTO conversation_retrievals (
	retrieval_id, turn_id, sort_order, file_path, score, snippet_text, language,
	vector_point_id, content_sha256, start_line, end_line, starts_mid_line
) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
			rid, turnID, i, strings.TrimSpace(h.FilePath), h.Score,
			h.SnippetText, strings.TrimSpace(h.Language),
			strings.TrimSpace(h.VectorPointID), strings.TrimSpace(h.ContentSHA256),
			h.StartLine, h.EndLine, boolToInt(h.StartsMidLine))
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ListConversations returns conversation summaries for a principal.
func (s *Store) ListConversations(ctx context.Context, principalID string, f ListConversationsFilter) ([]ConversationSummary, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("operator store unavailable")
	}
	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	offset := f.Offset
	if offset < 0 {
		offset = 0
	}
	q := `
SELECT conversation_id, principal_id, title, preview_text, flagged,
	workspace_project_id, workspace_flavor_id, workspace_row_id, created_at, updated_at
FROM conversations
WHERE principal_id = ?`
	args := []any{strings.TrimSpace(principalID)}
	if f.FlaggedOnly {
		q += ` AND flagged = 1`
	}
	q += ` ORDER BY updated_at DESC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ConversationSummary
	for rows.Next() {
		var c ConversationSummary
		var title sql.NullString
		var flagged int
		var rowID sql.NullInt64
		var ca, ua string
		if err := rows.Scan(&c.ConversationID, &c.PrincipalID, &title, &c.PreviewText, &flagged,
			&c.WorkspaceProjectID, &c.WorkspaceFlavorID, &rowID, &ca, &ua); err != nil {
			return nil, err
		}
		c.Title = title
		c.Flagged = flagged == 1
		c.WorkspaceRowID = rowID
		c.CreatedAt = parseTime(ca)
		c.UpdatedAt = parseTime(ua)
		out = append(out, c)
	}
	return out, rows.Err()
}

// GetConversationTranscript loads one conversation with ordered turns and retrievals.
func (s *Store) GetConversationTranscript(ctx context.Context, principalID, conversationID string) (*ConversationTranscript, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("operator store unavailable")
	}
	var sum ConversationSummary
	var title sql.NullString
	var flagged int
	var rowID sql.NullInt64
	var ca, ua string
	err := s.db.QueryRowContext(ctx, `
SELECT conversation_id, principal_id, title, preview_text, flagged,
	workspace_project_id, workspace_flavor_id, workspace_row_id, created_at, updated_at
FROM conversations WHERE conversation_id = ? AND principal_id = ?`,
		conversationID, strings.TrimSpace(principalID)).Scan(
		&sum.ConversationID, &sum.PrincipalID, &title, &sum.PreviewText, &flagged,
		&sum.WorkspaceProjectID, &sum.WorkspaceFlavorID, &rowID, &ca, &ua)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	sum.Title = title
	sum.Flagged = flagged == 1
	sum.WorkspaceRowID = rowID
	sum.CreatedAt = parseTime(ca)
	sum.UpdatedAt = parseTime(ua)

	rows, err := s.db.QueryContext(ctx, `
SELECT turn_id, conversation_id, turn_index, role, content,
	selected_model, resolved_model, error_detail, retry_user_text,
	prompt_tokens, completion_tokens, total_tokens, created_at
FROM conversation_turns
WHERE conversation_id = ?
ORDER BY turn_index ASC, role ASC`, conversationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var turns []ConversationTurn
	for rows.Next() {
		var t ConversationTurn
		var pt, ct, tot sql.NullInt64
		var tca string
		if err := rows.Scan(&t.TurnID, &t.ConversationID, &t.TurnIndex, &t.Role, &t.Content,
			&t.SelectedModel, &t.ResolvedModel, &t.ErrorDetail, &t.RetryUserText,
			&pt, &ct, &tot, &tca); err != nil {
			return nil, err
		}
		t.PromptTokens = pt
		t.CompletionTokens = ct
		t.TotalTokens = tot
		t.CreatedAt = parseTime(tca)
		turns = append(turns, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range turns {
		retr, err := s.listRetrievalsForTurn(ctx, turns[i].TurnID)
		if err != nil {
			return nil, err
		}
		turns[i].Retrievals = retr
	}
	return &ConversationTranscript{Summary: sum, Turns: turns}, nil
}

func (s *Store) listRetrievalsForTurn(ctx context.Context, turnID string) ([]ConversationRetrieval, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT retrieval_id, turn_id, sort_order, file_path, score, snippet_text, language,
	vector_point_id, content_sha256, start_line, end_line, starts_mid_line
FROM conversation_retrievals
WHERE turn_id = ?
ORDER BY sort_order ASC`, turnID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ConversationRetrieval
	for rows.Next() {
		var r ConversationRetrieval
		var mid int
		if err := rows.Scan(&r.RetrievalID, &r.TurnID, &r.SortOrder, &r.FilePath, &r.Score,
			&r.SnippetText, &r.Language, &r.VectorPointID, &r.ContentSHA256,
			&r.StartLine, &r.EndLine, &mid); err != nil {
			return nil, err
		}
		r.StartsMidLine = mid == 1
		out = append(out, r)
	}
	return out, rows.Err()
}

// UpdateConversationTitle sets a custom title (non-empty).
func (s *Store) UpdateConversationTitle(ctx context.Context, principalID, conversationID, title string) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("operator store unavailable")
	}
	title = strings.TrimSpace(title)
	if title == "" {
		return fmt.Errorf("title required")
	}
	now := s.nowRFC3339()
	res, err := s.db.ExecContext(ctx, `
UPDATE conversations SET title = ?, updated_at = ?
WHERE conversation_id = ? AND principal_id = ?`, title, now, conversationID, strings.TrimSpace(principalID))
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("conversation not found")
	}
	return nil
}

// SetConversationFlagged toggles the flagged bookmark bit.
func (s *Store) SetConversationFlagged(ctx context.Context, principalID, conversationID string, flagged bool) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("operator store unavailable")
	}
	v := 0
	if flagged {
		v = 1
	}
	now := s.nowRFC3339()
	res, err := s.db.ExecContext(ctx, `
UPDATE conversations SET flagged = ?, updated_at = ?
WHERE conversation_id = ? AND principal_id = ?`, v, now, conversationID, strings.TrimSpace(principalID))
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("conversation not found")
	}
	return nil
}

// DeleteConversation hard-deletes a conversation (cascade turns/retrievals).
func (s *Store) DeleteConversation(ctx context.Context, principalID, conversationID string) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("operator store unavailable")
	}
	res, err := s.db.ExecContext(ctx, `
DELETE FROM conversations WHERE conversation_id = ? AND principal_id = ?`,
		conversationID, strings.TrimSpace(principalID))
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("conversation not found")
	}
	return nil
}
