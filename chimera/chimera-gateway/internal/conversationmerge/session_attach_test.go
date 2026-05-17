package conversationmerge

import (
	"testing"
	"time"

	"github.com/lynn/porcelain/chimera/internal/config"
)

func TestMaybeStickyReassign_shortFollowUp(t *testing.T) {
	cfg := config.ConversationMerge{
		MatchThreshold:            0.75,
		SessionAttachMinutes:      60,
		SessionAttachMinCosine:    0.99,
		SessionShortFollowUpRunes: 50,
	}
	now := time.Unix(1000, 0)
	vec := []float32{1, 0, 0}
	candidates := []CandidateRow{
		{
			ConversationID:         "prev",
			LastUserEmbedding:      []float32{0, 1, 0},
			LastUpdated:            now.Add(-2 * time.Minute),
			LastUserTextNormalized: "something long and different",
		},
	}
	bid, bs := maybeStickyReassign(cfg, candidates, now, vec, "ok?", "", 0.1)
	if bid != "prev" || bs < cfg.MatchThreshold {
		t.Fatalf("got %q score %v", bid, bs)
	}
}

func TestMaybeStickyReassign_cosinePass(t *testing.T) {
	cfg := config.ConversationMerge{
		MatchThreshold:            0.75,
		SessionAttachMinutes:      60,
		SessionAttachMinCosine:    0.2,
		SessionShortFollowUpRunes: 0,
	}
	now := time.Unix(2000, 0)
	v := []float32{0.6, 0.8, 0}
	candidates := []CandidateRow{
		{
			ConversationID:         "x",
			LastUserEmbedding:      []float32{0.5, 0.9, 0},
			LastUpdated:            now.Add(-1 * time.Minute),
			LastUserTextNormalized: "prior",
		},
	}
	bid, bs := maybeStickyReassign(cfg, candidates, now, v, "long message that is not short", "", 0.2)
	if bid != "x" {
		t.Fatalf("got %q", bid)
	}
	if bs < cfg.MatchThreshold {
		t.Fatalf("score %v", bs)
	}
}

func TestMaybeStickyReassign_tooOld(t *testing.T) {
	cfg := config.ConversationMerge{
		MatchThreshold:            0.75,
		SessionAttachMinutes:      5,
		SessionAttachMinCosine:    0.01,
		SessionShortFollowUpRunes: 500,
	}
	now := time.Unix(3000, 0)
	vec := []float32{1, 0, 0}
	candidates := []CandidateRow{
		{
			ConversationID:         "old",
			LastUserEmbedding:      vec,
			LastUpdated:            now.Add(-30 * time.Minute),
			LastUserTextNormalized: "x",
		},
	}
	bid, bs := maybeStickyReassign(cfg, candidates, now, vec, "hi", "", 0.1)
	if bid != "" {
		t.Fatalf("expected no sticky, got %q", bid)
	}
	if bs != 0.1 {
		t.Fatalf("score %v", bs)
	}
}
