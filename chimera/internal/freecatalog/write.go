package freecatalog

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

// GroqLimits holds cells from the Groq docs “Free Plan Limits” table (strings as published).
type GroqLimits struct {
	RPM, RPD, TPM, TPD, ASH, ASD string
}

// Entry is one model line for the generated catalog.
type Entry struct {
	Provider   string      // "groq" | "gemini"
	SourceID   string      // as printed on the provider page
	BiFrostID  string      // provider/model for BiFrost
	SourcePage string      // URL (for header only)
	Groq       *GroqLimits // only for provider "groq"; snapshot comment includes limits
}

// WriteCatalogYAML writes a hand-formatted YAML file so each list item can carry an inline comment.
// Returns the number of unique BiFrost ids written.
func WriteCatalogYAML(path string, generatedAt time.Time, groqURL, geminiURL string, entries []Entry) (int, error) {
	byKey := make(map[string]Entry)
	for _, e := range entries {
		if e.BiFrostID == "" {
			continue
		}
		if _, ok := byKey[e.BiFrostID]; !ok {
			byKey[e.BiFrostID] = e
		}
	}
	keys := make([]string, 0, len(byKey))
	for k := range byKey {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	b.WriteString("# catalog-free — models inferred from public provider documentation.\n")
	b.WriteString("# BiFrost ids are best-effort; compare comments to the source page if a model 404s.\n")
	b.WriteString("# Re-run: make catalog-free (requires network).\n\n")
	fmt.Fprintf(&b, "format_version: 1\n")
	fmt.Fprintf(&b, "generated_at: %q\n", generatedAt.UTC().Format(time.RFC3339))
	b.WriteString("sources:\n")
	fmt.Fprintf(&b, "  groq: %q\n", groqURL)
	fmt.Fprintf(&b, "  gemini: %q\n", geminiURL)
	b.WriteString("models:\n")
	for _, k := range keys {
		e := byKey[k]
		comment := entryYAMLComment(e)
		fmt.Fprintf(&b, "  - %s  # %s\n", e.BiFrostID, comment)
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return 0, err
	}
	return len(keys), nil
}

func sanitizeYAMLComment(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "#", "")
	return strings.TrimSpace(s)
}

func entryYAMLComment(e Entry) string {
	if e.Provider == "groq" && e.Groq != nil {
		g := e.Groq
		line := e.SourceID + " | " + g.RPM + " | " + g.RPD + " | " + g.TPM + " | " + g.TPD + " | " + g.ASH + " | " + g.ASD
		return sanitizeYAMLComment(line)
	}
	return sanitizeYAMLComment(e.SourceID)
}
