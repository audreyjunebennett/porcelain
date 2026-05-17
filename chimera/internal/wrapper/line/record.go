package line

import (
	"encoding/json"
	"strconv"
	"strings"
)

// ChimeraNormValue marks a line that already passed chimera line normalization.
// The UI and line writers use this to avoid double-normalizing the same payload.
const ChimeraNormValue = 1

// orderedLog is the canonical operator-console JSON field order:
// timestamp, level, service, msg, then optional attributes, then _chimera_norm.
type orderedLog struct {
	Timestamp   string `json:"timestamp,omitempty"`
	Level       string `json:"level,omitempty"`
	Service     string `json:"service,omitempty"`
	Msg         string `json:"msg,omitempty"`
	Component   string `json:"component,omitempty"`
	BackendName string `json:"backend_name,omitempty"`
	BackendMode string `json:"backend_mode,omitempty"`
	Status      string `json:"status,omitempty"`
	Err         string `json:"err,omitempty"`
	Bin         string `json:"bin,omitempty"`
	Listen      string `json:"listen,omitempty"`
	Endpoint    string `json:"endpoint,omitempty"`
	Storage     string `json:"storage,omitempty"`
	HTTPPort    string `json:"http_port,omitempty"`
	GRPCPort    string `json:"grpc_port,omitempty"`
	Host        string `json:"host,omitempty"`
	Port        string `json:"port,omitempty"`
	AppDir      string `json:"app_dir,omitempty"`
	ConfigPath  string `json:"config_path,omitempty"`
	Workdir     string `json:"workdir,omitempty"`
	LogJSON     string `json:"log_json,omitempty"`
	Child       string `json:"child,omitempty"`
	PID         string `json:"pid,omitempty"`
	Timeout     string `json:"timeout,omitempty"`
	Detail      string `json:"detail,omitempty"`
	Forced      string `json:"forced,omitempty"`
	ExitCode    string `json:"exit_code,omitempty"`
	State       string `json:"state,omitempty"`
	ChimeraNorm int    `json:"_chimera_norm,omitempty"`
}

// MarshalOrdered emits JSON with stable key order for operator logs.
func MarshalOrdered(rec orderedLog) []byte {
	if rec.ChimeraNorm == 0 && rec.Msg != "" {
		rec.ChimeraNorm = ChimeraNormValue
	}
	b, err := json.Marshal(rec)
	if err != nil {
		return nil
	}
	return b
}

// ReorderNormalizedJSON re-marshals an existing normalized line into canonical key order.
func ReorderNormalizedJSON(raw []byte) ([]byte, bool) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, false
	}
	if IntFromJSON(fields, "_chimera_norm") != ChimeraNormValue {
		return nil, false
	}
	msg := strings.TrimSpace(JSONString(fields, "msg"))
	svc := strings.TrimSpace(JSONString(fields, "service"))
	if msg == "" || svc == "" {
		return nil, false
	}
	rec := orderedLogFromFields(fields)
	rec.Msg = msg
	rec.Service = svc
	rec.ChimeraNorm = ChimeraNormValue
	return MarshalOrdered(rec), true
}

func orderedLogFromFields(fields map[string]json.RawMessage) orderedLog {
	ts := strings.TrimSpace(JSONString(fields, "timestamp"))
	if ts == "" {
		ts = strings.TrimSpace(JSONString(fields, "time"))
	}
	return orderedLog{
		Timestamp:   ts,
		Level:       strings.ToUpper(strings.TrimSpace(JSONString(fields, "level"))),
		Service:     strings.TrimSpace(JSONString(fields, "service")),
		Msg:         strings.TrimSpace(JSONString(fields, "msg")),
		Component:   strings.TrimSpace(JSONString(fields, "component")),
		BackendName: strings.TrimSpace(JSONString(fields, "backend_name")),
		BackendMode: strings.TrimSpace(JSONString(fields, "backend_mode")),
		Status:      strings.TrimSpace(JSONString(fields, "status")),
		Err:         strings.TrimSpace(JSONString(fields, "err")),
		Bin:         strings.TrimSpace(JSONString(fields, "bin")),
		Listen:      strings.TrimSpace(JSONString(fields, "listen")),
		Endpoint:    strings.TrimSpace(JSONString(fields, "endpoint")),
		Storage:     strings.TrimSpace(JSONString(fields, "storage")),
		HTTPPort:    stringFromJSON(fields, "http_port"),
		GRPCPort:    stringFromJSON(fields, "grpc_port"),
		Host:        strings.TrimSpace(JSONString(fields, "host")),
		Port:        stringFromJSON(fields, "port"),
		AppDir:      strings.TrimSpace(JSONString(fields, "app_dir")),
		ConfigPath:  strings.TrimSpace(JSONString(fields, "config_path")),
		Workdir:     strings.TrimSpace(JSONString(fields, "workdir")),
		LogJSON:     stringFromJSON(fields, "log_json"),
		Child:       strings.TrimSpace(JSONString(fields, "child")),
		PID:         stringFromJSON(fields, "pid"),
		Timeout:     strings.TrimSpace(JSONString(fields, "timeout")),
		Detail:      strings.TrimSpace(JSONString(fields, "detail")),
		Forced:      stringFromJSON(fields, "forced"),
		ExitCode:    stringFromJSON(fields, "exit_code"),
		State:       strings.TrimSpace(JSONString(fields, "state")),
	}
}

func stringFromJSON(fields map[string]json.RawMessage, key string) string {
	if s := strings.TrimSpace(JSONString(fields, key)); s != "" {
		return s
	}
	if n := IntFromJSON(fields, key); n != 0 {
		return strconv.Itoa(n)
	}
	return ""
}
