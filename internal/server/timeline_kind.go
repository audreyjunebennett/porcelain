package server

import "strings"

// timelineKindForGatewayHTTPPath classifies inbound gateway HTTP access lines for the logs UI
// request-timeline bar (internal/server/embedui/logs.js TIMELINE_BAR_KINDS). Values: web, qdrant,
// upstream, indexer, gateway.
func timelineKindForGatewayHTTPPath(path string) string {
	if path == "" {
		return "web"
	}
	if strings.HasPrefix(path, "/v1/ingest") || strings.HasPrefix(path, "/v1/indexer") {
		return "indexer"
	}
	if path == "/v1/chat/completions" || path == "/v1/models" || path == "/ui/models" {
		return "upstream"
	}
	// Future gateway-exposed Qdrant proxy or passthrough; also matches raw collection REST paths.
	if strings.HasPrefix(path, "/v1/qdrant") || strings.Contains(path, "/collections/") {
		return "qdrant"
	}
	return "web"
}
