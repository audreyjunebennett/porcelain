// Package bifrostline normalizes raw bifrost-http process output into JSON lines with stable
// msg slugs (bifrost.*) and structured fields for the operator logs UI.
package bifrostline

import (
	"encoding/json"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

var (
	bootstrapMSRE   = regexp.MustCompile(`(?i)Time spent in Bifrost server bootstrap\s+(\d+)\s*ms`)
	readyURLRE      = regexp.MustCompile(`(?i)successfully started bifrost,\s*serving UI on\s+(https?://\S+)`)
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
	Timestamp         string  `json:"timestamp,omitempty"`
	Level             string  `json:"level,omitempty"`
	Msg               string  `json:"msg"`
	Service           string  `json:"service"`
	ProgressDetail    string  `json:"progress_detail,omitempty"`
	ClaudiaNorm       int     `json:"_claudia_norm,omitempty"`
	HTTPMethod        string  `json:"http_method,omitempty"`
	HTTPTarget        string  `json:"http_target,omitempty"`
	HTTPStatus        int     `json:"http_status,omitempty"`
	HTTPDurationMS    float64 `json:"http_duration_ms,omitempty"`
	TraceID           string  `json:"trace_id,omitempty"`
	BifrostVersion    string  `json:"bifrost_version,omitempty"`
	ListenPort        int     `json:"listen_port,omitempty"`
	ListenURL         string  `json:"listen_url,omitempty"`
	ProviderID        string  `json:"provider_id,omitempty"`
	PluginName        string  `json:"plugin_name,omitempty"`
	PluginStatus      string  `json:"plugin_status,omitempty"`
	BootstrapMS       int     `json:"bootstrap_ms,omitempty"`
	LogRetentionDays  int     `json:"log_retention_days,omitempty"`
	CatalogModelCount int     `json:"catalog_model_count,omitempty"`
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

	if raw[0] != '{' {
		return normalizePlain(raw)
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &fields); err != nil {
		return fallbackUnknown(raw, "", "", "")
	}

	level := jsonString(fields, "level")
	ts := jsonString(fields, "time")
	message := strings.TrimSpace(jsonString(fields, "message"))
	if message == "" {
		message = strings.TrimSpace(jsonString(fields, "msg"))
	}

	out := normalized{
		Timestamp:   ts,
		Level:       strings.ToUpper(level),
		Service:     "bifrost",
		ClaudiaNorm: 1,
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
		out.Msg = "bifrost.config.validation_failed"
		out.ProgressDetail = trimRunes(message, 4096)
	case strings.Contains(message, "does not include") && strings.Contains(lmsg, "schema"):
		out.Msg = "bifrost.config.schema_warn"
		out.ProgressDetail = trimRunes(message, 2048)
	case strings.HasPrefix(strings.ToLower(strings.TrimSpace(message)), "loading configuration from"):
		out.Msg = "bifrost.config.loaded"
		out.ProgressDetail = trimRunes(message, 512)
	case strings.Contains(strings.ToLower(message), "config store initialized"):
		out.Msg = "bifrost.store.config_ready"
		if strings.Contains(strings.ToLower(message), "memory") {
			out.ProgressDetail = "memory"
		} else if strings.Contains(strings.ToLower(message), "sqlite") {
			out.ProgressDetail = "sqlite"
		}
	case strings.Contains(strings.ToLower(message), "logs store initialized"):
		out.Msg = "bifrost.store.request_logs_ready"
	case strings.Contains(message, "Token refresh worker started"):
		out.Msg = "bifrost.auth.token_refresh"
	case strings.Contains(strings.ToLower(message), "initializing model catalog"):
		out.Msg = "bifrost.catalog.sync"
	case strings.Contains(strings.ToLower(message), "successfully synced") && strings.Contains(strings.ToLower(message), "pricing"):
		out.Msg = "bifrost.catalog.sync"
		out.ProgressDetail = trimRunes(message, 256)
	case strings.Contains(strings.ToLower(message), "populated model pool"):
		out.Msg = "bifrost.catalog.sync"
	case strings.Contains(strings.ToLower(message), "initializing mcp catalog"):
		out.Msg = "bifrost.mcp.startup"
	case strings.Contains(strings.ToLower(message), "log retention days:"):
		out.Msg = "bifrost.maintenance.log_retention"
		if m := retentionDaysRE.FindStringSubmatch(message); len(m) == 2 {
			if n, err := strconv.Atoi(m[1]); err == nil {
				out.LogRetentionDays = n
			}
		}
	case strings.Contains(strings.ToLower(message), "log cleanup routine started"):
		out.Msg = "bifrost.maintenance.log_retention"
		out.ProgressDetail = "cleanup_routine_started"
	case strings.Contains(strings.ToLower(message), "log retention cleaner initialized"):
		out.Msg = "bifrost.maintenance.log_retention"
		if m := logRetentionRE.FindStringSubmatch(message); len(m) == 2 {
			if n, err := strconv.Atoi(m[1]); err == nil {
				out.LogRetentionDays = n
			}
		}
	case strings.Contains(strings.ToLower(message), "starting log cleanup"):
		out.Msg = "bifrost.maintenance.log_retention"
		out.ProgressDetail = "cleanup_run"
	case strings.Contains(strings.ToLower(message), "governance store initialized"):
		out.Msg = "bifrost.governance.startup"
	case strings.Contains(strings.ToLower(message), "async job executor initialized"):
		out.Msg = "bifrost.jobs.async_ready"
	case strings.Contains(strings.ToLower(message), "bifrost client initialized"):
		out.Msg = "bifrost.client.ready"
	case strings.Contains(strings.ToLower(message), "listing all models and adding to model catalog"),
		strings.Contains(strings.ToLower(message), "models added to catalog"):
		out.Msg = "bifrost.catalog.sync"
	case strings.Contains(strings.ToLower(message), "model-parameters-sync"):
		out.Msg = "bifrost.catalog.sync"
	case bootstrapMSRE.MatchString(message):
		out.Msg = "bifrost.bootstrap.complete"
		if sm := bootstrapMSRE.FindStringSubmatch(message); len(sm) == 2 {
			if n, err := strconv.Atoi(sm[1]); err == nil {
				out.BootstrapMS = n
			}
		}
	case pluginStatusRE.MatchString(message):
		out.Msg = "bifrost.plugin.status"
		if sm := pluginStatusRE.FindStringSubmatch(message); len(sm) == 3 {
			out.PluginName = strings.TrimSpace(sm[1])
			out.PluginStatus = strings.TrimSpace(sm[2])
		}
	case readyURLRE.MatchString(message):
		out.Msg = "bifrost.ready"
		if sm := readyURLRE.FindStringSubmatch(message); len(sm) == 2 {
			u := strings.TrimSpace(sm[1])
			out.ListenURL = u
			if p := portFromURL(u); p > 0 {
				out.ListenPort = p
			}
		}
		// Also satisfy listen.http-style KV for operators (same line carries bind info).
		if out.ListenPort > 0 {
			out.ProgressDetail = "serving_ui"
		}
	case strings.Contains(strings.ToLower(message), "http listening"),
		strings.Contains(strings.ToLower(message), "server started"):
		out.Msg = "bifrost.listen.http"
		out.ProgressDetail = trimRunes(message, 512)
	case addedProviderRE.MatchString(message):
		out.Msg = "bifrost.provider.loaded"
		if sm := addedProviderRE.FindStringSubmatch(message); len(sm) == 2 {
			out.ProviderID = strings.TrimSpace(sm[1])
		}
	case strings.Contains(strings.ToLower(message), "updating provider configuration"),
		strings.Contains(strings.ToLower(message), "updated configuration for provider"),
		strings.Contains(strings.ToLower(message), "successfully updated provider configuration"):
		out.Msg = "bifrost.provider.loaded"
		if id := providerIDFromUpdateMessage(message); id != "" {
			out.ProviderID = id
		}
	default:
		out.Msg = "bifrost.log.zerolog"
		out.ProgressDetail = trimRunes(message, 2048)
	}

	if out.Msg == "bifrost.catalog.sync" {
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
	method := jsonString(fields, "http.method")
	target := jsonString(fields, "http.target")
	if method == "" && target == "" {
		return "", false
	}
	status := intFromJSON(fields, "http.status_code")
	dur := floatFromJSON(fields, "http.request_duration_ms")
	out.HTTPMethod = method
	out.HTTPTarget = target
	out.HTTPStatus = status
	out.HTTPDurationMS = dur
	out.TraceID = jsonString(fields, "trace_id")

	if status == 429 {
		return "bifrost.rate_limit", true
	}
	return "bifrost.http.access", true
}

func normalizePlain(raw string) []byte {
	s := strings.TrimSpace(raw)
	out := normalized{
		Service:     "bifrost",
		Level:       "INFO",
		ClaudiaNorm: 1,
	}

	switch {
	case strings.Contains(strings.ToLower(s), "maxprocs") || strings.Contains(strings.ToLower(s), "gomaxprocs"):
		out.Msg = "bifrost.startup.banner"
		out.ProgressDetail = trimRunes(s, 512)
	case strings.Contains(s, "vdev-build") || plainVersionRE.MatchString(s):
		out.Msg = "bifrost.version"
		if strings.Contains(s, "vdev-build") {
			out.BifrostVersion = "vdev-build"
		} else if m := plainVersionRE.FindStringSubmatch(s); len(m) == 2 {
			out.BifrostVersion = strings.TrimSpace(m[1])
		}
	case looksLikeBannerOrSchemaBox(s):
		if strings.Contains(strings.ToLower(s), "schema") || strings.Contains(strings.ToLower(s), "config file") {
			out.Msg = "bifrost.config.schema_warn"
		} else {
			out.Msg = "bifrost.startup.banner"
		}
		out.ProgressDetail = trimRunes(s, 512)
	case strings.Contains(strings.ToLower(s), "error when serving connection") ||
		strings.Contains(strings.ToLower(s), "wsasend") ||
		strings.Contains(strings.ToLower(s), "connection was aborted"):
		out.Msg = "bifrost.transport.serve_error"
		out.Level = "ERROR"
		out.ProgressDetail = trimRunes(s, 2048)
	default:
		out.Msg = "bifrost.startup.banner"
		out.ProgressDetail = trimRunes(s, 512)
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
	var ck struct {
		Msg         string `json:"msg"`
		Service     string `json:"service"`
		ClaudiaNorm int    `json:"_claudia_norm"`
	}
	if json.Unmarshal(raw, &ck) != nil {
		return nil, false
	}
	if ck.ClaudiaNorm == 1 && strings.HasPrefix(ck.Msg, "bifrost.") && ck.Service == "bifrost" {
		return raw, true
	}
	return nil, false
}

func fallbackUnknown(raw, level, _, _ string) []byte {
	out := normalized{
		Msg:            "bifrost.unparsed",
		Service:        "bifrost",
		Level:          strings.ToUpper(level),
		ProgressDetail: trimRunes(raw, 4096),
		ClaudiaNorm:    1,
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

func jsonString(fields map[string]json.RawMessage, key string) string {
	raw, ok := fields[key]
	if !ok {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) != nil {
		return ""
	}
	return s
}

func intFromJSON(fields map[string]json.RawMessage, key string) int {
	raw, ok := fields[key]
	if !ok {
		return 0
	}
	var f float64
	if json.Unmarshal(raw, &f) == nil {
		return int(f)
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		n, _ := strconv.Atoi(strings.TrimSpace(s))
		return n
	}
	var n int
	if json.Unmarshal(raw, &n) == nil {
		return n
	}
	return 0
}

func floatFromJSON(fields map[string]json.RawMessage, key string) float64 {
	raw, ok := fields[key]
	if !ok {
		return 0
	}
	var f float64
	if json.Unmarshal(raw, &f) == nil {
		return f
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		x, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
		return x
	}
	return 0
}

func portFromURL(u string) int {
	pu, err := url.Parse(u)
	if err != nil {
		return 0
	}
	if pu.Port() != "" {
		p, _ := strconv.Atoi(pu.Port())
		return p
	}
	switch strings.ToLower(pu.Scheme) {
	case "http":
		return 80
	case "https":
		return 443
	default:
		return 0
	}
}
