package chunk

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSplit_EmptyAndBlank(t *testing.T) {
	if got := Split("", 10, 2); got != nil {
		t.Fatalf("expected nil for empty, got %v", got)
	}
	if got := Split("   \n  ", 10, 2); got != nil {
		t.Fatalf("expected nil for blank, got %v", got)
	}
}

func TestSplit_SmallerThanSizeOneChunk(t *testing.T) {
	got := Split("hello world", 100, 10)
	if len(got) != 1 || got[0].Text != "hello world" {
		t.Fatalf("got %v", got)
	}
}

func TestSplit_ChunksWithOverlap(t *testing.T) {
	s := strings.Repeat("a", 1000)
	chunks := Split(s, 512, 128)
	if len(chunks) < 2 {
		t.Fatalf("expected >= 2 chunks, got %d", len(chunks))
	}
	for i, c := range chunks {
		if c.Index != i {
			t.Fatalf("index mismatch: %d vs %d", c.Index, i)
		}
		if utf8.RuneCountInString(c.Text) > 512 {
			t.Fatalf("chunk %d size %d > 512", i, utf8.RuneCountInString(c.Text))
		}
		if i > 0 {
			prev := chunks[i-1]
			if c.StartCh != prev.StartCh+(512-128) {
				t.Fatalf("step incorrect at %d: %d vs %d", i, c.StartCh, prev.StartCh+(512-128))
			}
		}
	}
	last := chunks[len(chunks)-1]
	if last.EndCh != utf8.RuneCountInString(s) {
		t.Fatalf("last chunk end %d != total %d", last.EndCh, utf8.RuneCountInString(s))
	}
}

func TestSplit_RuneBoundaryNotSplit(t *testing.T) {
	// 4-byte runes (musical clef): each rune is one rune unit, multibyte in UTF-8.
	rune4 := "𝄞"
	s := strings.Repeat(rune4, 1000)
	chunks := Split(s, 100, 20)
	for _, c := range chunks {
		if !utf8.ValidString(c.Text) {
			t.Fatalf("chunk text not valid utf-8: %q", c.Text)
		}
	}
}

func TestSplit_OverlapCappedWhenInvalid(t *testing.T) {
	s := strings.Repeat("a", 200)
	// overlap >= size should be coerced to size/4 internally.
	chunks := Split(s, 100, 100)
	if len(chunks) < 2 {
		t.Fatalf("expected progress, got %d chunks", len(chunks))
	}
}
