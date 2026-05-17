package conversationmerge

import (
	"context"
	"database/sql"
	"time"
)

// GetRollingFingerprint returns the stored fingerprint before a new turn, or "" if unknown.
func (s *Store) GetRollingFingerprint(ctx context.Context, conversationID string) string {
	if s == nil || s.db == nil || conversationID == "" {
		return ""
	}
	var fp string
	err := s.db.QueryRowContext(ctx,
		`SELECT rolling_fingerprint FROM conversation_context WHERE conversation_id = ?`, conversationID).Scan(&fp)
	if err != nil {
		return ""
	}
	return fp
}

// CandidateRow is one active conversation session for semantic matching.
type CandidateRow struct {
	ConversationID          string
	LastUserEmbedding       []float32
	EmbeddingDim            int
	LastUserTextNormalized  string
	LastModelTextNormalized string
	LastUpdated             time.Time
	RollingFingerprint      string
}

// Store wraps SQLite persistence for conversation merge.
type Store struct {
	db *sql.DB
}

// NewStore returns nil if db is nil.
func NewStore(db *sql.DB) *Store {
	if db == nil {
		return nil
	}
	return &Store{db: db}
}

// ListCandidates returns recent sessions for the same tenant + RAG scope.
func (s *Store) ListCandidates(ctx context.Context, tenantID, projectID, flavorID string, olderThan time.Time, limit int) ([]CandidateRow, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 32
	}
	q := `
SELECT conversation_id, last_user_embedding, embedding_dim,
       last_user_text_normalized, last_model_text_normalized, last_updated_unix, rolling_fingerprint
FROM conversation_context
WHERE tenant_id = ? AND project_id = ? AND flavor_id = ?
  AND last_updated_unix >= ?
ORDER BY last_updated_unix DESC
LIMIT ?`
	rows, err := s.db.QueryContext(ctx, q, tenantID, projectID, flavorID, float64(olderThan.UnixNano())/1e9, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CandidateRow
	for rows.Next() {
		var (
			id, userNorm, modelNorm, fp string
			dim                         int
			blob                        []byte
			updatedUnix                 float64
		)
		if err := rows.Scan(&id, &blob, &dim, &userNorm, &modelNorm, &updatedUnix, &fp); err != nil {
			return nil, err
		}
		vec, err := DecodeEmbedding(dim, blob)
		if err != nil {
			continue
		}
		out = append(out, CandidateRow{
			ConversationID:          id,
			LastUserEmbedding:       vec,
			EmbeddingDim:            dim,
			LastUserTextNormalized:  userNorm,
			LastModelTextNormalized: modelNorm,
			LastUpdated:             time.Unix(0, int64(updatedUnix*1e9)),
			RollingFingerprint:      fp,
		})
	}
	return out, rows.Err()
}

// UpsertUserSnapshotAtResolve records the latest user embedding after merge resolution so that
// streaming completions (which skip RecordTurn) still leave a row for the next request to match.
// On conflict, last_model_text_normalized and rolling_fingerprint are preserved until RecordTurn runs.
func (s *Store) UpsertUserSnapshotAtResolve(ctx context.Context, tenantID, projectID, flavorID, conversationID string,
	embedding []float32, userNorm string, at time.Time) error {
	if s == nil || s.db == nil || conversationID == "" {
		return nil
	}
	dim := len(embedding)
	blob := EncodeEmbedding(embedding)
	ts := float64(at.UnixNano()) / 1e9
	q := `
INSERT INTO conversation_context (
  conversation_id, tenant_id, project_id, flavor_id,
  last_user_embedding, embedding_dim,
  last_user_text_normalized, last_model_text_normalized,
  last_updated_unix, rolling_fingerprint
) VALUES (?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(conversation_id) DO UPDATE SET
  tenant_id = excluded.tenant_id,
  project_id = excluded.project_id,
  flavor_id = excluded.flavor_id,
  last_user_embedding = excluded.last_user_embedding,
  embedding_dim = excluded.embedding_dim,
  last_user_text_normalized = excluded.last_user_text_normalized,
  last_updated_unix = excluded.last_updated_unix,
  last_model_text_normalized = conversation_context.last_model_text_normalized,
  rolling_fingerprint = conversation_context.rolling_fingerprint`
	_, err := s.db.ExecContext(ctx, q,
		conversationID, tenantID, projectID, flavorID,
		blob, dim,
		userNorm, "",
		ts, "",
	)
	return err
}

// UpsertConversation persists rolling state after a completed turn.
func (s *Store) UpsertConversation(ctx context.Context, tenantID, projectID, flavorID, conversationID string,
	embedding []float32, userNorm, modelNorm, fingerprint string, at time.Time) error {
	if s == nil || s.db == nil {
		return nil
	}
	dim := len(embedding)
	blob := EncodeEmbedding(embedding)
	ts := float64(at.UnixNano()) / 1e9
	q := `
INSERT INTO conversation_context (
  conversation_id, tenant_id, project_id, flavor_id,
  last_user_embedding, embedding_dim,
  last_user_text_normalized, last_model_text_normalized,
  last_updated_unix, rolling_fingerprint
) VALUES (?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(conversation_id) DO UPDATE SET
  tenant_id = excluded.tenant_id,
  project_id = excluded.project_id,
  flavor_id = excluded.flavor_id,
  last_user_embedding = excluded.last_user_embedding,
  embedding_dim = excluded.embedding_dim,
  last_user_text_normalized = excluded.last_user_text_normalized,
  last_model_text_normalized = excluded.last_model_text_normalized,
  last_updated_unix = excluded.last_updated_unix,
  rolling_fingerprint = excluded.rolling_fingerprint`
	_, err := s.db.ExecContext(ctx, q,
		conversationID, tenantID, projectID, flavorID,
		blob, dim,
		userNorm, modelNorm,
		ts, fingerprint,
	)
	return err
}

// GetDedup returns a cached JSON body for duplicate request retries.
func (s *Store) GetDedup(ctx context.Context, dedupKey string) ([]byte, bool, error) {
	if s == nil || s.db == nil || dedupKey == "" {
		return nil, false, nil
	}
	var blob []byte
	err := s.db.QueryRowContext(ctx, `SELECT response_body FROM conversation_dedup_cache WHERE dedup_key = ?`, dedupKey).Scan(&blob)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return blob, true, nil
}

// PutDedup stores a completion payload keyed for deduplication.
func (s *Store) PutDedup(ctx context.Context, dedupKey string, responseJSON []byte, at time.Time, maxRetention time.Duration) error {
	if s == nil || s.db == nil || dedupKey == "" || len(responseJSON) == 0 {
		return nil
	}
	ts := float64(at.UnixNano()) / 1e9
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO conversation_dedup_cache (dedup_key, response_body, created_unix) VALUES (?,?,?)
		 ON CONFLICT(dedup_key) DO UPDATE SET response_body = excluded.response_body, created_unix = excluded.created_unix`,
		dedupKey, responseJSON, ts,
	)
	if err != nil {
		return err
	}
	if maxRetention > 0 {
		cutoff := float64(at.Add(-maxRetention).UnixNano()) / 1e9
		_, _ = s.db.ExecContext(ctx, `DELETE FROM conversation_dedup_cache WHERE created_unix < ?`, cutoff)
	}
	return nil
}
