package indexerline

import (
	"encoding/json"
	"fmt"
	"strings"

	wline "github.com/lynn/porcelain/chimera/internal/wrapper/line"
)

type normalized struct {
	Timestamp      string `json:"timestamp,omitempty"`
	Level          string `json:"level,omitempty"`
	Service        string `json:"service"`
	Msg            string `json:"msg"`
	State          string `json:"state,omitempty"`
	ProgressDetail string `json:"progress_detail,omitempty"`
	ChimeraNorm    int    `json:"_chimera_norm,omitempty"`
}

// NormalizePayload converts one raw indexer line into a stable structured JSON line.
func NormalizePayload(raw string) []byte {
	return wline.NormalizePerLine(raw, alreadyNormalized, normalizePlain, normalizeJSON)
}

func normalizePlain(raw string) []byte {
	out := normalized{
		Msg:            "indexer.log.line",
		Service:        "chimera-indexer",
		ProgressDetail: strings.TrimSpace(raw),
		ChimeraNorm:    1,
	}
	b, _ := json.Marshal(out)
	return b
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
	if msg == "" {
		msg = "indexer.log.line"
	}
	service := "chimera-indexer"
	level := strings.ToUpper(strings.TrimSpace(wline.JSONString(fields, "level")))
	state := strings.TrimSpace(wline.JSONString(fields, "state"))
	progress := strings.TrimSpace(wline.JSONString(fields, "progress_detail"))

	ts := strings.TrimSpace(wline.JSONString(fields, "time"))
	if ts == "" {
		ts = strings.TrimSpace(wline.JSONString(fields, "timestamp"))
	}
	out := normalized{
		Timestamp:      ts,
		Msg:            msg,
		Service:        service,
		Level:          level,
		State:          state,
		ProgressDetail: progress,
		ChimeraNorm:    1,
	}
	b, err := json.Marshal(out)
	if err != nil {
		return normalizePlain(raw)
	}
	return b
}

func alreadyNormalized(raw []byte) ([]byte, bool) {
	if b, ok := wline.ReorderNormalizedJSON(raw); ok {
		return b, true
	}
	if _, ok := wline.AlreadyNormalizedChimera(raw, "indexer.", "chimera-indexer"); ok {
		return wline.ReorderNormalizedJSON(raw)
	}
	if b, ok := wline.PassthroughSlogJSON(raw, "chimera-indexer"); ok {
		return b, true
	}
	return nil, false
}

// SupervisorHeartbeat extracts indexer supervisor state from one normalized/raw line.
type SupervisorHeartbeat struct {
	DeclaredState string
	WorkerState   string
}

// ParseSupervisorHeartbeat returns heartbeat details when line is an indexer.state event.
func ParseSupervisorHeartbeat(raw string) (SupervisorHeartbeat, bool) {
	var out SupervisorHeartbeat
	var flat map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &flat); err != nil {
		return out, false
	}
	msg := strings.TrimSpace(fmt.Sprint(flat["msg"]))
	if msg == "" || msg == "<nil>" {
		msg = strings.TrimSpace(fmt.Sprint(flat["message"]))
	}
	msg = strings.ToLower(msg)
	if msg != "indexer.state" && msg != "indexer state" {
		return out, false
	}
	declaredState := strings.TrimSpace(fmt.Sprint(flat["state"]))
	if declaredState == "<nil>" {
		declaredState = ""
	}
	recovery := false
	if rv, ok := flat["recovery"].(bool); ok && rv {
		recovery = true
	}
	workerState := "up"
	if recovery || strings.EqualFold(declaredState, "recovery") {
		workerState = "degraded"
	}
	out.DeclaredState = declaredState
	out.WorkerState = workerState
	return out, true
}
