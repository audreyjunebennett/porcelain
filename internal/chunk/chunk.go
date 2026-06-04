// Package chunk splits normalized UTF-8 text into rune-based segments with line and byte spans.
package chunk

import (
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// Schema version for manifest ingest (gateway Qdrant payload chunk_schema).
const SchemaV2 = 2

// Segment is one chunk with display-oriented line/byte metadata.
type Segment struct {
	Index         int
	Text          string
	StartCh       int // inclusive rune index in normalized text
	EndCh         int // exclusive rune index
	StartLine     int // 1-based
	EndLine       int // 1-based, line of last character
	StartByte     int // UTF-8 byte offset in normalized text
	EndByte       int // exclusive
	StartsMidLine bool
	Language      string
}

// NormalizeNewlines converts CRLF and lone CR to LF for platform-independent ingest.
func NormalizeNewlines(s string) string {
	if s == "" {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\r':
			if i+1 < len(s) && s[i+1] == '\n' {
				i++
			}
			b.WriteByte('\n')
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

// LanguageFromPath returns a short language tag from a root-relative path extension.
func LanguageFromPath(source string) string {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(source), "."))
	switch ext {
	case "go":
		return "go"
	case "js", "mjs", "cjs":
		return "javascript"
	case "ts", "tsx", "mts", "cts":
		return "typescript"
	case "py", "pyw":
		return "python"
	case "rs":
		return "rust"
	case "java":
		return "java"
	case "cs":
		return "csharp"
	case "cpp", "cc", "cxx", "hpp", "h":
		return "cpp"
	case "c":
		return "c"
	case "rb":
		return "ruby"
	case "php":
		return "php"
	case "swift":
		return "swift"
	case "kt", "kts":
		return "kotlin"
	case "sql":
		return "sql"
	case "sh", "bash", "zsh":
		return "shell"
	case "yaml", "yml":
		return "yaml"
	case "json":
		return "json"
	case "md", "markdown":
		return "markdown"
	case "html", "htm":
		return "html"
	case "css":
		return "css"
	default:
		return ""
	}
}

// Split returns annotated segments for normalized s using rune-based size and overlap.
func Split(normalized string, size, overlap int) []Segment {
	if strings.TrimSpace(normalized) == "" {
		return nil
	}
	if size <= 0 {
		n := utf8.RuneCountInString(normalized)
		return []Segment{{
			Index: 0, Text: normalized, StartCh: 0, EndCh: n,
			StartLine: 1, EndLine: lineOfByte(normalized, len(normalized)-1, lineStarts(normalized)),
			StartByte: 0, EndByte: len(normalized),
		}}
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

	runes := []rune(normalized)
	n := len(runes)
	lines := lineStarts(normalized)
	lang := ""

	if n <= size {
		endB := byteOffsetForRuneStart(normalized, n)
		lastB := endB - 1
		if lastB < 0 {
			lastB = 0
		}
		return []Segment{{
			Index: 0, Text: string(runes), StartCh: 0, EndCh: n,
			StartLine: 1, EndLine: lineOfByte(normalized, lastB, lines),
			StartByte: 0, EndByte: endB,
		}}
	}

	out := make([]Segment, 0, (n/step)+1)
	idx := 0
	for start := 0; start < n; start += step {
		end := start + size
		if end > n {
			end = n
		}
		text := string(runes[start:end])
		startB := byteOffsetForRuneStart(normalized, start)
		endB := byteOffsetForRuneStart(normalized, end)
		startLine := lineOfByte(normalized, startB, lines)
		lastB := endB - 1
		if lastB < startB {
			lastB = startB
		}
		endLine := lineOfByte(normalized, lastB, lines)
		out = append(out, Segment{
			Index:         idx,
			Text:          text,
			StartCh:       start,
			EndCh:         end,
			StartLine:     startLine,
			EndLine:       endLine,
			StartByte:     startB,
			EndByte:       endB,
			StartsMidLine: startB > lines[startLine-1],
			Language:      lang,
		})
		idx++
		if end == n {
			break
		}
	}
	return out
}

func lineStarts(text string) []int {
	starts := []int{0}
	for i := 0; i < len(text); i++ {
		if text[i] == '\n' && i+1 < len(text) {
			starts = append(starts, i+1)
		}
	}
	return starts
}

func lineOfByte(text string, b int, starts []int) int {
	if b < 0 {
		b = 0
	}
	if b >= len(text) {
		b = len(text) - 1
	}
	if len(text) == 0 {
		return 1
	}
	line := 1
	for i := len(starts) - 1; i >= 0; i-- {
		if starts[i] <= b {
			return i + 1
		}
	}
	return line
}

func byteOffsetForRuneStart(s string, runeIndex int) int {
	if runeIndex <= 0 {
		return 0
	}
	i := 0
	for pos := range s {
		if i == runeIndex {
			return pos
		}
		i++
	}
	return len(s)
}

