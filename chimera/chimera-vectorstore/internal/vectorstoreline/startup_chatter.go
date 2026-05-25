package vectorstoreline

import (
	"encoding/json"
	"strings"
	"sync"

	wline "github.com/lynn/porcelain/chimera/internal/wrapper/line"
)

var (
	vectorstoreBannerMu          sync.Mutex
	vectorstoreBannerInfoEmitted bool
)

func resetVectorstoreStartupChatterForTest() {
	vectorstoreBannerMu.Lock()
	vectorstoreBannerInfoEmitted = false
	vectorstoreBannerMu.Unlock()
}

func postProcessStartupChatterLine(b []byte) []byte {
	if len(b) == 0 {
		return b
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(b, &fields); err != nil {
		return b
	}

	slug := strings.TrimSpace(wline.JSONString(fields, "msg"))
	level := strings.ToUpper(strings.TrimSpace(wline.JSONString(fields, "level")))

	switch slug {
	case "vectorstore.version":
		resetVectorstoreStartupBanner()
	case "vectorstore.startup.banner":
		if level == "DEBUG" {
			return b
		}
		vectorstoreBannerMu.Lock()
		first := !vectorstoreBannerInfoEmitted
		if first {
			vectorstoreBannerInfoEmitted = true
		}
		vectorstoreBannerMu.Unlock()
		if first {
			return vectorstoreStartupBannerMarkerLine(b, fields)
		}
		return demoteLineLevel(b, fields, "DEBUG")
	}

	if level == "ERROR" || level == "WARN" {
		return b
	}
	if vectorstoreRoutineDebugSlug(slug, fields) {
		return demoteLineLevel(b, fields, "DEBUG")
	}
	return b
}

func resetVectorstoreStartupBanner() {
	vectorstoreBannerMu.Lock()
	vectorstoreBannerInfoEmitted = false
	vectorstoreBannerMu.Unlock()
}

func vectorstoreStartupBannerMarkerLine(b []byte, fields map[string]json.RawMessage) []byte {
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

func vectorstoreRoutineDebugSlug(slug string, fields map[string]json.RawMessage) bool {
	detail := wline.JSONString(fields, "progress_detail")
	switch slug {
	case "vectorstore.actix.workers",
		"vectorstore.actix.bind",
		"vectorstore.listen.tls_disabled_rest",
		"vectorstore.listen.tls_disabled_grpc",
		"vectorstore.listen.tls_enabled_rest",
		"vectorstore.listen.tls_enabled_grpc",
		"vectorstore.listen.http",
		"vectorstore.listen.grpc",
		"vectorstore.listen.internal_grpc",
		"vectorstore.telemetry.enabled",
		"vectorstore.telemetry.disabled",
		"vectorstore.inference.disabled",
		"vectorstore.inference.configured",
		"vectorstore.web_ui_hint",
		"vectorstore.cluster.single_node",
		"vectorstore.consensus.raft_load",
		"vectorstore.ui.static_missing",
		"vectorstore.config.optional_missing":
		return true
	case "vectorstore.shard.recover_progress":
		return shardRecoverProgressIsZero(detail)
	default:
		return false
	}
}

func shardRecoverProgressIsZero(detail string) bool {
	d := strings.ToLower(strings.TrimSpace(detail))
	if d == "" {
		return false
	}
	return strings.Contains(d, "(0%)") || strings.Contains(d, " 0/") || strings.HasSuffix(d, " 0%")
}
