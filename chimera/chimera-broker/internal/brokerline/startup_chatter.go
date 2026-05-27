package brokerline

import (
	"encoding/json"
	"strings"
	"sync"

	wline "github.com/lynn/porcelain/chimera/internal/wrapper/line"
)

var (
	brokerBannerMu          sync.Mutex
	brokerBannerInfoEmitted bool
)

func resetBrokerStartupChatterForTest() {
	brokerBannerMu.Lock()
	brokerBannerInfoEmitted = false
	brokerBannerMu.Unlock()
}

func postProcessNormalizedLine(b []byte) []byte {
	if len(b) == 0 {
		return b
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(b, &fields); err != nil {
		return b
	}

	slug := strings.TrimSpace(wline.JSONString(fields, "msg"))
	level := strings.ToUpper(strings.TrimSpace(wline.JSONString(fields, "level")))
	detail := wline.JSONString(fields, "progress_detail")

	switch slug {
	case "broker.version":
		resetBrokerStartupBanner()
	case "broker.startup.banner":
		if level == "DEBUG" {
			return b
		}
		brokerBannerMu.Lock()
		first := !brokerBannerInfoEmitted
		if first {
			brokerBannerInfoEmitted = true
		}
		brokerBannerMu.Unlock()
		if first {
			return brokerStartupBannerMarkerLine(b, fields)
		}
		return demoteLineLevel(b, fields, "DEBUG")
	case "broker.config.schema_warn":
		if level == "WARN" || level == "ERROR" {
			if isDecorativeBannerDetail(detail) {
				return demoteLineLevel(b, fields, "DEBUG")
			}
			return b
		}
		if isDecorativeBannerDetail(detail) {
			return demoteLineLevel(b, fields, "DEBUG")
		}
		return b
	}

	if level == "ERROR" || level == "WARN" {
		return b
	}
	if brokerRoutineDebugSlug(slug, fields) {
		return demoteLineLevel(b, fields, "DEBUG")
	}
	return b
}

func resetBrokerStartupBanner() {
	brokerBannerMu.Lock()
	brokerBannerInfoEmitted = false
	brokerBannerMu.Unlock()
}

func brokerStartupBannerMarkerLine(b []byte, fields map[string]json.RawMessage) []byte {
	var m map[string]any
	if json.Unmarshal(b, &m) != nil {
		return demoteLineLevel(b, fields, "INFO")
	}
	m["level"] = "INFO"
	delete(m, "progress_detail")
	out, err := json.Marshal(m)
	if err != nil {
		return b
	}
	return wline.ReorderNormalizedJSONBytes(out)
}

func brokerRoutineDebugSlug(slug string, fields map[string]json.RawMessage) bool {
	switch slug {
	case "broker.store.config_ready",
		"broker.store.request_logs_ready",
		"broker.auth.token_refresh",
		"broker.mcp.startup",
		"broker.governance.startup",
		"broker.jobs.async_ready",
		"broker.client.ready",
		"broker.config.loaded",
		"broker.maintenance.log_retention",
		"broker.log.zerolog",
		"broker.listen.http":
		return true
	case "broker.catalog.sync":
		return wline.IntFromJSON(fields, "catalog_model_count") == 0
	default:
		return false
	}
}

func isDecorativeBannerDetail(detail string) bool {
	detail = strings.TrimSpace(detail)
	if detail == "" {
		return false
	}
	return looksLikeBannerOrSchemaBox(detail)
}

func demoteLineLevel(b []byte, fields map[string]json.RawMessage, level string) []byte {
	var m map[string]any
	if fields != nil {
		if json.Unmarshal(b, &m) != nil {
			return b
		}
	} else if json.Unmarshal(b, &m) != nil {
		return b
	}
	m["level"] = level
	out, err := json.Marshal(m)
	if err != nil {
		return b
	}
	return wline.ReorderNormalizedJSONBytes(out)
}
