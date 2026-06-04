// Package chunk re-exports rune splitting for tests and legacy callers.
// Ingest uses indexer-built manifests; gateway does not split whole files at ingest.
package chunk

import (
	ichunk "github.com/lynn/porcelain/internal/chunk"
)

// Chunk is a slice of the input text plus its character span (legacy shape).
type Chunk struct {
	Index   int
	Text    string
	StartCh int
	EndCh   int
}

// Split returns chunks for s using rune-based size + overlap.
func Split(s string, size, overlap int) []Chunk {
	segs := ichunk.Split(ichunk.NormalizeNewlines(s), size, overlap)
	out := make([]Chunk, len(segs))
	for i, seg := range segs {
		out[i] = Chunk{Index: seg.Index, Text: seg.Text, StartCh: seg.StartCh, EndCh: seg.EndCh}
	}
	return out
}
