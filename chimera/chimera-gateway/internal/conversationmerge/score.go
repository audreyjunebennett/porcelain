package conversationmerge

import "math"

// CosineSimilarity returns cosine similarity in [0,1] for non-negative typical embeddings;
// for arbitrary embeddings the result is clamped to [0,1].
func CosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	if na == 0 || nb == 0 {
		return 0
	}
	v := dot / (math.Sqrt(na) * math.Sqrt(nb))
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// MatchScore combines semantic similarity, lexical overlap, and recency (copilot weights).
func MatchScore(cosineSim, wordJaccard, recentBonus float64) float64 {
	return 0.6*cosineSim + 0.2*wordJaccard + 0.2*recentBonus
}
