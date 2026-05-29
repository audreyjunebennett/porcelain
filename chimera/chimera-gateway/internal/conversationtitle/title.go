package conversationtitle

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	// PreviewMaxRunes is the stored preview_text excerpt limit.
	PreviewMaxRunes = 512
	// TitleMaxRunes is the auto-generated title length (single-line excerpt).
	TitleMaxRunes = 80
	// EditMaxRunes is the maximum operator-edited title length.
	EditMaxRunes = 256
)

// TitleFromFirstUserMessage returns an auto-generated title: up to TitleMaxRunes
// runes, or through the first punctuation mark (inclusive), whichever is shorter.
func TitleFromFirstUserMessage(text string) string {
	return excerptFromFirstUserMessage(text, TitleMaxRunes, true)
}

// FromFirstUserMessage returns a single-line excerpt suitable for previews.
func FromFirstUserMessage(text string, maxRunes int) string {
	return excerptFromFirstUserMessage(text, maxRunes, false)
}

func excerptFromFirstUserMessage(text string, maxRunes int, stopAtFirstPunct bool) string {
	if maxRunes <= 0 {
		return ""
	}
	s := normalizeFirstUserMessage(text)
	if s == "" {
		return ""
	}

	limit := maxRunes
	punctEnd := 0
	if stopAtFirstPunct {
		punctEnd = indexThroughFirstPunctuation(s)
		if punctEnd > 0 && punctEnd < limit {
			limit = punctEnd
		}
	}

	runeCount := utf8.RuneCountInString(s)
	if runeCount <= limit {
		return s
	}

	out := strings.TrimSpace(truncateRunes(s, limit))
	if out == "" {
		return "..."
	}
	if stopAtFirstPunct && punctEnd > 0 && limit == punctEnd {
		return out
	}
	return out + "..."
}

func normalizeFirstUserMessage(text string) string {
	s := strings.TrimSpace(text)
	if s == "" {
		return ""
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = strings.Join(strings.Fields(strings.ReplaceAll(s, "\n", " ")), " ")
	return strings.TrimSpace(s)
}

func indexThroughFirstPunctuation(s string) int {
	n := 0
	for _, r := range s {
		n++
		if unicode.IsPunct(r) {
			return n
		}
	}
	return 0
}

func truncateRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	n := 0
	for i := range s {
		if n == max {
			return s[:i]
		}
		n++
	}
	return s
}
