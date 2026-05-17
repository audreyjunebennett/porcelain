package rag

import (
	"sort"
	"strings"

	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/vectorstore"
)

// HitsBySourceCount groups retrieval hits by Payload.Source; empty sources
// are counted as "unknown".
func HitsBySourceCount(hits []vectorstore.Hit) map[string]int {
	m := make(map[string]int)
	for _, h := range hits {
		src := strings.TrimSpace(h.Payload.Source)
		if src == "" {
			src = "unknown"
		}
		m[src]++
	}
	return m
}

// SortedSources returns map keys sorted lexicographically for stable log order.
func SortedSources(counts map[string]int) []string {
	out := make([]string, 0, len(counts))
	for s := range counts {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}
