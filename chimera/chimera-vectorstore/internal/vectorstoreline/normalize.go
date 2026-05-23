// Package vectorstoreline normalizes raw Qdrant process output into JSON lines with stable
// vectorstore.* msg slugs and structured fields for the operator settings UI (/ui/settings).
//
// Operator-facing copy for these slugs lives in internal/operatorcopy/messages.yaml
// (legacy alias qdrant.* until 2026-08-01). Do not add prose here.
package vectorstoreline

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"

	wline "github.com/lynn/porcelain/chimera/internal/wrapper/line"
	"github.com/lynn/porcelain/internal/naming"
)

var accessRE = regexp.MustCompile(`"(?P<method>GET|POST|PUT|PATCH|DELETE|HEAD)\s+(?P<path>[^"]+)\s+HTTP/1\.1"\s+(?P<status>\d{3})`)

var collectionPathRE = regexp.MustCompile(`(?i)/collections/([^/?#]+)`)

var recoveredCollectionRE = regexp.MustCompile(`(?i)Recovered collection\s+([^:]+)`)

var loadingCollectionRE = regexp.MustCompile(`(?i)Loading collection:\s*(.+)\s*$`)

var creatingCollectionRE = regexp.MustCompile(`(?i)Creating collection\s+(.+?)\s*$`)

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
	Service           string `json:"service"`
	Msg               string `json:"msg"`
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
	ChimeraNorm       int    `json:"_chimera_norm,omitempty"`
}

// NormalizePayload converts one raw line (no trailing \n) into a single JSON log line.
func NormalizePayload(raw string) []byte {
	return wline.NormalizePerLine(raw, alreadyNormalized, normalizePlain, normalizeJSON)
}

func normalizeJSON(raw string) []byte {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &fields); err == nil {
		if wline.IntFromJSON(fields, "_chimera_norm") == wline.ChimeraNormValue {
			if b, ok := wline.ReorderNormalizedJSON([]byte(raw)); ok {
				return b
			}
		}
		slug := strings.TrimSpace(wline.JSONString(fields, "msg"))
		if slug == "" {
			slug = strings.TrimSpace(wline.JSONString(fields, "message"))
		}
		if strings.HasPrefix(slug, "vectorstore.") {
			return normalizeVectorstoreDomainSlog(fields, slug, raw)
		}
	}

	var parsed rustTracingLine
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return fallbackUnknown(raw, parsed.Level, "", "")
	}
	msg := strings.TrimSpace(parsed.Fields.Message)
	tgt := strings.TrimSpace(parsed.Target)
	out := normalized{
		Timestamp:    wline.NormalizeTimestampUTC(parsed.Timestamp),
		Level:        parsed.Level,
		Service:      naming.ProductVectorstoreName,
		ChimeraNorm:  1,
		QdrantTarget: tgt,
	}

	switch {
	case tgt == "qdrant::settings" && strings.Contains(msg, "Config file not found"):
		out.Msg = "vectorstore.config.optional_missing"
		out.QdrantConfig = "supervised"
	case tgt == "storage::content_manager::consensus::persistent" && strings.Contains(msg, "raft"):
		out.Msg = "vectorstore.consensus.raft_load"
	case tgt == "storage::content_manager::toc" && strings.HasPrefix(strings.TrimSpace(msg), "Loading collection:"):
		out.Msg = "vectorstore.collection.loading"
		if m := loadingCollectionRE.FindStringSubmatch(msg); len(m) == 2 {
			out.Collection = strings.TrimSpace(m[1])
		}
	case strings.Contains(tgt, "collection_meta_ops") && strings.HasPrefix(strings.TrimSpace(msg), "Creating collection"):
		out.Msg = "vectorstore.collection.creating"
		if m := creatingCollectionRE.FindStringSubmatch(msg); len(m) == 2 {
			out.Collection = strings.TrimSpace(m[1])
		}
	case tgt == "collection::shards::local_shard" && strings.Contains(msg, "Recovering shard"):
		out.Msg = "vectorstore.shard.recover_progress"
		out.ProgressDetail = msg
		if m := recoveringShardCollRE.FindStringSubmatch(msg); len(m) == 2 {
			out.Collection = strings.TrimSpace(m[1])
		}
	case tgt == "collection::shards::local_shard" && strings.Contains(msg, "Recovered collection"):
		out.Msg = "vectorstore.shard.recovered"
		out.ProgressDetail = msg
		if m := recoveredCollectionRE.FindStringSubmatch(msg); len(m) == 2 {
			out.Collection = strings.TrimSpace(m[1])
		}
	case tgt == naming.ProductQdrantBinName && strings.Contains(msg, "Distributed mode disabled"):
		out.Msg = "vectorstore.cluster.single_node"
		out.QdrantMode = "single-node"
	case tgt == "qdrant::actix::web_ui":
		out.Msg = "vectorstore.ui.static_missing"
	case tgt == "qdrant::actix" && strings.Contains(strings.ToUpper(msg), "TLS DISABLED"):
		out.Msg = "vectorstore.listen.tls_disabled_rest"
		out.QdrantTLSREST = "disabled"
	case tgt == "qdrant::actix" && strings.Contains(msg, "TLS enabled for REST API"):
		out.Msg = "vectorstore.listen.tls_enabled_rest"
		out.QdrantTLSREST = "enabled"
	case tgt == "qdrant::actix" && (strings.Contains(msg, "HTTP listening") || strings.Contains(msg, "Qdrant HTTP listening")):
		out.Msg = "vectorstore.listen.http"
		if m := httpListenPortRE.FindStringSubmatch(msg); len(m) == 2 {
			if p, err := strconv.Atoi(m[1]); err == nil {
				out.RESTPort = p
			}
		}
	case tgt == "actix_server::builder":
		out.Msg = "vectorstore.actix.workers"
	case tgt == "actix_server::server":
		out.Msg = "vectorstore.actix.bind"
	case tgt == "qdrant::tonic" && strings.Contains(msg, "internal gRPC listening"):
		out.Msg = "vectorstore.listen.internal_grpc"
		if m := internalGRPCListenPortRE.FindStringSubmatch(msg); len(m) == 2 {
			if p, err := strconv.Atoi(m[1]); err == nil {
				out.InternalGRPCPort = p
			}
		}
	case tgt == "qdrant::tonic" && strings.Contains(msg, "gRPC listening"):
		out.Msg = "vectorstore.listen.grpc"
		if m := grpcListenPortRE.FindStringSubmatch(msg); len(m) == 2 {
			if p, err := strconv.Atoi(m[1]); err == nil {
				out.GRPCPort = p
			}
		}
	case tgt == "qdrant::tonic" && strings.Contains(strings.ToUpper(msg), "TLS DISABLED"):
		out.Msg = "vectorstore.listen.tls_disabled_grpc"
		out.QdrantTLSGRPC = "disabled"
	case tgt == "qdrant::tonic" && strings.Contains(msg, "TLS enabled for gRPC API"):
		out.Msg = "vectorstore.listen.tls_enabled_grpc"
		out.QdrantTLSGRPC = "enabled"
	case tgt == "actix_web::middleware::logger":
		return normalizeAccessJSON(parsed, msg, out)
	default:
		if classifyOperatorSignals(&out, msg, tgt) {
			break
		}
		out.Msg = "vectorstore.trace.other"
		out.ProgressDetail = traceDetailFromQdrant(msg, raw)
	}

	ensureVectorstoreTimestamp(&out)
	b, err := json.Marshal(out)
	if err != nil {
		return fallbackUnknown(raw, parsed.Level, tgt, msg)
	}
	return b
}

func alreadyNormalized(raw []byte) ([]byte, bool) {
	if b, ok := wline.ReorderNormalizedJSON(raw); ok {
		return b, true
	}
	if _, ok := wline.AlreadyNormalizedChimera(raw, "vectorstore.", naming.ProductVectorstoreName); ok {
		return wline.ReorderNormalizedJSON(raw)
	}
	if b, ok := wline.PassthroughSlogJSON(raw, naming.ProductVectorstoreName); ok {
		return b, true
	}
	return nil, false
}

func normalizeVectorstoreDomainSlog(fields map[string]json.RawMessage, slug, raw string) []byte {
	ts := wline.NormalizeTimestampUTC(wline.JSONString(fields, "time"))
	if wline.JSONString(fields, "timestamp") != "" {
		ts = wline.NormalizeTimestampUTC(wline.JSONString(fields, "timestamp"))
	}
	out := normalized{
		Timestamp:   ts,
		Level:       strings.ToUpper(strings.TrimSpace(wline.JSONString(fields, "level"))),
		Service:     naming.ProductVectorstoreName,
		Msg:         slug,
		ChimeraNorm: 1,
	}
	if out.Level == "" {
		out.Level = "INFO"
	}
	if detail := wline.UpstreamDetailFromFields(fields); detail != "" {
		out.ProgressDetail = wline.TrimRunes(detail, 2048)
	} else if wline.IsUpstreamLineMsg(slug) {
		out.ProgressDetail = wline.TrimRunes(raw, 2048)
	}
	b, _ := json.Marshal(out)
	return b
}

func traceDetailFromQdrant(msg, raw string) string {
	if s := strings.TrimSpace(msg); s != "" {
		return s
	}
	return wline.TrimRunes(raw, 2048)
}

func ensureVectorstoreTimestamp(out *normalized) {
	if strings.TrimSpace(out.Timestamp) == "" {
		out.Timestamp = wline.UTCTimestampNow()
	} else {
		out.Timestamp = wline.NormalizeTimestampUTC(out.Timestamp)
	}
}

func normalizePlain(raw string) []byte {
	s := strings.TrimSpace(raw)
	out := normalized{
		Timestamp:   wline.UTCTimestampNow(),
		Service:     naming.ProductVectorstoreName,
		Level:       "INFO",
		ChimeraNorm: 1,
	}
	switch {
	case strings.HasPrefix(s, "Version:"):
		out.Msg = "vectorstore.version"
		out.QdrantVersion = strings.TrimSpace(strings.TrimPrefix(s, "Version:"))
	case strings.Contains(s, "Access web UI at"):
		out.Msg = "vectorstore.web_ui_hint"
	default:
		out.Msg = "vectorstore.startup.banner"
		out.ProgressDetail = s
	}
	b, _ := json.Marshal(out)
	return b
}

func normalizeAccessJSON(_ rustTracingLine, msg string, base normalized) []byte {
	m := accessRE.FindStringSubmatch(msg)
	if len(m) != 4 {
		base.Msg = "vectorstore.http.access_other"
		base.ProgressDetail = traceDetailFromQdrant(msg, "")
		base.QdrantTarget = "actix_web::middleware::logger"
		ensureVectorstoreTimestamp(&base)
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
		base.Msg = "vectorstore.http.collection_meta"
	case method == "PUT" && strings.Contains(lowPath, "/collections/") && strings.HasSuffix(lowPath, "/index"):
		if status >= 200 && status < 300 {
			base.Msg = "vectorstore.http.collection_index"
		} else {
			base.Msg = "vectorstore.http.collection_index_rejected"
		}
	case method == "PUT" && strings.Contains(lowPath, "/collections/") && !strings.Contains(lowPath, "/points"):
		if status >= 200 && status < 300 {
			base.Msg = "vectorstore.http.collection_create"
		} else {
			base.Msg = "vectorstore.http.collection_create_rejected"
		}
	case method == "POST" && strings.Contains(lowPath, "/points/delete"):
		base.Msg = "vectorstore.http.points_delete"
	case method == "POST" && strings.Contains(lowPath, "/points/search"):
		base.Msg = "vectorstore.http.vector_search"
	case method == "PUT" && strings.Contains(lowPath, "/points"):
		if status == 200 {
			base.Msg = "vectorstore.http.points_upsert_ok"
		} else {
			base.Msg = "vectorstore.http.points_upsert_rejected"
		}
	default:
		base.Msg = "vectorstore.http.access_other"
	}
	applyVectorstoreHTTPAccessLevel(&base, method, path)
	ensureVectorstoreTimestamp(&base)
	b, _ := json.Marshal(base)
	return b
}

// vectorstoreHTTPAccessLevel returns DEBUG for successful wrapper readiness / health
// probes so default INFO streams (supervisor mirror, settings UI) stay readable.
// Per-file ingest HTTP (collection meta, upserts) is demoted in postProcessNormalizedLine;
// collection_create and vectorstore.http.upsert.summary stay at INFO.
func vectorstoreHTTPAccessLevel(method, path string, status int) string {
	if status < 200 || status >= 300 {
		return ""
	}
	method = strings.ToUpper(strings.TrimSpace(method))
	path = strings.TrimSpace(path)
	if i := strings.Index(path, "?"); i >= 0 {
		path = path[:i]
	}
	if len(path) > 1 && strings.HasSuffix(path, "/") {
		path = strings.TrimRight(path, "/")
	}
	lowPath := strings.ToLower(path)
	switch {
	case method == "GET" && lowPath == "/collections":
		return "DEBUG"
	case method == "GET" && (lowPath == "/health" || lowPath == "/healthz" || lowPath == "/readyz" || lowPath == "/livez"):
		return "DEBUG"
	default:
		return ""
	}
}

func applyVectorstoreHTTPAccessLevel(out *normalized, method, path string) {
	if lvl := vectorstoreHTTPAccessLevel(method, path, out.HTTPStatus); lvl != "" {
		out.Level = lvl
	}
}

// classifyOperatorSignals maps high-signal backend log messages that are not tied to a single
// tracing target. Returns true when out.Msg is set.
func classifyOperatorSignals(out *normalized, msg, _ string) bool {
	msgTrim := strings.TrimSpace(msg)
	switch {
	case strings.Contains(msg, "Panic occurred") || strings.Contains(msg, "Panic backtrace"):
		out.Msg = "vectorstore.runtime.panic"
		out.ProgressDetail = msg
		return true
	case strings.Contains(msg, "Error while starting") && strings.Contains(msg, "server:"):
		out.Msg = "vectorstore.process.server_start_failed"
		out.ProgressDetail = msg
		return true
	case strings.Contains(msg, "Can't initialize GPU"):
		out.Msg = "vectorstore.gpu.init_failed"
		out.ProgressDetail = msg
		return true
	case strings.Contains(msg, "Qdrant is loaded in recovery mode"):
		out.Msg = "vectorstore.storage.recovery_mode"
		out.QdrantRecovery = "active"
		out.ProgressDetail = msg
		return true
	case isBootstrapURIDuplicateWarning(msg):
		out.Msg = "vectorstore.cluster.bootstrap_uri_duplicate"
		out.ProgressDetail = msg
		return true
	case strings.Contains(msg, "Failed to create init file indicator") || strings.Contains(msg, "Failed to remove init file indicator"):
		out.Msg = "vectorstore.runtime.init_file_warning"
		out.ProgressDetail = msg
		return true
	case strings.Contains(msg, "JWT RBAC") ||
		(strings.Contains(strings.ToLower(msg), "jwt") && strings.Contains(strings.ToLower(msg), "api key")):
		out.Msg = "vectorstore.security.jwt_rbac_warning"
		out.ProgressDetail = msg
		return true
	case strings.Contains(msg, "TLS disabled for internal gRPC API"):
		out.Msg = "vectorstore.cluster.internal_tls_disabled"
		out.QdrantInternalTLS = "disabled"
		out.ProgressDetail = msg
		return true
	case strings.Contains(msg, "TLS enabled for internal gRPC API"):
		out.Msg = "vectorstore.cluster.internal_tls_enabled"
		out.QdrantInternalTLS = "enabled"
		out.ProgressDetail = msg
		return true
	case strings.Contains(msg, "Telemetry reporting disabled"):
		out.Msg = "vectorstore.telemetry.disabled"
		out.QdrantTelemetry = "disabled"
		return true
	case strings.Contains(msg, "Telemetry reporting enabled"):
		out.Msg = "vectorstore.telemetry.enabled"
		out.QdrantTelemetry = "enabled"
		return true
	case strings.Contains(msg, "Hardware reporting enabled"):
		out.Msg = "vectorstore.hardware_reporting.enabled"
		out.ProgressDetail = msg
		return true
	case strings.Contains(msg, "Inference service is not configured"):
		out.Msg = "vectorstore.inference.disabled"
		out.ProgressDetail = msg
		return true
	case strings.Contains(msg, "Inference service is configured"):
		out.Msg = "vectorstore.inference.configured"
		out.ProgressDetail = msg
		return true
	case strings.Contains(msg, "gRPC endpoint disabled"):
		out.Msg = "vectorstore.grpc.endpoint_disabled"
		out.ProgressDetail = msg
		return true
	case strings.Contains(msg, "Qdrant internal gRPC listening"):
		out.Msg = "vectorstore.listen.internal_grpc"
		if m := internalGRPCListenPortRE.FindStringSubmatch(msg); len(m) == 2 {
			if p, err := strconv.Atoi(m[1]); err == nil {
				out.InternalGRPCPort = p
			}
		}
		out.ProgressDetail = msg
		return true
	case strings.Contains(msg, "Stopping ") && (strings.Contains(msg, " on SIGINT") || strings.Contains(msg, " on SIGTERM")):
		out.Msg = "vectorstore.process.shutdown_signal"
		out.ProgressDetail = msg
		return true
	case strings.HasPrefix(strings.TrimSpace(msg), "Feature flags:"):
		out.Msg = "vectorstore.debug.feature_flags"
		out.ProgressDetail = msg
		return true
	case strings.HasPrefix(msgTrim, "Loaded collection:"):
		out.Msg = "vectorstore.debug.collection_loaded"
		out.ProgressDetail = msg
		return true
	case strings.Contains(msg, "TLS enabled for REST API"):
		out.Msg = "vectorstore.listen.tls_enabled_rest"
		out.QdrantTLSREST = "enabled"
		return true
	case strings.Contains(msg, "TLS enabled for gRPC API"):
		out.Msg = "vectorstore.listen.tls_enabled_grpc"
		out.QdrantTLSGRPC = "enabled"
		return true
	default:
		return false
	}
}

func fallbackUnknown(raw, level, target, detail string) []byte {
	out := normalized{
		Msg:            "vectorstore.unparsed",
		Service:        naming.ProductVectorstoreName,
		Level:          level,
		QdrantTarget:   target,
		ProgressDetail: wline.TrimRunes(raw, 2048),
		ChimeraNorm:    1,
	}
	if detail != "" {
		out.ProgressDetail = detail
	}
	b, _ := json.Marshal(out)
	return b
}

// isBootstrapURIDuplicateWarning matches backend cluster bootstrap URI / peer mismatch hints.
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
