// Package embedline normalizes raw llama-server output into JSON lines with stable embed.* msg slugs.
package embedline

import (
	"encoding/json"
	"io"
	"strings"

	wline "github.com/lynn/porcelain/chimera/internal/wrapper/line"
	"github.com/lynn/porcelain/internal/naming"
)

// NormalizePayload converts one upstream line into normalized JSON bytes.
func NormalizePayload(raw string) []byte {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	level := "INFO"
	lower := strings.ToLower(raw)
	switch {
	case strings.Contains(lower, "error"):
		level = "ERROR"
	case strings.Contains(lower, "warn"):
		level = "WARN"
	}
	msg := "embed.upstream.line"
	if strings.Contains(lower, "listening") {
		msg = "embed.llama_server.listening"
	}
	rec := map[string]any{
		"service": naming.ProductEmbedName,
		"msg":     msg,
		"level":   level,
		"detail":  raw,
	}
	b, err := json.Marshal(rec)
	if err != nil {
		return []byte(`{"service":"` + naming.ProductEmbedName + `","msg":"embed.upstream.line","detail":` + jsonString(raw) + `}`)
	}
	return b
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// NewWriter returns a line writer that normalizes upstream output.
func NewWriter(dst io.Writer) io.Writer {
	return wline.NewWriter(dst, NormalizePayload)
}
