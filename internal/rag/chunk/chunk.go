// Package chunk implements server-side text chunking for v0.2 ingest.
//
// Defaults: 512 UTF-8 code units (runes) per chunk with 128-rune overlap, per
// docs/plans/version-v0.2.md. The implementation walks runes (not bytes) so that
// multibyte code points are not split mid-character.
package chunk

import (
	"strings"
	"unicode/utf8"
)

// Chunk is a slice of the input text plus its character span.
type Chunk struct {
	Index   int
	Text    string
	StartCh int // inclusive (rune index)
	EndCh   int // exclusive (rune index)
}

// Split returns chunks for s using rune-based size + overlap. When size <= 0
// or s is empty/blank, Split returns a single chunk containing the trimmed
// text (or no chunks if completely blank).
func Split(s string, size, overlap int) []Chunk {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	if size <= 0 {
		return []Chunk{{Index: 0, Text: s, StartCh: 0, EndCh: utf8.RuneCountInString(s)}}
	}
	if overlap < 0 {
		overlap = 0
	}
	if overlap >= size {
		overlap = size / 4
	}
	step := size - overlap
	if step <= 0 {
		step = size
	}

	runes := []rune(s)
	n := len(runes)
	if n <= size {
		return []Chunk{{Index: 0, Text: string(runes), StartCh: 0, EndCh: n}}
	}
	out := make([]Chunk, 0, (n/step)+1)
	idx := 0
	for start := 0; start < n; start += step {
		end := start + size
		if end > n {
			end = n
		}
		out = append(out, Chunk{
			Index:   idx,
			Text:    string(runes[start:end]),
			StartCh: start,
			EndCh:   end,
		})
		idx++
		if end == n {
			break
		}
	}
	return out
}
