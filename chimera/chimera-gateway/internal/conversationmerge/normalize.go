package conversationmerge

import (
	"strings"
)

const maxNormalizedRunes = 512

// Normalize collapses whitespace, lowercases, and truncates for stable comparison.
func Normalize(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	fields := strings.Fields(strings.ToLower(s))
	s = strings.Join(fields, " ")
	return truncateRunes(s, maxNormalizedRunes)
}

func truncateRunes(s string, max int) string {
	if max <= 0 || s == "" {
		return ""
	}
	n := 0
	for i := range s {
		if n == max {
			return s[:i] + "…"
		}
		n++
	}
	return s
}

func wordSet(s string) map[string]struct{} {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return nil
	}
	out := make(map[string]struct{})
	for _, f := range strings.Fields(s) {
		if len(f) > 256 {
			f = f[:256]
		}
		out[f] = struct{}{}
	}
	return out
}

// WordJaccard is token overlap in [0,1] (Jaccard index on word sets).
func WordJaccard(a, b string) float64 {
	wa := wordSet(a)
	wb := wordSet(b)
	if len(wa) == 0 && len(wb) == 0 {
		return 1
	}
	if len(wa) == 0 || len(wb) == 0 {
		return 0
	}
	inter := 0
	for w := range wa {
		if _, ok := wb[w]; ok {
			inter++
		}
	}
	union := len(wa) + len(wb) - inter
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}
