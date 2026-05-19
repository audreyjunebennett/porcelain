// Package brokerline normalizes raw chimera-broker process output into JSON lines with stable
// broker.* msg slugs and structured fields for the operator logs UI.
//
// Operator-facing copy for these slugs lives in internal/operatorcopy/messages.yaml
// (legacy alias chimera-broker.* until 2026-08-01). Do not add prose here.
package brokerline

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"

	wline "github.com/lynn/porcelain/chimera/internal/wrapper/line"
	"github.com/lynn/porcelain/internal/naming"
)

var (
	bootstrapMSRE   = regexp.MustCompile(`(?i)Time spent in ` + regexp.QuoteMeta(naming.ProductBrokerName) + ` server bootstrap\s+(\d+)\s*ms`)
	readyURLRE      = regexp.MustCompile(`(?i)successfully started ` + regexp.QuoteMeta(naming.ProductBrokerName) + `,\s*serving UI on\s+(https?://\S+)`)
	pluginStatusRE  = regexp.MustCompile(`(?i)plugin status:\s*([^-]+)\s*-\s*(.+)\s*$`)
	addedProviderRE = regexp.MustCompile(`(?i)added provider:\s*(\S+)`)
	logRetentionRE  = regexp.MustCompile(`(?i)log retention cleaner initialized with\s+(\d+)\s*days retention`)
	retentionDaysRE = regexp.MustCompile(`(?i)log retention days:\s*(\d+)`)
	// Catalog lines that include a numeric model count for the logs UI "Available models" card.
	catalogModelsAddedRE   = regexp.MustCompile(`(?i)(\d+)\s+models?\s+added\s+to\s+(?:the\s+)?catalog`)
	catalogModelsRegRE     = regexp.MustCompile(`(?i)\b(\d+)\s+models?\s+registered\b`)
	catalogListingModelsRE = regexp.MustCompile(`(?i)listing\s+(\d+)\s+models?\b`)
	catalogPoolModelsRE    = regexp.MustCompile(`(?i)populated\s+model\s+pool[^\n\d]{0,160}(\d+)\s+models?\b`)
)

type normalized struct {
	Timestamp            string  `json:"timestamp,omitempty"`
	Level                string  `json:"level,omitempty"`
	Service              string  `json:"service"`
	Msg                  string  `json:"msg"`
	ProgressDetail       string  `json:"progress_detail,omitempty"`
	HTTPMethod           string  `json:"http_method,omitempty"`
	HTTPTarget           string  `json:"http_target,omitempty"`
	HTTPStatus           int     `json:"http_status,omitempty"`
	HTTPDurationMS       float64 `json:"http_duration_ms,omitempty"`
	TraceID              string  `json:"trace_id,omitempty"`
	ChimeraBrokerVersion string  `json:"chimera_broker_version,omitempty"`
	ListenPort           int     `json:"listen_port,omitempty"`
	ListenURL            string  `json:"listen_url,omitempty"`
	ProviderID           string  `json:"provider_id,omitempty"`
	PluginName           string  `json:"plugin_name,omitempty"`
	PluginStatus         string  `json:"plugin_status,omitempty"`
	BootstrapMS          int     `json:"bootstrap_ms,omitempty"`
	LogRetentionDays     int     `json:"log_retention_days,omitempty"`
	CatalogModelCount    int     `json:"catalog_model_count,omitempty"`
	ChimeraNorm          int     `json:"_chimera_norm,omitempty"`
}

// NormalizePayload converts one raw line (no trailing \n) into a single JSON log line.
func NormalizePayload(raw string) []byte {
	return wline.NormalizePerLine(raw, alreadyNormalized, normalizePlain, normalizeJSON)
}

func normalizeJSON(raw string) []byte {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &fields); err != nil {
		return fallbackUnknown(raw, "", "", "")
	}

	if wline.IntFromJSON(fields, "_chimera_norm") == wline.ChimeraNormValue {
		if b, ok := wline.ReorderNormalizedJSON([]byte(raw)); ok {
			return b
		}
	}

	level := wline.JSONString(fields, "level")
	ts := wline.JSONString(fields, "time")
	if strings.TrimSpace(ts) == "" {
		ts = wline.JSONString(fields, "timestamp")
	}
	ts = wline.NormalizeTimestampUTC(ts)
	message := strings.TrimSpace(wline.JSONString(fields, "message"))
	if message == "" {
		message = strings.TrimSpace(wline.JSONString(fields, "msg"))
	}

	if isBrokerDomainSlug(message) {
		return normalizeBrokerDomainSlog(fields, message, raw, level, ts)
	}

	out := normalized{
		Timestamp:   ts,
		Level:       strings.ToUpper(level),
		Service:     naming.ProductBrokerName,
		ChimeraNorm: 1,
	}

	if msg, handled := classifyHTTPAccess(&out, fields, message); handled {
		out.Msg = msg
		b, err := json.Marshal(out)
		if err != nil {
			return fallbackUnknown(raw, level, "", message)
		}
		return b
	}

	lmsg := strings.ToLower(message)

	switch {
	case strings.Contains(message, "config validation failed") || strings.Contains(message, "schema validation failed"):
		out.Msg = "broker.config.validation_failed"
		out.ProgressDetail = wline.TrimRunes(message, 4096)
	case strings.Contains(message, "does not include") && strings.Contains(lmsg, "schema"):
		out.Msg = "broker.config.schema_warn"
		out.ProgressDetail = wline.TrimRunes(message, 2048)
	case strings.HasPrefix(strings.ToLower(strings.TrimSpace(message)), "loading configuration from"):
		out.Msg = "broker.config.loaded"
		out.ProgressDetail = wline.TrimRunes(message, 512)
	case strings.Contains(strings.ToLower(message), "config store initialized"):
		out.Msg = "broker.store.config_ready"
		if strings.Contains(strings.ToLower(message), "memory") {
			out.ProgressDetail = "memory"
		} else if strings.Contains(strings.ToLower(message), "sqlite") {
			out.ProgressDetail = "sqlite"
		}
	case strings.Contains(strings.ToLower(message), "logs store initialized"):
		out.Msg = "broker.store.request_logs_ready"
	case strings.Contains(message, "Token refresh worker started"):
		out.Msg = "broker.auth.token_refresh"
	case strings.Contains(strings.ToLower(message), "initializing model catalog"):
		out.Msg = "broker.catalog.sync"
	case strings.Contains(strings.ToLower(message), "successfully synced") && strings.Contains(strings.ToLower(message), "pricing"):
		out.Msg = "broker.catalog.sync"
		out.ProgressDetail = wline.TrimRunes(message, 256)
	case strings.Contains(strings.ToLower(message), "populated model pool"):
		out.Msg = "broker.catalog.sync"
	case strings.Contains(strings.ToLower(message), "initializing mcp catalog"):
		out.Msg = "broker.mcp.startup"
	case strings.Contains(strings.ToLower(message), "log retention days:"):
		out.Msg = "broker.maintenance.log_retention"
		if m := retentionDaysRE.FindStringSubmatch(message); len(m) == 2 {
			if n, err := strconv.Atoi(m[1]); err == nil {
				out.LogRetentionDays = n
			}
		}
	case strings.Contains(strings.ToLower(message), "log cleanup routine started"):
		out.Msg = "broker.maintenance.log_retention"
		out.ProgressDetail = "cleanup_routine_started"
	case strings.Contains(strings.ToLower(message), "log retention cleaner initialized"):
		out.Msg = "broker.maintenance.log_retention"
		if m := logRetentionRE.FindStringSubmatch(message); len(m) == 2 {
			if n, err := strconv.Atoi(m[1]); err == nil {
				out.LogRetentionDays = n
			}
		}
	case strings.Contains(strings.ToLower(message), "starting log cleanup"):
		out.Msg = "broker.maintenance.log_retention"
		out.ProgressDetail = "cleanup_run"
	case strings.Contains(strings.ToLower(message), "governance store initialized"):
		out.Msg = "broker.governance.startup"
	case strings.Contains(strings.ToLower(message), "async job executor initialized"):
		out.Msg = "broker.jobs.async_ready"
	case strings.Contains(strings.ToLower(message), naming.ProductBrokerName+" client initialized"):
		out.Msg = "broker.client.ready"
	case strings.Contains(strings.ToLower(message), "listing all models and adding to model catalog"),
		strings.Contains(strings.ToLower(message), "models added to catalog"):
		out.Msg = "broker.catalog.sync"
	case strings.Contains(strings.ToLower(message), "model-parameters-sync"):
		out.Msg = "broker.catalog.sync"
	case bootstrapMSRE.MatchString(message):
		out.Msg = "broker.bootstrap.complete"
		if sm := bootstrapMSRE.FindStringSubmatch(message); len(sm) == 2 {
			if n, err := strconv.Atoi(sm[1]); err == nil {
				out.BootstrapMS = n
			}
		}
	case pluginStatusRE.MatchString(message):
		out.Msg = "broker.plugin.status"
		if sm := pluginStatusRE.FindStringSubmatch(message); len(sm) == 3 {
			out.PluginName = strings.TrimSpace(sm[1])
			out.PluginStatus = strings.TrimSpace(sm[2])
		}
	case readyURLRE.MatchString(message):
		out.Msg = "broker.ready"
		if sm := readyURLRE.FindStringSubmatch(message); len(sm) == 2 {
			u := strings.TrimSpace(sm[1])
			out.ListenURL = u
			if p := wline.PortFromURL(u); p > 0 {
				out.ListenPort = p
			}
		}
		if out.ListenPort > 0 {
			out.ProgressDetail = "serving_ui"
		}
	case strings.Contains(strings.ToLower(message), "http listening"),
		strings.Contains(strings.ToLower(message), "server started"):
		out.Msg = "broker.listen.http"
		out.ProgressDetail = wline.TrimRunes(message, 512)
	case addedProviderRE.MatchString(message):
		out.Msg = "broker.provider.loaded"
		if sm := addedProviderRE.FindStringSubmatch(message); len(sm) == 2 {
			out.ProviderID = strings.TrimSpace(sm[1])
		}
	case strings.Contains(strings.ToLower(message), "updating provider configuration"),
		strings.Contains(strings.ToLower(message), "updated configuration for provider"),
		strings.Contains(strings.ToLower(message), "successfully updated provider configuration"):
		out.Msg = "broker.provider.loaded"
		if id := providerIDFromUpdateMessage(message); id != "" {
			out.ProviderID = id
		}
	default:
		out.Msg = "broker.log.zerolog"
		out.ProgressDetail = wline.TrimRunes(message, 2048)
	}

	if out.Msg == "broker.catalog.sync" {
		if n := catalogModelCountFromMessage(message); n > 0 {
			out.CatalogModelCount = n
		}
	}

	b, err := json.Marshal(out)
	if err != nil {
		return fallbackUnknown(raw, level, "", message)
	}
	return b
}

func catalogModelCountFromMessage(msg string) int {
	lm := strings.ToLower(strings.TrimSpace(msg))
	if strings.Contains(lm, "pricing") && !strings.Contains(lm, "model") {
		return 0
	}
	for _, re := range []*regexp.Regexp{
		catalogModelsAddedRE,
		catalogModelsRegRE,
		catalogListingModelsRE,
		catalogPoolModelsRE,
	} {
		if m := re.FindStringSubmatch(msg); len(m) == 2 {
			if n, err := strconv.Atoi(m[1]); err == nil && n > 0 && n < 10_000_000 {
				return n
			}
		}
	}
	return 0
}

var providerUpdateRE = regexp.MustCompile(`(?i)(?:for\s+provider\s+([a-z0-9_.-]+)|provider:\s*([a-z0-9_.-]+))`)

func providerIDFromUpdateMessage(s string) string {
	m := providerUpdateRE.FindAllStringSubmatch(s, -1)
	if len(m) == 0 {
		return ""
	}
	last := m[len(m)-1]
	if last[1] != "" {
		return strings.TrimSpace(last[1])
	}
	return strings.TrimSpace(last[2])
}

func classifyHTTPAccess(out *normalized, fields map[string]json.RawMessage, message string) (string, bool) {
	if strings.TrimSpace(message) != "request completed" {
		return "", false
	}
	method := wline.JSONString(fields, "http.method")
	target := wline.JSONString(fields, "http.target")
	if method == "" && target == "" {
		return "", false
	}
	status := wline.IntFromJSON(fields, "http.status_code")
	dur := wline.FloatFromJSON(fields, "http.request_duration_ms")
	out.HTTPMethod = method
	out.HTTPTarget = target
	out.HTTPStatus = status
	out.HTTPDurationMS = dur
	out.TraceID = wline.JSONString(fields, "trace_id")

	if status == 429 {
		return "broker.rate_limit", true
	}
	return "broker.http.access", true
}

func isBrokerDomainSlug(s string) bool {
	s = strings.TrimSpace(s)
	return strings.HasPrefix(s, "broker.")
}

func normalizeBrokerDomainSlog(fields map[string]json.RawMessage, slug, raw, level, ts string) []byte {
	out := normalized{
		Timestamp:   ts,
		Level:       strings.ToUpper(strings.TrimSpace(level)),
		Service:     naming.ProductBrokerName,
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
	b, err := json.Marshal(out)
	if err != nil {
		return fallbackUnknown(raw, level, slug, slug)
	}
	return b
}

func normalizePlain(raw string) []byte {
	s := strings.TrimSpace(raw)
	out := normalized{
		Timestamp:   wline.UTCTimestampNow(),
		Service:     naming.ProductBrokerName,
		Level:       "INFO",
		ChimeraNorm: 1,
	}

	switch {
	case strings.Contains(strings.ToLower(s), "maxprocs") || strings.Contains(strings.ToLower(s), "gomaxprocs"):
		out.Msg = "broker.startup.banner"
		out.ProgressDetail = wline.TrimRunes(s, 512)
	case strings.Contains(s, "vdev-build") || plainVersionRE.MatchString(s):
		out.Msg = "broker.version"
		if strings.Contains(s, "vdev-build") {
			out.ChimeraBrokerVersion = "vdev-build"
		} else if m := plainVersionRE.FindStringSubmatch(s); len(m) == 2 {
			out.ChimeraBrokerVersion = strings.TrimSpace(m[1])
		}
	case looksLikeBannerOrSchemaBox(s):
		if strings.Contains(strings.ToLower(s), "schema") || strings.Contains(strings.ToLower(s), "config file") {
			out.Msg = "broker.config.schema_warn"
		} else {
			out.Msg = "broker.startup.banner"
		}
		out.ProgressDetail = wline.TrimRunes(s, 512)
	case strings.Contains(strings.ToLower(s), "error when serving connection") ||
		strings.Contains(strings.ToLower(s), "wsasend") ||
		strings.Contains(strings.ToLower(s), "connection was aborted"):
		out.Msg = "broker.transport.serve_error"
		out.Level = "ERROR"
		out.ProgressDetail = wline.TrimRunes(s, 2048)
	default:
		out.Msg = "broker.startup.banner"
		out.ProgressDetail = wline.TrimRunes(s, 512)
	}

	b, _ := json.Marshal(out)
	return b
}

var plainVersionRE = regexp.MustCompile(`(?i)version[:\s]+([0-9a-zA-Z._\-+]+)`)

func looksLikeBannerOrSchemaBox(s string) bool {
	if strings.ContainsAny(s, "╔║╚═╗╝") || strings.Contains(s, "█") {
		return true
	}
	return strings.Contains(s, "[33m") || strings.Contains(s, "\x1b[")
}

func alreadyNormalized(raw []byte) ([]byte, bool) {
	if b, ok := wline.ReorderNormalizedJSON(raw); ok {
		return b, true
	}
	if _, ok := wline.AlreadyNormalizedChimera(raw, "broker.", naming.ProductBrokerName); ok {
		return wline.ReorderNormalizedJSON(raw)
	}
	if b, ok := wline.PassthroughSlogJSON(raw, naming.ProductBrokerName); ok {
		return b, true
	}
	return nil, false
}

func fallbackUnknown(raw, level, _, _ string) []byte {
	out := normalized{
		Msg:            "broker.unparsed",
		Service:        naming.ProductBrokerName,
		Level:          strings.ToUpper(level),
		ProgressDetail: wline.TrimRunes(raw, 4096),
		ChimeraNorm:    1,
	}
	b, _ := json.Marshal(out)
	return b
}
