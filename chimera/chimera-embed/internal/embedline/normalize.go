package embedline

import (
	"encoding/json"
	"io"
	"strings"

	wline "github.com/lynn/porcelain/chimera/internal/wrapper/line"
	"github.com/lynn/porcelain/internal/naming"
)

type normalized struct {
	Timestamp   string `json:"timestamp,omitempty"`
	Level       string `json:"level,omitempty"`
	Service     string `json:"service"`
	Msg         string `json:"msg"`
	Detail      string `json:"detail,omitempty"`
	ChimeraNorm int    `json:"_chimera_norm,omitempty"`
}

func NormalizePayload(raw string) []byte {
	return wline.NormalizePerLine(raw, alreadyNormalized, normalizePlain, normalizeJSON)
}

func alreadyNormalized(raw []byte) ([]byte, bool) {
	if reordered, ok := wline.ReorderNormalizedJSON(raw); ok {
		return reordered, true
	}
	return wline.NormalizeSlogLine(raw, naming.ProductEmbedName)
}

func normalizeJSON(raw string) []byte {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &fields); err != nil {
		return normalizePlain(raw)
	}
	msg := strings.TrimSpace(wline.JSONString(fields, "msg"))
	if msg == "" {
		msg = strings.TrimSpace(wline.JSONString(fields, "message"))
	}
	if !strings.HasPrefix(msg, "embed.") {
		return normalizePlain(raw)
	}
	level := strings.ToUpper(strings.TrimSpace(wline.JSONString(fields, "level")))
	if level == "" {
		level = "INFO"
	}
	timestamp := wline.JSONString(fields, "time")
	if timestamp == "" {
		timestamp = wline.JSONString(fields, "timestamp")
	}
	rec := normalized{
		Timestamp:   wline.NormalizeTimestampUTC(timestamp),
		Level:       level,
		Service:     naming.ProductEmbedName,
		Msg:         msg,
		Detail:      wline.TrimRunes(wline.UpstreamDetailFromFields(fields), 2048),
		ChimeraNorm: wline.ChimeraNormValue,
	}
	b, _ := json.Marshal(rec)
	return b
}

func normalizePlain(raw string) []byte {
	detail := strings.TrimSpace(raw)
	lower := strings.ToLower(detail)
	level := "INFO"
	switch {
	case strings.Contains(lower, "error"), strings.Contains(lower, "fatal"):
		level = "ERROR"
	case strings.Contains(lower, "warn"):
		level = "WARN"
	}
	msg := "embed.upstream.line"
	if strings.Contains(lower, "listening") {
		msg = "embed.llama_server.listening"
	}
	rec := normalized{
		Timestamp:   wline.UTCTimestampNow(),
		Level:       level,
		Service:     naming.ProductEmbedName,
		Msg:         msg,
		Detail:      wline.TrimRunes(detail, 4096),
		ChimeraNorm: wline.ChimeraNormValue,
	}
	b, _ := json.Marshal(rec)
	return b
}

func NewWriter(dst io.Writer) io.Writer { return wline.NewWriter(dst, NormalizePayload) }
