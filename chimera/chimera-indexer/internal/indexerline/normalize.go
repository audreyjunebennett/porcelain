// Package indexerline normalizes raw chimera-indexer process output into JSON lines with stable
// indexer.* msg slugs and structured fields for the operator settings UI (/ui/settings).
package indexerline

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	wline "github.com/lynn/porcelain/chimera/internal/wrapper/line"
	"github.com/lynn/porcelain/internal/naming"
)

var indexerReservedKeys = map[string]struct{}{
	"msg":             {},
	"message":         {},
	"time":            {},
	"timestamp":       {},
	"level":           {},
	"service":         {},
	"progress_detail": {},
	"state":           {},
	"_chimera_norm":   {},
}

var indexerCoreKeyOrder = []string{
	"timestamp",
	"level",
	"service",
	"msg",
	"state",
	"progress_detail",
}

type indexerCore struct {
	Timestamp      string
	Level          string
	Service        string
	Msg            string
	State          string
	ProgressDetail string
}

// NormalizePayload converts one raw indexer line into a stable structured JSON line.
func NormalizePayload(raw string) []byte {
	return wline.NormalizePerLine(raw, alreadyNormalized, normalizePlain, normalizeJSON)
}

func normalizePlain(raw string) []byte {
	out := indexerCore{
		Timestamp:      wline.UTCTimestampNow(),
		Msg:            "indexer.log.line",
		Service:        naming.ProductIndexerBinName,
		ProgressDetail: strings.TrimSpace(raw),
	}
	b, err := marshalIndexerLine(out, nil, false)
	if err != nil {
		return nil
	}
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

	service := normalizeIndexerService(wline.JSONString(fields, "service"))
	level := strings.ToUpper(strings.TrimSpace(wline.JSONString(fields, "level")))
	state := strings.TrimSpace(wline.JSONString(fields, "state"))
	progress := strings.TrimSpace(wline.JSONString(fields, "progress_detail"))

	ts := strings.TrimSpace(wline.JSONString(fields, "time"))
	if ts == "" {
		ts = strings.TrimSpace(wline.JSONString(fields, "timestamp"))
	}
	if ts != "" {
		ts = wline.NormalizeTimestampUTC(ts)
	}

	core := indexerCore{
		Timestamp:      ts,
		Msg:            msg,
		Service:        service,
		Level:          level,
		State:          state,
		ProgressDetail: progress,
	}
	b, err := marshalIndexerLine(core, fields, isIndexerDomainMsg(msg))
	if err != nil {
		return normalizePlain(raw)
	}
	return b
}

func normalizeIndexerService(raw string) string {
	s := strings.TrimSpace(raw)
	switch strings.ToLower(s) {
	case "", "indexer":
		return naming.ProductIndexerBinName
	default:
		return s
	}
}

func isIndexerDomainMsg(msg string) bool {
	msg = strings.TrimSpace(msg)
	return strings.HasPrefix(msg, "indexer.") || strings.HasPrefix(msg, "chimera-indexer.")
}

func marshalIndexerLine(core indexerCore, fields map[string]json.RawMessage, passthrough bool) ([]byte, error) {
	buf := &bytes.Buffer{}
	buf.WriteByte('{')
	first := true
	emit := func(key string, val json.RawMessage) {
		if len(val) == 0 {
			return
		}
		if !first {
			buf.WriteByte(',')
		}
		first = false
		keyJSON, err := json.Marshal(key)
		if err != nil {
			return
		}
		buf.Write(keyJSON)
		buf.WriteByte(':')
		buf.Write(val)
	}

	for _, key := range indexerCoreKeyOrder {
		if raw, ok := indexerCoreFieldRaw(core, key); ok {
			emit(key, raw)
		}
	}

	if passthrough && fields != nil {
		for _, key := range indexerExtraKeys(fields) {
			emit(key, fields[key])
		}
	}

	emit("_chimera_norm", json.RawMessage("1"))
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

func indexerCoreFieldRaw(core indexerCore, key string) (json.RawMessage, bool) {
	switch key {
	case "timestamp":
		if core.Timestamp != "" {
			return marshalJSONScalar(core.Timestamp)
		}
	case "level":
		if core.Level != "" {
			return marshalJSONScalar(core.Level)
		}
	case "service":
		if core.Service != "" {
			return marshalJSONScalar(core.Service)
		}
	case "msg":
		if core.Msg != "" {
			return marshalJSONScalar(core.Msg)
		}
	case "state":
		if core.State != "" {
			return marshalJSONScalar(core.State)
		}
	case "progress_detail":
		if core.ProgressDetail != "" {
			return marshalJSONScalar(core.ProgressDetail)
		}
	}
	return nil, false
}

func indexerExtraKeys(fields map[string]json.RawMessage) []string {
	var extras []string
	for k := range fields {
		if _, reserved := indexerReservedKeys[k]; reserved {
			continue
		}
		extras = append(extras, k)
	}
	sort.Strings(extras)
	return extras
}

func marshalJSONScalar(v any) (json.RawMessage, bool) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, false
	}
	return b, true
}

func alreadyNormalized(raw []byte) ([]byte, bool) {
	if b, ok := wline.ReorderNormalizedJSON(raw); ok {
		return b, true
	}
	if _, ok := wline.AlreadyNormalizedChimera(raw, "indexer.", naming.ProductIndexerBinName); ok {
		return wline.ReorderNormalizedJSON(raw)
	}
	if _, ok := wline.AlreadyNormalizedChimera(raw, "chimera-indexer.", naming.ProductIndexerBinName); ok {
		return wline.ReorderNormalizedJSON(raw)
	}
	if b, ok := wline.PassthroughSlogJSON(raw, naming.ProductIndexerBinName); ok {
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
