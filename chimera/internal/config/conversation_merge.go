package config

// ConversationMerge configures semantic merging of chat sessions that share the same
// tenant + RAG scope (project/flavor) when the client does not send X-Chimera-Conversation-Id.
type ConversationMerge struct {
	Enabled bool

	// MatchThreshold is the minimum combined score (0–1) to reuse an existing conversation_id.
	MatchThreshold float64

	// RecentWindowMinutes adds up to 0.2 to the score when the candidate was updated within this window.
	RecentWindowMinutes int

	// CandidateLimit caps rows scanned per request for matching.
	CandidateLimit int

	// MaxIdleHours excludes candidates older than now−MaxIdleHours from matching (0 = no age filter).
	MaxIdleHours float64

	// SessionAttachMinutes: if >0 and semantic score is below MatchThreshold, still reuse the
	// most recently active conversation in scope when the new turn is soon enough and either
	// cosine similarity to that turn exceeds SessionAttachMinCosine or the user message is a
	// short follow-up (≤ SessionShortFollowUpRunes). 0 disables this behavior.
	SessionAttachMinutes int
	// SessionAttachMinCosine is the minimum cosine (0–1) vs the latest session for sticky attach.
	SessionAttachMinCosine float64
	// SessionShortFollowUpRunes treats user text no longer than this many runes as a follow-up
	// for sticky attach (0 disables the short-message shortcut).
	SessionShortFollowUpRunes int
}

const (
	defaultConversationMergeThreshold            = 0.75
	defaultConversationMergeRecentMinutes        = 10
	defaultConversationMergeCandidateLimit       = 32
	defaultConversationMergeMaxIdleHours         = 168 // 7 days
	defaultConversationMergeSessionAttachMinutes = 45
	defaultSessionAttachMinCosine                = 0.18
	defaultSessionShortFollowUpRunes             = 220
)

func conversationMergeEffective(doc conversationMergeDoc) ConversationMerge {
	out := ConversationMerge{
		Enabled:                   doc.Enabled,
		MatchThreshold:            doc.MatchThreshold,
		RecentWindowMinutes:       doc.RecentWindowMinutes,
		CandidateLimit:            doc.CandidateLimit,
		MaxIdleHours:              doc.MaxIdleHours,
		SessionAttachMinutes:      0,
		SessionAttachMinCosine:    0,
		SessionShortFollowUpRunes: 0,
	}
	if doc.SessionAttachMinutes != nil {
		out.SessionAttachMinutes = *doc.SessionAttachMinutes
	} else if out.Enabled {
		out.SessionAttachMinutes = defaultConversationMergeSessionAttachMinutes
	}
	if doc.SessionAttachMinCosine != nil {
		out.SessionAttachMinCosine = *doc.SessionAttachMinCosine
	} else if out.Enabled {
		out.SessionAttachMinCosine = defaultSessionAttachMinCosine
	}
	if doc.SessionShortFollowUpRunes != nil {
		out.SessionShortFollowUpRunes = *doc.SessionShortFollowUpRunes
	} else if out.Enabled {
		out.SessionShortFollowUpRunes = defaultSessionShortFollowUpRunes
	}
	if out.MatchThreshold <= 0 {
		out.MatchThreshold = defaultConversationMergeThreshold
	}
	if out.MatchThreshold > 1 {
		out.MatchThreshold = 1
	}
	if out.RecentWindowMinutes <= 0 {
		out.RecentWindowMinutes = defaultConversationMergeRecentMinutes
	}
	if out.CandidateLimit <= 0 {
		out.CandidateLimit = defaultConversationMergeCandidateLimit
	}
	if out.MaxIdleHours < 0 {
		out.MaxIdleHours = 0
	}
	if out.MaxIdleHours == 0 {
		out.MaxIdleHours = defaultConversationMergeMaxIdleHours
	}
	return out
}

type conversationMergeDoc struct {
	Enabled                   bool     `yaml:"enabled"`
	MatchThreshold            float64  `yaml:"match_threshold"`
	RecentWindowMinutes       int      `yaml:"recent_window_minutes"`
	CandidateLimit            int      `yaml:"candidate_limit"`
	MaxIdleHours              float64  `yaml:"max_idle_hours"`
	SessionAttachMinutes      *int     `yaml:"session_attach_minutes"`
	SessionAttachMinCosine    *float64 `yaml:"session_attach_min_cosine"`
	SessionShortFollowUpRunes *int     `yaml:"session_short_follow_up_runes"`
}
