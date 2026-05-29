package rag

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/vectorstore"
	"github.com/lynn/porcelain/internal/naming"
)

const hitPreviewMaxRunes = 1600

// HitSummary is a compact retrieval hit exposed to operator/chat clients.
type HitSummary struct {
	Source   string  `json:"source"`
	Text     string  `json:"text"`
	Score    float32 `json:"score"`
	Language string  `json:"language,omitempty"`
}

// LanguageFromSource infers a highlight/render language id from a file path or source label.
func LanguageFromSource(source string) string {
	ext := strings.ToLower(filepath.Ext(strings.TrimSpace(source)))
	switch ext {
	case ".go":
		return "go"
	case ".js", ".mjs", ".cjs", ".jsx":
		return "javascript"
	case ".ts", ".tsx":
		return "typescript"
	case ".py", ".pyw":
		return "python"
	case ".json":
		return "json"
	case ".yaml", ".yml":
		return "yaml"
	case ".md", ".markdown":
		return "markdown"
	case ".sql":
		return "sql"
	case ".sh", ".bash", ".zsh":
		return "shell"
	case ".css":
		return "css"
	case ".html", ".htm":
		return "html"
	case ".rs":
		return "rust"
	case ".java":
		return "java"
	case ".xml":
		return "xml"
	case ".toml":
		return "toml"
	default:
		return ""
	}
}

// SummarizeHits returns bounded excerpts suitable for response headers and UI footnotes.
func SummarizeHits(hits []vectorstore.Hit) []HitSummary {
	if len(hits) == 0 {
		return nil
	}
	out := make([]HitSummary, 0, len(hits))
	for _, h := range hits {
		src := strings.TrimSpace(h.Payload.Source)
		if src == "" {
			src = "unknown"
		}
		out = append(out, HitSummary{
			Source:   src,
			Text:     previewHitText(h.Payload.Text),
			Score:    h.Score,
			Language: LanguageFromSource(src),
		})
	}
	return out
}

func previewHitText(s string) string {
	t := strings.TrimSpace(s)
	if t == "" {
		return ""
	}
	t = strings.ReplaceAll(t, "\r\n", "\n")
	t = strings.ReplaceAll(t, "\r", "\n")
	n := 0
	for i := range t {
		if n == hitPreviewMaxRunes {
			return t[:i] + "…"
		}
		n++
	}
	return t
}

// WriteResponseHeaders sets optional chat-turn metadata headers when values are present.
func WriteResponseHeaders(w http.ResponseWriter, upstreamModel string, hits []vectorstore.Hit) {
	if w == nil {
		return
	}
	if m := strings.TrimSpace(upstreamModel); m != "" {
		w.Header().Set(naming.HeaderUpstreamModelTarget, m)
	}
	summaries := SummarizeHits(hits)
	if len(summaries) == 0 {
		return
	}
	b, err := json.Marshal(summaries)
	if err != nil {
		return
	}
	// HTTP response headers must stay ASCII-safe; raw UTF-8 JSON is mangled by many
	// clients (e.g. browser fetch) and shows mojibake such as "â" for em dashes.
	w.Header().Set(naming.HeaderRAGHitsTarget, base64.StdEncoding.EncodeToString(b))
}
