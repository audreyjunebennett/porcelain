package line

import (
	"encoding/json"
	"strings"
)

// PassthroughSlogJSON converts log/slog JSON lines (msg + level/time) into chimera-normalized JSON.
func PassthroughSlogJSON(raw []byte, defaultService string) ([]byte, bool) {
	if b, ok := ReorderNormalizedJSON(raw); ok {
		return b, true
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, false
	}
	msg := strings.TrimSpace(JSONString(fields, "msg"))
	if msg == "" {
		return nil, false
	}
	if IsDomainServiceMsg(msg, defaultService) {
		return nil, false
	}
	level := strings.TrimSpace(JSONString(fields, "level"))
	ts := strings.TrimSpace(JSONString(fields, "time"))
	if ts == "" {
		ts = strings.TrimSpace(JSONString(fields, "timestamp"))
	}
	if level == "" && ts == "" {
		return nil, false
	}
	svc := strings.TrimSpace(JSONString(fields, "service"))
	if svc == "" {
		svc = serviceFromComponent(JSONString(fields, "component"), defaultService)
	}
	rec := orderedLog{
		Timestamp:   ts,
		Level:       strings.ToUpper(level),
		Service:     svc,
		Msg:         msg,
		Component:   strings.TrimSpace(JSONString(fields, "component")),
		BackendName: strings.TrimSpace(JSONString(fields, "backend_name")),
		BackendMode: strings.TrimSpace(JSONString(fields, "backend_mode")),
		Status:      strings.TrimSpace(JSONString(fields, "status")),
		Err:         strings.TrimSpace(JSONString(fields, "err")),
		Bin:         strings.TrimSpace(JSONString(fields, "bin")),
		Listen:      strings.TrimSpace(JSONString(fields, "listen")),
		Endpoint:    strings.TrimSpace(JSONString(fields, "endpoint")),
		Storage:     strings.TrimSpace(JSONString(fields, "storage")),
		HTTPPort:    strings.TrimSpace(JSONString(fields, "http_port")),
		GRPCPort:    strings.TrimSpace(JSONString(fields, "grpc_port")),
		Host:        strings.TrimSpace(JSONString(fields, "host")),
		Port:        strings.TrimSpace(JSONString(fields, "port")),
		AppDir:      strings.TrimSpace(JSONString(fields, "app_dir")),
		ConfigPath:  strings.TrimSpace(JSONString(fields, "config_path")),
		Workdir:     strings.TrimSpace(JSONString(fields, "workdir")),
		LogJSON:     strings.TrimSpace(JSONString(fields, "log_json")),
		Child:       strings.TrimSpace(JSONString(fields, "child")),
		PID:         strings.TrimSpace(JSONString(fields, "pid")),
		Timeout:     strings.TrimSpace(JSONString(fields, "timeout")),
		Detail:      strings.TrimSpace(JSONString(fields, "detail")),
		Forced:      strings.TrimSpace(JSONString(fields, "forced")),
		ExitCode:    strings.TrimSpace(JSONString(fields, "exit_code")),
		State:       strings.TrimSpace(JSONString(fields, "state")),
		ChimeraNorm: ChimeraNormValue,
	}
	return MarshalOrdered(rec), true
}

// IsDomainServiceMsg reports whether msg is already a service-scoped event slug (not wrapper slog).
func IsDomainServiceMsg(msg, service string) bool {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return false
	}
	switch service {
	case "chimera-gateway", "gateway":
		return strings.HasPrefix(msg, "gateway.")
	case "chimera-broker", "broker":
		return strings.HasPrefix(msg, "broker.")
	case "chimera-vectorstore", "vectorstore":
		return strings.HasPrefix(msg, "vectorstore.")
	case "chimera-indexer", "indexer":
		return strings.HasPrefix(msg, "indexer.")
	case "chimera-supervisor":
		return strings.HasPrefix(msg, "chimera-supervisor.")
	default:
		return false
	}
}

func serviceFromComponent(component, defaultService string) string {
	c := strings.ToLower(strings.TrimSpace(component))
	switch {
	case strings.Contains(c, "broker"):
		return "chimera-broker"
	case strings.Contains(c, "vectorstore"), strings.Contains(c, "qdrant"):
		return "chimera-vectorstore"
	case strings.Contains(c, "supervisor"):
		return "chimera-supervisor"
	case strings.Contains(c, "gateway"):
		return "chimera-gateway"
	case strings.Contains(c, "indexer"):
		return "chimera-indexer"
	default:
		return defaultService
	}
}
