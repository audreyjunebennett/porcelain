// Package supervisorline normalizes chimera-supervisor slog output for the operator log buffer.
package supervisorline

import (
	"encoding/json"
	"strings"

	wline "github.com/lynn/porcelain/chimera/internal/wrapper/line"
)

const serviceName = "chimera-supervisor"

type normalized struct {
	Timestamp      string `json:"timestamp,omitempty"`
	Level          string `json:"level,omitempty"`
	Service        string `json:"service"`
	Msg            string `json:"msg"`
	ProgressDetail string `json:"progress_detail,omitempty"`
	ChimeraNorm    int    `json:"_chimera_norm,omitempty"`
}

// NormalizePayload converts one raw line into structured JSON for the logs UI.
func NormalizePayload(raw string) []byte {
	return wline.NormalizePerLine(raw, alreadyNormalized, normalizePlain, normalizeJSON)
}

func normalizeJSON(raw string) []byte {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &fields); err != nil {
		return fallbackUnknown(raw, "", "")
	}
	out := normalized{
		Service:     serviceName,
		ChimeraNorm: 1,
	}
	out.Timestamp = wline.JSONString(fields, "time")
	out.Level = strings.ToUpper(strings.TrimSpace(wline.JSONString(fields, "level")))
	msg := strings.TrimSpace(wline.JSONString(fields, "msg"))
	if msg == "" {
		msg = strings.TrimSpace(wline.JSONString(fields, "message"))
	}
	if msg == "" {
		msg = "chimera-supervisor.log.text"
	}
	out.Msg = msg
	if out.Level == "" {
		out.Level = "INFO"
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
		Service:        serviceName,
		Level:          "INFO",
		Msg:            "chimera-supervisor.log.text",
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
	if _, ok := wline.AlreadyNormalizedChimera(raw, "chimera-supervisor.", serviceName); ok {
		return wline.ReorderNormalizedJSON(raw)
	}
	if b, ok := wline.PassthroughSlogJSON(raw, serviceName); ok {
		return b, true
	}
	return nil, false
}

func fallbackUnknown(raw, level, msg string) []byte {
	if strings.TrimSpace(msg) == "" {
		msg = "chimera-supervisor.unparsed"
	}
	out := normalized{
		Service:        serviceName,
		Level:          strings.ToUpper(strings.TrimSpace(level)),
		Msg:            msg,
		ProgressDetail: wline.TrimRunes(raw, 2048),
		ChimeraNorm:    1,
	}
	b, _ := json.Marshal(out)
	return b
}
