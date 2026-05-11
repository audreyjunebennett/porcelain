package conversationmerge

import (
	"context"
	"database/sql"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lynn/claudia-gateway/internal/config"
	"github.com/lynn/claudia-gateway/internal/rag/embed"
)

const dedupRetention = 24 * time.Hour

// mergeCorrelationAttrs returns structured log fields for gateway chat correlation
// (request_id, conversation_id, principal_id, service) when values are non-empty.
func mergeCorrelationAttrs(in ResolveInput, conversationID string) []any {
	out := []any{"service", "gateway"}
	if in.RequestID != "" {
		out = append(out, "request_id", in.RequestID)
	}
	if in.TenantID != "" {
		out = append(out, "principal_id", in.TenantID)
	}
	if conversationID != "" {
		out = append(out, "conversation_id", conversationID)
	}
	return out
}

// Service performs semantic conversation resolution and persistence.
type Service struct {
	cfg    config.ConversationMerge
	store  *Store
	embed  *embed.Client
	expDim int
	log    *slog.Logger
}

// NewService returns nil when merge is disabled or dependencies are missing.
func NewService(cfg config.ConversationMerge, db *sql.DB, upstreamBaseURL, upstreamAPIKey string, rag config.RAG, log *slog.Logger) *Service {
	if !cfg.Enabled || db == nil {
		return nil
	}
	model := rag.EmbeddingModel
	if model == "" {
		model = "text-embedding-3-small"
	}
	dim := rag.EmbeddingDim
	if dim <= 0 {
		dim = 1536
	}
	url := rag.EmbeddingURL(upstreamBaseURL)
	if url == "" || upstreamAPIKey == "" {
		if log != nil {
			log.Info("conversation merge disabled: missing embedding URL or upstream API key", "msg", "conversation.merge.disabled")
		}
		return nil
	}
	ec := embed.New(url, upstreamAPIKey, model)
	st := NewStore(db)
	if st == nil {
		return nil
	}
	return &Service{
		cfg:    cfg,
		store:  st,
		embed:  ec,
		expDim: dim,
		log:    log,
	}
}

// ResolveInput carries one chat request's correlation scope.
type ResolveInput struct {
	TenantID             string
	ProjectID            string
	FlavorID             string
	LastUserText         string
	IncomingFingerprint  string
	ClientConversationID string // from X-Claudia-Conversation-Id when present (already validated)
	// RequestID is the gateway HTTP request id (Phase 2 correlation for merge logs).
	RequestID string
	// NextTurnIndex, when set, supplies turn_index for lifecycle logs (dedup_hit, merged) and
	// increments the per-conversation turn counter in the gateway runtime.
	NextTurnIndex func(conversationID string) int
}

// ResolveOutcome is the result of semantic resolution.
type ResolveOutcome struct {
	ConversationID string
	DedupJSON      []byte // when non-nil, handler should write this body and skip upstream
	// TurnIndex is the 1-based turn assigned when NextTurnIndex was invoked for merge lifecycle logs (dedup or merged).
	TurnIndex int
}

// Resolve picks a canonical conversation id using embeddings + scoring.
func (s *Service) Resolve(ctx context.Context, in ResolveInput) (ResolveOutcome, error) {
	var out ResolveOutcome
	out.ConversationID = uuid.NewString()

	if s == nil || s.store == nil || s.embed == nil {
		return out, nil
	}

	if in.ClientConversationID != "" {
		out.ConversationID = in.ClientConversationID
		return out, nil
	}

	last := clipUserMessage(in.LastUserText)
	if last == "" {
		return out, nil
	}

	userNorm := Normalize(last)
	vec, err := s.embed.EmbedOne(ctx, last)
	if err != nil {
		if s.log != nil {
			s.log.Warn("conversation merge: embed failed; using fresh conversation id",
				append([]any{"msg", "conversation.merge.embed_failed", "err", err}, mergeCorrelationAttrs(in, out.ConversationID)...)...)
		}
		return out, nil
	}
	if len(vec) != s.expDim {
		if s.log != nil {
			s.log.Warn("conversation merge: embedding dim mismatch; using fresh id",
				append([]any{"msg", "conversation.merge.embed_dim_mismatch",
					"got", len(vec), "want", s.expDim}, mergeCorrelationAttrs(in, out.ConversationID)...)...)
		}
		return out, nil
	}

	now := time.Now()
	minTime := time.Time{}
	if s.cfg.MaxIdleHours > 0 {
		minTime = now.Add(-time.Duration(s.cfg.MaxIdleHours * float64(time.Hour)))
	}

	candidates, err := s.store.ListCandidates(ctx, in.TenantID, in.ProjectID, in.FlavorID, minTime, s.cfg.CandidateLimit)
	if err != nil {
		if s.log != nil {
			s.log.Warn("conversation merge: list candidates failed",
				append([]any{"msg", "conversation.merge.list_candidates_failed", "err", err}, mergeCorrelationAttrs(in, out.ConversationID)...)...)
		}
		return out, nil
	}

	recentCutoff := now.Add(-time.Duration(s.cfg.RecentWindowMinutes) * time.Minute)

	var bestID string
	var bestScore float64 = -1

	for _, c := range candidates {
		if len(c.LastUserEmbedding) != len(vec) {
			continue
		}
		cos := CosineSimilarity(vec, c.LastUserEmbedding)
		jac := WordJaccard(userNorm, c.LastUserTextNormalized)
		recent := 0.0
		if c.LastUpdated.After(recentCutoff) || c.LastUpdated.Equal(recentCutoff) {
			recent = 1.0
		}
		sc := MatchScore(cos, jac, recent)
		if sc > bestScore {
			bestScore = sc
			bestID = c.ConversationID
		}
	}

	preStickyScore := bestScore
	bestID, bestScore = maybeStickyReassign(s.cfg, candidates, now, vec, last, bestID, bestScore)

	if bestID == "" || bestScore < s.cfg.MatchThreshold {
		out.ConversationID = uuid.NewString()
		s.persistUserSnapshot(ctx, in, out.ConversationID, vec, userNorm, now)
		return out, nil
	}

	dk := DedupKey(bestID, in.IncomingFingerprint, userNorm)
	body, hit, err := s.store.GetDedup(ctx, dk)
	if err != nil && s.log != nil {
		cid := out.ConversationID
		if bestID != "" {
			cid = bestID
		}
		s.log.Debug("conversation merge: dedup read failed",
			append([]any{"msg", "conversation.merge.dedup_read_failed", "err", err}, mergeCorrelationAttrs(in, cid)...)...)
	}
	if hit && len(body) > 0 {
		out.ConversationID = bestID
		out.DedupJSON = body
		if in.NextTurnIndex != nil {
			out.TurnIndex = in.NextTurnIndex(bestID)
		}
		if s.log != nil {
			args := append([]any{"msg", "conversation.dedup_hit", "dedup_bytes", len(body)}, mergeCorrelationAttrs(in, bestID)...)
			if out.TurnIndex != 0 {
				args = append(args, "turn_index", out.TurnIndex)
			}
			s.log.Info("conversation dedup hit", args...)
		}
		return out, nil
	}

	mergeReason := "semantic"
	if preStickyScore < s.cfg.MatchThreshold && bestScore >= s.cfg.MatchThreshold {
		mergeReason = "sticky"
	}
	if in.NextTurnIndex != nil {
		out.TurnIndex = in.NextTurnIndex(bestID)
	}
	if s.log != nil {
		args := append([]any{
			"msg", "conversation.merged",
			"match_score", bestScore,
			"candidate_count", len(candidates),
			"merge_reason", mergeReason,
		}, mergeCorrelationAttrs(in, bestID)...)
		if out.TurnIndex != 0 {
			args = append(args, "turn_index", out.TurnIndex)
		}
		s.log.Info("conversation matched", args...)
	}

	out.ConversationID = bestID
	s.persistUserSnapshot(ctx, in, out.ConversationID, vec, userNorm, now)
	return out, nil
}

// RecordTurn updates SQLite after a successful JSON completion (non-streaming).
// requestID is the gateway HTTP request id for structured logs (may be empty in tests).
func (s *Service) RecordTurn(ctx context.Context, tenantID, projectID, flavorID, conversationID string,
	lastUserRaw string, completionJSON []byte, now time.Time, requestID string) string {
	if s == nil || s.store == nil || s.embed == nil {
		return ""
	}
	last := clipUserMessage(lastUserRaw)
	if last == "" || conversationID == "" {
		return ""
	}

	userNorm := Normalize(last)
	vec, err := s.embed.EmbedOne(ctx, last)
	if err != nil || len(vec) != s.expDim {
		return ""
	}

	modelNorm := Normalize(AssistantTextFromCompletionJSON(completionJSON))

	// Load previous fingerprint for chaining (best-effort).
	prevFP := s.store.GetRollingFingerprint(ctx, conversationID)

	fp := RollingFingerprint(prevFP, userNorm, modelNorm)

	if err := s.store.UpsertConversation(ctx, tenantID, projectID, flavorID, conversationID, vec, userNorm, modelNorm, fp, now); err != nil && s.log != nil {
		s.log.Warn("conversation merge: upsert failed",
			append([]any{"msg", "conversation.merge.upsert_failed", "err", err}, mergeCorrelationAttrs(ResolveInput{TenantID: tenantID, RequestID: requestID}, conversationID)...)...)
	}

	dk := DedupKey(conversationID, prevFP, userNorm)
	if err := s.store.PutDedup(ctx, dk, completionJSON, now, dedupRetention); err != nil && s.log != nil {
		s.log.Debug("conversation merge: dedup cache write failed",
			append([]any{"msg", "conversation.merge.dedup_cache_write_failed", "err", err}, mergeCorrelationAttrs(ResolveInput{TenantID: tenantID, RequestID: requestID}, conversationID)...)...)
	}
	return fp
}

const maxUserEmbedBytes = 16 * 1024

// RollingFingerprint returns the current rolling fingerprint for a conversation (for response headers).
func (s *Service) RollingFingerprint(ctx context.Context, conversationID string) string {
	if s == nil || s.store == nil {
		return ""
	}
	return s.store.GetRollingFingerprint(ctx, conversationID)
}

func (s *Service) persistUserSnapshot(ctx context.Context, in ResolveInput, conversationID string, vec []float32, userNorm string, at time.Time) {
	if s == nil || s.store == nil || len(vec) == 0 || conversationID == "" {
		return
	}
	if err := s.store.UpsertUserSnapshotAtResolve(ctx, in.TenantID, in.ProjectID, in.FlavorID, conversationID, vec, userNorm, at); err != nil && s.log != nil {
		s.log.Warn("conversation merge: resolve snapshot upsert failed",
			append([]any{"msg", "conversation.merge.snapshot_upsert_failed", "err", err}, mergeCorrelationAttrs(in, conversationID)...)...)
	}
}

func clipUserMessage(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if len(s) > maxUserEmbedBytes {
		s = s[:maxUserEmbedBytes]
	}
	return strings.TrimSpace(s)
}
