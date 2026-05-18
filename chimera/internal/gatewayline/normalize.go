// Package gatewayline normalizes raw gateway process output into JSON lines with
// stable gateway.* msg slugs and structured fields for wrapper debug logs.
package gatewayline

import (
	"encoding/json"
	"strings"

	wline "github.com/lynn/porcelain/chimera/internal/wrapper/line"
)

type normalized struct {
	Timestamp      string `json:"timestamp,omitempty"`
	Level          string `json:"level,omitempty"`
	Service        string `json:"service"`
	Msg            string `json:"msg"`
	Method         string `json:"method,omitempty"`
	Path           string `json:"path,omitempty"`
	StatusCode     int    `json:"statusCode,omitempty"`
	ResponseTimeMS int64  `json:"responseTimeMs,omitempty"`
	TimelineKind   string `json:"timeline_kind,omitempty"`
	RequestID      string `json:"request_id,omitempty"`
	Authorization  string `json:"authorization,omitempty"`
	ProgressDetail string `json:"progress_detail,omitempty"`
	ChimeraNorm    int    `json:"_chimera_norm,omitempty"`
}

// NormalizePayload converts one raw line (no trailing \n) into a JSON log line.
func NormalizePayload(raw string) []byte {
	return wline.NormalizePerLine(raw, alreadyNormalized, normalizePlain, normalizeJSON)
}

func normalizeJSON(raw string) []byte {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &fields); err != nil {
		return fallbackUnknown(raw, "", "")
	}
	out := normalized{
		Service:     "chimera-gateway",
		ChimeraNorm: 1,
	}
	out.Timestamp = wline.JSONString(fields, "time")
	out.Level = strings.ToUpper(strings.TrimSpace(wline.JSONString(fields, "level")))
	msg := strings.TrimSpace(wline.JSONString(fields, "msg"))
	if msg == "" {
		msg = strings.TrimSpace(wline.JSONString(fields, "message"))
	}
	if msg == "" {
		msg = "gateway.log.text"
	}
	out.Msg = msg
	out.Method = wline.JSONString(fields, "method")
	out.Path = wline.JSONString(fields, "path")
	out.StatusCode = wline.IntFromJSON(fields, "statusCode")
	out.ResponseTimeMS = int64(wline.FloatFromJSON(fields, "responseTimeMs"))
	out.TimelineKind = wline.JSONString(fields, "timeline_kind")
	out.RequestID = wline.JSONString(fields, "request_id")
	out.Authorization = wline.JSONString(fields, "authorization")
	if out.Msg == "gateway.http.access" && out.Method == "" && out.Path == "" {
		out.ProgressDetail = wline.TrimRunes(raw, 2048)
	}
	b, err := json.Marshal(out)
	if err != nil {
		return fallbackUnknown(raw, out.Level, msg)
	}
	return b
}

func normalizePlain(raw string) []byte {
	s := strings.TrimSpace(raw)
	if s == "" {
		return nil
	}
	out := normalized{
		Service:        "chimera-gateway",
		Level:          "INFO",
		Msg:            "gateway.log.text",
		ProgressDetail: wline.TrimRunes(s, 2048),
		ChimeraNorm:    1,
	}
	b, _ := json.Marshal(out)
	return b
}

func alreadyNormalized(raw []byte) ([]byte, bool) {
	if b, ok := wline.ReorderNormalizedJSON(raw); ok {
		return b, true
	}
	if _, ok := wline.AlreadyNormalizedChimera(raw, "gateway.", "chimera-gateway"); ok {
		return wline.ReorderNormalizedJSON(raw)
	}
	if b, ok := wline.PassthroughSlogJSON(raw, "chimera-gateway"); ok {
		return b, true
	}
	return nil, false
}

func fallbackUnknown(raw, level, msg string) []byte {
	if strings.TrimSpace(msg) == "" {
		msg = "gateway.unparsed"
	}
	out := normalized{
		Service:        "chimera-gateway",
		Level:          strings.ToUpper(strings.TrimSpace(level)),
		Msg:            msg,
		ProgressDetail: wline.TrimRunes(raw, 2048),
		ChimeraNorm:    1,
	}
	b, _ := json.Marshal(out)
	return b
}
