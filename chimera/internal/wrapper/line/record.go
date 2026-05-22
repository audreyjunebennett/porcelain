package line

import (
	"bytes"
	"encoding/json"
	"sort"
	"strconv"
	"strings"
)

// ChimeraNormValue marks a line that already passed chimera line normalization.
// The UI and line writers use this to avoid double-normalizing the same payload.
//
// ReorderNormalizedJSON rewrites canonical keys into stable order but preserves every
// other JSON field from the input (lossless reorder). Supervisor LogSink may run
// normalizers twice on child stdout; the second pass must not drop structured attrs.
const ChimeraNormValue = 1

// orderedLog is the canonical operator-console JSON field order:
// timestamp, level, service, msg, then optional attributes, then extension keys
// (sorted), then _chimera_norm.
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

// canonicalJSONKeys is the stable prefix emitted for every normalized line.
var canonicalJSONKeys = []string{
	"timestamp",
	"level",
	"service",
	"msg",
	"component",
	"backend_name",
	"backend_mode",
	"status",
	"err",
	"bin",
	"listen",
	"endpoint",
	"storage",
	"http_port",
	"grpc_port",
	"host",
	"port",
	"app_dir",
	"config_path",
	"workdir",
	"log_json",
	"child",
	"pid",
	"timeout",
	"detail",
	"forced",
	"exit_code",
	"state",
}

var canonicalJSONKeySet map[string]struct{}

func init() {
	canonicalJSONKeySet = make(map[string]struct{}, len(canonicalJSONKeys))
	for _, k := range canonicalJSONKeys {
		canonicalJSONKeySet[k] = struct{}{}
	}
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

// ReorderNormalizedJSON re-marshals an existing normalized line into canonical key order
// while preserving all other fields from the input object (lossless reorder).
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
	if rec.Timestamp != "" {
		rec.Timestamp = NormalizeTimestampUTC(rec.Timestamp)
	}
	rec.ChimeraNorm = ChimeraNormValue
	b, err := marshalLosslessNormalized(rec, fields)
	if err != nil {
		return nil, false
	}
	return b, true
}

func marshalLosslessNormalized(rec orderedLog, fields map[string]json.RawMessage) ([]byte, error) {
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

	for _, key := range canonicalJSONKeys {
		if raw, ok := canonicalFieldRaw(rec, key, fields); ok {
			emit(key, raw)
		}
	}
	for _, key := range extraNormalizedKeys(fields) {
		emit(key, fields[key])
	}
	emit("_chimera_norm", json.RawMessage("1"))
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

func marshalJSONScalar(v any) (json.RawMessage, bool) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, false
	}
	return b, true
}

func canonicalFieldRaw(rec orderedLog, key string, fields map[string]json.RawMessage) (json.RawMessage, bool) {
	switch key {
	case "timestamp":
		if rec.Timestamp != "" {
			return marshalJSONScalar(rec.Timestamp)
		}
	case "level":
		if rec.Level != "" {
			return marshalJSONScalar(rec.Level)
		}
	case "service":
		if rec.Service != "" {
			return marshalJSONScalar(rec.Service)
		}
	case "msg":
		if rec.Msg != "" {
			return marshalJSONScalar(rec.Msg)
		}
	case "component":
		if rec.Component != "" {
			return marshalJSONScalar(rec.Component)
		}
	case "backend_name":
		if rec.BackendName != "" {
			return marshalJSONScalar(rec.BackendName)
		}
	case "backend_mode":
		if rec.BackendMode != "" {
			return marshalJSONScalar(rec.BackendMode)
		}
	case "status":
		if rec.Status != "" {
			return marshalJSONScalar(rec.Status)
		}
	case "err":
		if rec.Err != "" {
			return marshalJSONScalar(rec.Err)
		}
	case "bin":
		if rec.Bin != "" {
			return marshalJSONScalar(rec.Bin)
		}
	case "listen":
		if rec.Listen != "" {
			return marshalJSONScalar(rec.Listen)
		}
	case "endpoint":
		if rec.Endpoint != "" {
			return marshalJSONScalar(rec.Endpoint)
		}
	case "storage":
		if rec.Storage != "" {
			return marshalJSONScalar(rec.Storage)
		}
	case "http_port":
		if rec.HTTPPort != "" {
			return marshalJSONScalar(rec.HTTPPort)
		}
	case "grpc_port":
		if rec.GRPCPort != "" {
			return marshalJSONScalar(rec.GRPCPort)
		}
	case "host":
		if rec.Host != "" {
			return marshalJSONScalar(rec.Host)
		}
	case "port":
		if rec.Port != "" {
			return marshalJSONScalar(rec.Port)
		}
	case "app_dir":
		if rec.AppDir != "" {
			return marshalJSONScalar(rec.AppDir)
		}
	case "config_path":
		if rec.ConfigPath != "" {
			return marshalJSONScalar(rec.ConfigPath)
		}
	case "workdir":
		if rec.Workdir != "" {
			return marshalJSONScalar(rec.Workdir)
		}
	case "log_json":
		if rec.LogJSON != "" {
			return marshalJSONScalar(rec.LogJSON)
		}
	case "child":
		if rec.Child != "" {
			return marshalJSONScalar(rec.Child)
		}
	case "pid":
		if rec.PID != "" {
			return marshalJSONScalar(rec.PID)
		}
	case "timeout":
		if rec.Timeout != "" {
			return marshalJSONScalar(rec.Timeout)
		}
	case "detail":
		if rec.Detail != "" {
			return marshalJSONScalar(rec.Detail)
		}
	case "forced":
		if rec.Forced != "" {
			return marshalJSONScalar(rec.Forced)
		}
	case "exit_code":
		if rec.ExitCode != "" {
			return marshalJSONScalar(rec.ExitCode)
		}
	case "state":
		if rec.State != "" {
			return marshalJSONScalar(rec.State)
		}
	}
	// Fall back to the input value when the canonical struct field is empty but the
	// source line carried the key (e.g. numeric http_port only in raw JSON).
	if raw, ok := fields[key]; ok && len(raw) > 0 && string(raw) != "null" {
		return raw, true
	}
	return nil, false
}

func extraNormalizedKeys(fields map[string]json.RawMessage) []string {
	var extras []string
	for k := range fields {
		if k == "_chimera_norm" || k == "time" {
			continue
		}
		if _, canon := canonicalJSONKeySet[k]; canon {
			continue
		}
		extras = append(extras, k)
	}
	sort.Strings(extras)
	return extras
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
