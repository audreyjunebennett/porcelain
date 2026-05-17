package freecatalog

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

var stripScript = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)
var stripTags = regexp.MustCompile(`<[^>]+>`)

// StripHTMLToText removes script blocks and tags so markdown-like tables in HTML still match.
func StripHTMLToText(html string) string {
	s := stripScript.ReplaceAllString(html, "\n")
	s = stripTags.ReplaceAllString(s, "\n")
	return s
}

// groqModelRateTableSection narrows parsing to the per-model limits table, not the
// “Rate limit headers” table (which also has pipe rows and numeric second columns).
func groqModelRateTableSection(body string) string {
	t := StripHTMLToText(body)
	low := strings.ToLower(t)
	idx := strings.Index(low, "| model id |")
	if idx < 0 {
		return t
	}
	rest := t[idx:]
	lowRest := strings.ToLower(rest)
	end := strings.Index(lowRest, "rate limit headers")
	if end < 0 {
		return rest
	}
	return rest[:end]
}

func isMarkdownSeparatorRow(cells []string) bool {
	if len(cells) == 0 {
		return false
	}
	for _, c := range cells {
		t := strings.TrimSpace(c)
		if t == "" {
			continue
		}
		if strings.Trim(t, "-") == "" {
			continue
		}
		return false
	}
	return true
}

func isPlausibleGroqModelID(id string) bool {
	if id == "" {
		return false
	}
	r0, _ := utf8.DecodeRuneInString(id)
	if r0 == utf8.RuneError {
		return false
	}
	if !unicode.IsLetter(r0) && !unicode.IsDigit(r0) {
		return false
	}
	for _, r := range id {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == '/' || r == '_' || r == '-' {
			continue
		}
		return false
	}
	return true
}

// isGroqRPMCell requires the RPM column to look like a doc limit (excludes “Header | Value” rows).
func isGroqRPMCell(s string) bool {
	s = strings.TrimSpace(s)
	if s == "-" {
		return true
	}
	if s == "" {
		return false
	}
	r0, _ := utf8.DecodeRuneInString(s)
	return unicode.IsDigit(r0)
}

// groqRowFromCells validates a model table row (markdown or HTML-derived cells).
// minCols is the minimum number of cells (markdown tables may have 3; live HTML uses 7).
func groqRowFromCells(cells []string, minCols int) (GroqDocRow, bool) {
	if minCols <= 0 {
		minCols = 3
	}
	if len(cells) < minCols {
		return GroqDocRow{}, false
	}
	for i := range cells {
		cells[i] = strings.TrimSpace(cells[i])
	}
	id := cells[0]
	if id == "" || strings.EqualFold(id, "MODEL ID") || strings.EqualFold(id, "MODEL") {
		return GroqDocRow{}, false
	}
	if isMarkdownSeparatorRow(cells) {
		return GroqDocRow{}, false
	}
	if !isPlausibleGroqModelID(id) {
		return GroqDocRow{}, false
	}
	rpm := cells[1]
	if !isGroqRPMCell(rpm) {
		return GroqDocRow{}, false
	}
	var rpd, tpm, tpd, ash, asd string
	if len(cells) > 2 {
		rpd = cells[2]
	}
	if len(cells) > 3 {
		tpm = cells[3]
	}
	if len(cells) > 4 {
		tpd = cells[4]
	}
	if len(cells) > 5 {
		ash = cells[5]
	}
	if len(cells) > 6 {
		asd = cells[6]
	}
	return GroqDocRow{SourceID: id, GroqLimits: GroqLimits{RPM: rpm, RPD: rpd, TPM: tpm, TPD: tpd, ASH: ash, ASD: asd}}, true
}

// parseGroqDataRowLine parses one markdown table row from the model limits table.
func parseGroqDataRowLine(line string) (GroqDocRow, bool) {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "|") {
		return GroqDocRow{}, false
	}
	parts := strings.Split(line, "|")
	var cells []string
	for i := 1; i < len(parts)-1; i++ {
		cells = append(cells, strings.TrimSpace(parts[i]))
	}
	return groqRowFromCells(cells, 3)
}

// GroqDocRow is one model row from the Groq published rate-limits page.
type GroqDocRow struct {
	SourceID string
	GroqLimits
}

// ParseGroqRateLimitRows extracts model ids and published limit columns from the rate-limits document.
func ParseGroqRateLimitRows(body string) []GroqDocRow {
	plain := StripHTMLToText(body)
	if strings.Contains(strings.ToLower(plain), "| model id |") {
		return parseGroqMarkdownTableRows(groqModelRateTableSection(body))
	}
	if rows := parseGroqFromHTMLTable(body); len(rows) > 0 {
		return rows
	}
	return parseGroqMarkdownTableRows(plain)
}

func parseGroqMarkdownTableRows(section string) []GroqDocRow {
	seen := make(map[string]struct{})
	var out []GroqDocRow
	for _, line := range strings.Split(section, "\n") {
		row, ok := parseGroqDataRowLine(strings.TrimSpace(line))
		if !ok {
			continue
		}
		if _, dup := seen[row.SourceID]; dup {
			continue
		}
		seen[row.SourceID] = struct{}{}
		out = append(out, row)
	}
	return out
}

// ParseGroqRateLimitPage extracts MODEL ID values (backward compatible with older callers).
func ParseGroqRateLimitPage(body string) []string {
	rows := ParseGroqRateLimitRows(body)
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.SourceID
	}
	return out
}
