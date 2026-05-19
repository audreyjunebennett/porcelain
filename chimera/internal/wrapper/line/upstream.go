package line

import (
	"encoding/json"
	"strings"
)

// UpstreamDetailFromFields returns wrapper-captured upstream text from slog JSON fields.
func UpstreamDetailFromFields(fields map[string]json.RawMessage) string {
	if s := strings.TrimSpace(JSONString(fields, "upstream_raw")); s != "" {
		return s
	}
	return strings.TrimSpace(JSONString(fields, "upstream_wrapped"))
}

// IsUpstreamLineMsg reports wrapper forward lines (e.g. broker.upstream.line).
func IsUpstreamLineMsg(msg string) bool {
	return strings.HasSuffix(strings.TrimSpace(msg), ".upstream.line")
}
