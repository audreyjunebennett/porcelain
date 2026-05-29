package conversationtitle

import (
	"strings"
	"testing"
)

func TestFromFirstUserMessage_truncatesLong(t *testing.T) {
	long := strings.Repeat("a", PreviewMaxRunes+10)
	got := FromFirstUserMessage(long, PreviewMaxRunes)
	if got == "" {
		t.Fatalf("got empty")
	}
	if !stringsHasSuffix(got, "...") {
		t.Fatalf("expected ellipsis, got=%q", got)
	}
	if len([]rune(got)) != PreviewMaxRunes+3 {
		t.Fatalf("want %d runes, got %d %q", PreviewMaxRunes+3, len([]rune(got)), got)
	}
}

func TestFromFirstUserMessage_collapsesNewlines(t *testing.T) {
	got := FromFirstUserMessage("line one\nline two", PreviewMaxRunes)
	if got != "line one line two" {
		t.Fatalf("got=%q", got)
	}
}

func TestFromFirstUserMessage_empty(t *testing.T) {
	if FromFirstUserMessage("  \n  ", PreviewMaxRunes) != "" {
		t.Fatal("expected empty")
	}
}

func TestTitleFromFirstUserMessage_stopsAtFirstPunctuation(t *testing.T) {
	got := TitleFromFirstUserMessage("Do I use plan document templates, or something else?")
	if got != "Do I use plan document templates," {
		t.Fatalf("got=%q", got)
	}
}

func TestTitleFromFirstUserMessage_truncatesAtMaxRunes(t *testing.T) {
	long := strings.Repeat("a", 100)
	got := TitleFromFirstUserMessage(long)
	if !stringsHasSuffix(got, "...") {
		t.Fatalf("expected ellipsis, got=%q", got)
	}
	if len([]rune(got)) != TitleMaxRunes+3 {
		t.Fatalf("want %d runes, got %d %q", TitleMaxRunes+3, len([]rune(got)), got)
	}
}

func TestTitleFromFirstUserMessage_punctuationAfterMaxRunes(t *testing.T) {
	text := strings.Repeat("a", TitleMaxRunes) + "!"
	got := TitleFromFirstUserMessage(text)
	if !stringsHasSuffix(got, "...") {
		t.Fatalf("expected ellipsis, got=%q", got)
	}
	if len([]rune(got)) != TitleMaxRunes+3 {
		t.Fatalf("want %d runes, got %d %q", TitleMaxRunes+3, len([]rune(got)), got)
	}
}

func TestTitleFromFirstUserMessage_shortWithoutPunctuation(t *testing.T) {
	got := TitleFromFirstUserMessage("hello world")
	if got != "hello world" {
		t.Fatalf("got=%q", got)
	}
}

func stringsHasSuffix(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}
