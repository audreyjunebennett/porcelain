package chunk

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestNormalizeNewlines(t *testing.T) {
	got := NormalizeNewlines("a\r\nb\rc\n")
	want := "a\nb\nc\n"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestSplit_EmptyAndBlank(t *testing.T) {
	if got := Split("", 10, 2); got != nil {
		t.Fatalf("expected nil for empty, got %v", got)
	}
	if got := Split("   \n  ", 10, 2); got != nil {
		t.Fatalf("expected nil for blank, got %v", got)
	}
}

func TestSplit_CRLFLineRange(t *testing.T) {
	// "line1\nline2\nline3" with chunk covering line2-line3 region
	s := NormalizeNewlines("line1\r\nline2\r\nline3\r\n")
	segs := Split(s, 512, 128)
	if len(segs) != 1 {
		t.Fatalf("expected one segment, got %d", len(segs))
	}
	if segs[0].StartLine != 1 || segs[0].EndLine < 1 {
		t.Fatalf("lines: %+v", segs[0])
	}
}

func TestSplit_MidLineStart(t *testing.T) {
	// Force small chunks on multi-line text.
	lines := strings.Repeat("x", 40) + "\n" + strings.Repeat("y", 40) + "\n"
	s := NormalizeNewlines(lines)
	segs := Split(s, 50, 10)
	if len(segs) < 2 {
		t.Fatalf("expected multiple segments, got %d", len(segs))
	}
	foundMid := false
	for _, seg := range segs[1:] {
		if seg.StartsMidLine {
			foundMid = true
			break
		}
	}
	if !foundMid {
		t.Fatalf("expected a mid-line start in later segments: %+v", segs)
	}
}

func TestSplit_RuneBoundaryNotSplit(t *testing.T) {
	rune4 := "𝄞"
	s := strings.Repeat(rune4, 200)
	segs := Split(s, 50, 10)
	for _, c := range segs {
		if !utf8.ValidString(c.Text) {
			t.Fatalf("invalid utf-8: %q", c.Text)
		}
	}
}

func TestSplit_L42SpanFixture(t *testing.T) {
	var b strings.Builder
	for i := 1; i <= 60; i++ {
		b.WriteString(strings.Repeat("x", 20))
		b.WriteByte('\n')
	}
	s := b.String()
	segs := Split(s, 4096, 128)
	if len(segs) != 1 {
		t.Fatalf("expected one segment for 60 short lines, got %d", len(segs))
	}
	if segs[0].StartLine != 1 || segs[0].EndLine != 60 {
		t.Fatalf("span L%d–%d want L1–60", segs[0].StartLine, segs[0].EndLine)
	}
}
