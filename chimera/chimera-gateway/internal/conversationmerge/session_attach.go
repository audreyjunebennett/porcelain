package conversationmerge

import (
	"time"
	"unicode/utf8"

	"github.com/lynn/porcelain/chimera/internal/config"
)

// maybeStickyReassign attaches the request to the most recently active conversation in scope
// when semantic score alone would start a new id: short follow-ups ("where is that?") or
// modest cosine similarity within a recent time window.
func maybeStickyReassign(cfg config.ConversationMerge, candidates []CandidateRow, now time.Time, vec []float32, lastUser string, bestID string, bestScore float64) (string, float64) {
	if cfg.SessionAttachMinutes <= 0 || len(candidates) == 0 {
		return bestID, bestScore
	}
	if bestScore >= cfg.MatchThreshold {
		return bestID, bestScore
	}
	mr := candidates[0] // ListCandidates is ordered by last_updated DESC
	if now.Sub(mr.LastUpdated) > time.Duration(cfg.SessionAttachMinutes)*time.Minute {
		return bestID, bestScore
	}
	if len(mr.LastUserEmbedding) != len(vec) {
		return bestID, bestScore
	}
	cosMR := CosineSimilarity(vec, mr.LastUserEmbedding)
	short := cfg.SessionShortFollowUpRunes > 0 && utf8.RuneCountInString(lastUser) <= cfg.SessionShortFollowUpRunes
	if cosMR >= cfg.SessionAttachMinCosine || short {
		return mr.ConversationID, cfg.MatchThreshold
	}
	return bestID, bestScore
}
