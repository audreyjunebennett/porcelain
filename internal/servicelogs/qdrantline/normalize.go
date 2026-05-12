// Package qdrantline normalizes raw Qdrant process output into JSON lines with stable
// msg slugs (qdrant.*) and structured fields for the operator logs UI.
package qdrantline

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
)

var accessRE = regexp.MustCompile(`"(?P<method>GET|POST|PUT|PATCH|DELETE|HEAD)\s+(?P<path>[^"]+)\s+HTTP/1\.1"\s+(?P<status>\d{3})`)

var collectionPathRE = regexp.MustCompile(`(?i)/collections/([^/?#]+)`)

var recoveredCollectionRE = regexp.MustCompile(`(?i)Recovered collection\s+([^:]+)`)

var loadingCollectionRE = regexp.MustCompile(`(?i)Loading collection:\s*(.+)\s*$`)

// Recovering shard paths embed the collection directory name before segment "\d+:".
var recoveringShardCollRE = regexp.MustCompile(`(?i)[/\\]collections[/\\]([^/\\]+)[/\\]`)

var httpListenPortRE = regexp.MustCompile(`(?i)(?:HTTP listening|listening on).*?(\d{2,5})\s*$`)

var grpcListenPortRE = regexp.MustCompile(`(?i)gRPC listening.*?(\d{2,5})\s*$`)

var internalGRPCListenPortRE = regexp.MustCompile(`(?i)internal\s+gRPC\s+listening\s+on\s+(\d{2,5})`)

// rustTracingLine is the common Qdrant JSON tracing shape.
type rustTracingLine struct {
	Timestamp string            `json:"timestamp"`
	Level     string            `json:"level"`
	Target    string            `json:"target"`
	Fields    rustTracingFields `json:"fields"`
}

type rustTracingFields struct {
	Message string `json:"message"`
}

type normalized struct {
	Timestamp         string `json:"timestamp,omitempty"`
	Level             string `json:"level,omitempty"`
	Msg               string `json:"msg"`
	Service           string `json:"service"`
	Collection        string `json:"collection,omitempty"`
	HTTPStatus        int    `json:"http_status,omitempty"`
	QdrantVersion     string `json:"qdrant_version,omitempty"`
	RESTPort          int    `json:"rest_port,omitempty"`
	GRPCPort          int    `json:"grpc_port,omitempty"`
	QdrantMode        string `json:"qdrant_mode,omitempty"`
	QdrantTLSREST     string `json:"qdrant_tls_rest,omitempty"`
	QdrantTLSGRPC     string `json:"qdrant_tls_grpc,omitempty"`
	QdrantInternalTLS string `json:"qdrant_internal_tls,omitempty"`
	QdrantTelemetry   string `json:"qdrant_telemetry,omitempty"`
	QdrantRecovery    string `json:"qdrant_recovery,omitempty"`
	QdrantConfig      string `json:"qdrant_config,omitempty"`
	ProgressDetail    string `json:"progress_detail,omitempty"`
	QdrantTarget      string `json:"qdrant_target,omitempty"`
	InternalGRPCPort  int    `json:"internal_grpc_port,omitempty"`
	ClaudiaNorm       int    `json:"_claudia_norm,omitempty"`
}

// NormalizePayload converts one raw line (no trailing \n) into a single JSON log line.
func NormalizePayload(raw string) []byte {
	raw = strings.TrimSuffix(strings.TrimSpace(raw), "\r")
	if raw == "" {
		return nil
	}
	if ja, ok := alreadyNormalized([]byte(raw)); ok {
		return ja
	}

	// Plain-text lines (banner, version, web UI hint).
	if raw[0] != '{' {
		return normalizePlain(raw)
	}

	var line rustTracingLine
	if err := json.Unmarshal([]byte(raw), &line); err != nil {
		return fallbackUnknown(raw, line.Level, "", "")
	}
	msg := strings.TrimSpace(line.Fields.Message)
	tgt := strings.TrimSpace(line.Target)
	out := normalized{
		Timestamp:    line.Timestamp,
		Level:        line.Level,
		Service:      "qdrant",
		ClaudiaNorm:  1,
		QdrantTarget: tgt,
	}

	switch {
	case tgt == "qdrant::settings" && strings.Contains(msg, "Config file not found"):
		out.Msg = "qdrant.config.optional_missing"
		out.QdrantConfig = "supervised"
	case tgt == "storage::content_manager::consensus::persistent" && strings.Contains(msg, "raft"):
		out.Msg = "qdrant.consensus.raft_load"
	case tgt == "storage::content_manager::toc" && strings.HasPrefix(strings.TrimSpace(msg), "Loading collection:"):
		out.Msg = "qdrant.collection.loading"
		if m := loadingCollectionRE.FindStringSubmatch(msg); len(m) == 2 {
			out.Collection = strings.TrimSpace(m[1])
		}
	case tgt == "collection::shards::local_shard" && strings.Contains(msg, "Recovering shard"):
		out.Msg = "qdrant.shard.recover_progress"
		out.ProgressDetail = msg
		if m := recoveringShardCollRE.FindStringSubmatch(msg); len(m) == 2 {
			out.Collection = strings.TrimSpace(m[1])
		}
	case tgt == "collection::shards::local_shard" && strings.Contains(msg, "Recovered collection"):
		out.Msg = "qdrant.shard.recovered"
		out.ProgressDetail = msg
		if m := recoveredCollectionRE.FindStringSubmatch(msg); len(m) == 2 {
			out.Collection = strings.TrimSpace(m[1])
		}
	case tgt == "qdrant" && strings.Contains(msg, "Distributed mode disabled"):
		out.Msg = "qdrant.cluster.single_node"
		out.QdrantMode = "single-node"
	case tgt == "qdrant::actix::web_ui":
		out.Msg = "qdrant.ui.static_missing"
	case tgt == "qdrant::actix" && strings.Contains(strings.ToUpper(msg), "TLS DISABLED"):
		out.Msg = "qdrant.listen.tls_disabled_rest"
		out.QdrantTLSREST = "disabled"
	case tgt == "qdrant::actix" && strings.Contains(msg, "TLS enabled for REST API"):
		out.Msg = "qdrant.listen.tls_enabled_rest"
		out.QdrantTLSREST = "enabled"
	case tgt == "qdrant::actix" && (strings.Contains(msg, "HTTP listening") || strings.Contains(msg, "Qdrant HTTP listening")):
		out.Msg = "qdrant.listen.http"
		if m := httpListenPortRE.FindStringSubmatch(msg); len(m) == 2 {
			if p, err := strconv.Atoi(m[1]); err == nil {
				out.RESTPort = p
			}
		}
	case tgt == "actix_server::builder":
		out.Msg = "qdrant.actix.workers"
	case tgt == "actix_server::server":
		out.Msg = "qdrant.actix.bind"
	case tgt == "qdrant::tonic" && strings.Contains(msg, "internal gRPC listening"):
		out.Msg = "qdrant.listen.internal_grpc"
		if m := internalGRPCListenPortRE.FindStringSubmatch(msg); len(m) == 2 {
			if p, err := strconv.Atoi(m[1]); err == nil {
				out.InternalGRPCPort = p
			}
		}
	case tgt == "qdrant::tonic" && strings.Contains(msg, "gRPC listening"):
		out.Msg = "qdrant.listen.grpc"
		if m := grpcListenPortRE.FindStringSubmatch(msg); len(m) == 2 {
			if p, err := strconv.Atoi(m[1]); err == nil {
				out.GRPCPort = p
			}
		}
	case tgt == "qdrant::tonic" && strings.Contains(strings.ToUpper(msg), "TLS DISABLED"):
		out.Msg = "qdrant.listen.tls_disabled_grpc"
		out.QdrantTLSGRPC = "disabled"
	case tgt == "qdrant::tonic" && strings.Contains(msg, "TLS enabled for gRPC API"):
		out.Msg = "qdrant.listen.tls_enabled_grpc"
		out.QdrantTLSGRPC = "enabled"
	case tgt == "actix_web::middleware::logger":
		return normalizeAccessJSON(line, msg, out)
	default:
		if classifyOperatorSignals(&out, msg, tgt) {
			break
		}
		out.Msg = "qdrant.trace.other"
		out.ProgressDetail = msg
	}

	b, err := json.Marshal(out)
	if err != nil {
		return fallbackUnknown(raw, line.Level, tgt, msg)
	}
	return b
}

func alreadyNormalized(raw []byte) ([]byte, bool) {
	var ck struct {
		Msg         string `json:"msg"`
		Service     string `json:"service"`
		ClaudiaNorm int    `json:"_claudia_norm"`
	}
	if json.Unmarshal(raw, &ck) != nil {
		return nil, false
	}
	if ck.ClaudiaNorm == 1 && strings.HasPrefix(ck.Msg, "qdrant.") && ck.Service == "qdrant" {
		return raw, true
	}
	return nil, false
}

func normalizePlain(raw string) []byte {
	s := strings.TrimSpace(raw)
	out := normalized{
		Service:     "qdrant",
		Level:       "INFO",
		ClaudiaNorm: 1,
	}
	switch {
	case strings.HasPrefix(s, "Version:"):
		out.Msg = "qdrant.version"
		out.QdrantVersion = strings.TrimSpace(strings.TrimPrefix(s, "Version:"))
	case strings.Contains(s, "Access web UI at"):
		out.Msg = "qdrant.web_ui_hint"
	default:
		out.Msg = "qdrant.startup.banner"
		out.ProgressDetail = s
	}
	b, _ := json.Marshal(out)
	return b
}

func normalizeAccessJSON(line rustTracingLine, msg string, base normalized) []byte {
	m := accessRE.FindStringSubmatch(msg)
	if len(m) != 4 {
		base.Msg = "qdrant.http.access_other"
		base.ProgressDetail = msg
		base.QdrantTarget = "actix_web::middleware::logger"
		b, _ := json.Marshal(base)
		return b
	}
	method, path, statusStr := m[1], m[2], m[3]
	status, _ := strconv.Atoi(statusStr)
	base.HTTPStatus = status
	coll := ""
	if cm := collectionPathRE.FindStringSubmatch(path); len(cm) == 2 {
		coll = cm[1]
	}
	base.Collection = coll
	base.QdrantTarget = "actix_web::middleware::logger"

	lowPath := strings.ToLower(path)
	switch {
	case method == "GET" && strings.Contains(lowPath, "/collections/") && !strings.Contains(lowPath, "/points"):
		base.Msg = "qdrant.http.collection_meta"
	case method == "POST" && strings.Contains(lowPath, "/points/delete"):
		base.Msg = "qdrant.http.points_delete"
	case method == "POST" && strings.Contains(lowPath, "/points/search"):
		base.Msg = "qdrant.http.vector_search"
	case method == "PUT" && strings.Contains(lowPath, "/points"):
		if status == 200 {
			base.Msg = "qdrant.http.points_upsert_ok"
		} else {
			base.Msg = "qdrant.http.points_upsert_rejected"
		}
	default:
		base.Msg = "qdrant.http.access_other"
	}
	b, _ := json.Marshal(base)
	return b
}

// classifyOperatorSignals maps high-signal Qdrant log messages that are not tied to a single
// tracing target. Returns true when out.Msg is set.
func classifyOperatorSignals(out *normalized, msg, _ string) bool {
	msgTrim := strings.TrimSpace(msg)
	switch {
	case strings.Contains(msg, "Panic occurred") || strings.Contains(msg, "Panic backtrace"):
		out.Msg = "qdrant.runtime.panic"
		out.ProgressDetail = msg
		return true
	case strings.Contains(msg, "Error while starting") && strings.Contains(msg, "server:"):
		out.Msg = "qdrant.process.server_start_failed"
		out.ProgressDetail = msg
		return true
	case strings.Contains(msg, "Can't initialize GPU"):
		out.Msg = "qdrant.gpu.init_failed"
		out.ProgressDetail = msg
		return true
	case strings.Contains(msg, "Qdrant is loaded in recovery mode"):
		out.Msg = "qdrant.storage.recovery_mode"
		out.QdrantRecovery = "active"
		out.ProgressDetail = msg
		return true
	case isBootstrapURIDuplicateWarning(msg):
		out.Msg = "qdrant.cluster.bootstrap_uri_duplicate"
		out.ProgressDetail = msg
		return true
	case strings.Contains(msg, "Failed to create init file indicator") || strings.Contains(msg, "Failed to remove init file indicator"):
		out.Msg = "qdrant.runtime.init_file_warning"
		out.ProgressDetail = msg
		return true
	case strings.Contains(msg, "JWT RBAC") ||
		(strings.Contains(strings.ToLower(msg), "jwt") && strings.Contains(strings.ToLower(msg), "api key")):
		out.Msg = "qdrant.security.jwt_rbac_warning"
		out.ProgressDetail = msg
		return true
	case strings.Contains(msg, "TLS disabled for internal gRPC API"):
		out.Msg = "qdrant.cluster.internal_tls_disabled"
		out.QdrantInternalTLS = "disabled"
		out.ProgressDetail = msg
		return true
	case strings.Contains(msg, "TLS enabled for internal gRPC API"):
		out.Msg = "qdrant.cluster.internal_tls_enabled"
		out.QdrantInternalTLS = "enabled"
		out.ProgressDetail = msg
		return true
	case strings.Contains(msg, "Telemetry reporting disabled"):
		out.Msg = "qdrant.telemetry.disabled"
		out.QdrantTelemetry = "disabled"
		return true
	case strings.Contains(msg, "Telemetry reporting enabled"):
		out.Msg = "qdrant.telemetry.enabled"
		out.QdrantTelemetry = "enabled"
		return true
	case strings.Contains(msg, "Hardware reporting enabled"):
		out.Msg = "qdrant.hardware_reporting.enabled"
		out.ProgressDetail = msg
		return true
	case strings.Contains(msg, "Inference service is not configured"):
		out.Msg = "qdrant.inference.disabled"
		out.ProgressDetail = msg
		return true
	case strings.Contains(msg, "Inference service is configured"):
		out.Msg = "qdrant.inference.configured"
		out.ProgressDetail = msg
		return true
	case strings.Contains(msg, "gRPC endpoint disabled"):
		out.Msg = "qdrant.grpc.endpoint_disabled"
		out.ProgressDetail = msg
		return true
	case strings.Contains(msg, "Qdrant internal gRPC listening"):
		out.Msg = "qdrant.listen.internal_grpc"
		if m := internalGRPCListenPortRE.FindStringSubmatch(msg); len(m) == 2 {
			if p, err := strconv.Atoi(m[1]); err == nil {
				out.InternalGRPCPort = p
			}
		}
		out.ProgressDetail = msg
		return true
	case strings.Contains(msg, "Stopping ") && (strings.Contains(msg, " on SIGINT") || strings.Contains(msg, " on SIGTERM")):
		out.Msg = "qdrant.process.shutdown_signal"
		out.ProgressDetail = msg
		return true
	case strings.HasPrefix(strings.TrimSpace(msg), "Feature flags:"):
		out.Msg = "qdrant.debug.feature_flags"
		out.ProgressDetail = msg
		return true
	case strings.HasPrefix(msgTrim, "Loaded collection:"):
		out.Msg = "qdrant.debug.collection_loaded"
		out.ProgressDetail = msg
		return true
	case strings.Contains(msg, "TLS enabled for REST API"):
		out.Msg = "qdrant.listen.tls_enabled_rest"
		out.QdrantTLSREST = "enabled"
		return true
	case strings.Contains(msg, "TLS enabled for gRPC API"):
		out.Msg = "qdrant.listen.tls_enabled_grpc"
		out.QdrantTLSGRPC = "enabled"
		return true
	default:
		return false
	}
}

func fallbackUnknown(raw, level, target, detail string) []byte {
	out := normalized{
		Msg:            "qdrant.unparsed",
		Service:        "qdrant",
		Level:          level,
		QdrantTarget:   target,
		ProgressDetail: trimRunes(raw, 2048),
		ClaudiaNorm:    1,
	}
	if detail != "" {
		out.ProgressDetail = detail
	}
	b, _ := json.Marshal(out)
	return b
}

func trimRunes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// isBootstrapURIDuplicateWarning matches Qdrant cluster bootstrap URI / peer mismatch hints.
func isBootstrapURIDuplicateWarning(msg string) bool {
	lower := strings.ToLower(msg)
	if !strings.Contains(lower, "bootstrap uri") {
		return false
	}
	return strings.Contains(lower, "same") ||
		strings.Contains(lower, "equal") ||
		strings.Contains(lower, "peer") ||
		strings.Contains(lower, "duplicate")
}
