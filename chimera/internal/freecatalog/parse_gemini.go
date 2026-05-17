package freecatalog

import (
	"regexp"
	"strings"
)

var geminiBacktickID = regexp.MustCompile("`" + `(gemini-[a-z0-9][a-z0-9.-]*)` + "`")

// geminiCodeTagID matches model ids in rendered HTML (<code>gemini-…</code>).
var geminiCodeTagID = regexp.MustCompile(`(?is)<code[^>]*>\s*(gemini-[a-z0-9][a-z0-9.-]*)\s*</code>`)

// geminiSectionSplit splits pricing page body on ## headings (model sections).
var geminiSectionSplit = regexp.MustCompile(`(?m)^##\s+`)

// geminiHTMLSectionSplit splits on <h2 …> (live pricing docs are HTML, not markdown).
var geminiHTMLSectionSplit = regexp.MustCompile(`(?i)<h2[^>]*>`)

// inputPriceFreeTier marks pricing text where the free tier lists text input as free (markdown tables
// or tag-stripped HTML). Heuristic only — verify against https://ai.google.dev/gemini-api/docs/pricing .
var inputPriceFreeTier = regexp.MustCompile(`(?is)Input price.{0,400}?Free of charge`)

// ParseGeminiPricingFreeInputModels scans the Gemini pricing page for model sections where the
// pricing tables show "Input price" as "Free of charge" on the Free Tier column (text/chat models
// with a real free tier). Sections without that pattern are skipped (e.g. many image-only or
// paid-only rows). This is heuristic: page layout changes can break or widen/narrow the set.
func ParseGeminiPricingFreeInputModels(body string) []string {
	if strings.Contains(strings.ToLower(body), "<h2") {
		return parseGeminiPricingFreeInputModelsHTML(body)
	}
	return parseGeminiPricingFreeInputModelsMarkdown(body)
}

func parseGeminiPricingFreeInputModelsMarkdown(body string) []string {
	parts := geminiSectionSplit.Split(body, -1)
	seen := make(map[string]struct{})
	var out []string
	for _, sec := range parts {
		if !inputPriceFreeTier.MatchString(sec) {
			continue
		}
		for _, m := range geminiBacktickID.FindAllStringSubmatch(sec, -1) {
			id := strings.TrimSpace(m[1])
			if id == "" {
				continue
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			out = append(out, id)
		}
	}
	return out
}

func parseGeminiPricingFreeInputModelsHTML(body string) []string {
	parts := geminiHTMLSectionSplit.Split(body, -1)
	seen := make(map[string]struct{})
	var out []string
	for _, sec := range parts {
		if !inputPriceFreeTier.MatchString(sec) {
			continue
		}
		for _, m := range geminiCodeTagID.FindAllStringSubmatch(sec, -1) {
			id := strings.TrimSpace(m[1])
			if id == "" {
				continue
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			out = append(out, id)
		}
	}
	return out
}
