package server

import (
	"encoding/json"
	"log/slog"
	"strings"
)

// LogConversationIncomingToolMessages emits conversation.tool.call_* for each OpenAI-style
// tool-role message in the chat completion payload (client-executed tools whose results are
// relayed upstream). The gateway does not execute tools locally; this is the observable relay
// boundary before upstream proxying.
func LogConversationIncomingToolMessages(log *slog.Logger, messages json.RawMessage) {
	if log == nil || len(messages) == 0 {
		return
	}
	var arr []json.RawMessage
	if err := json.Unmarshal(messages, &arr); err != nil {
		return
	}
	for _, raw := range arr {
		var probe struct {
			Role       string          `json:"role"`
			Content    json.RawMessage `json:"content"`
			ToolCallID string          `json:"tool_call_id"`
			Name       string          `json:"name"`
		}
		if err := json.Unmarshal(raw, &probe); err != nil {
			continue
		}
		if strings.TrimSpace(probe.Role) != "tool" {
			continue
		}
		argBytes := len(raw)
		contentBytes := toolMessageContentBytes(probe.Content)
		toolName := strings.TrimSpace(probe.Name)
		toolCallID := strings.TrimSpace(probe.ToolCallID)
		failed := toolRelayContentFailed(probe.Content)

		startArgs := []any{
			"msg", "conversation.tool.call_started",
			"arg_bytes", argBytes,
			"timeline_kind", "upstream",
		}
		startArgs = appendIfNonEmpty(startArgs, "tool_name", toolName)
		startArgs = appendIfNonEmpty(startArgs, "tool_call_id", toolCallID)
		log.Info("conversation tool started", startArgs...)

		if failed {
			errMsg := toolRelayFailureHint(probe.Content)
			failArgs := []any{
				"msg", "conversation.tool.call_failed",
				"latency_ms", int64(0),
				"err", errMsg,
				"timeline_kind", "upstream",
			}
			failArgs = appendIfNonEmpty(failArgs, "tool_name", toolName)
			failArgs = appendIfNonEmpty(failArgs, "tool_call_id", toolCallID)
			log.Warn("conversation tool failed", failArgs...)
			continue
		}
		doneArgs := []any{
			"msg", "conversation.tool.call_completed",
			"latency_ms", int64(0),
			"result_bytes", contentBytes,
			"timeline_kind", "upstream",
		}
		doneArgs = appendIfNonEmpty(doneArgs, "tool_name", toolName)
		doneArgs = appendIfNonEmpty(doneArgs, "tool_call_id", toolCallID)
		log.Info("conversation tool completed", doneArgs...)
	}
}

func appendIfNonEmpty(args []any, key, val string) []any {
	if strings.TrimSpace(val) == "" {
		return args
	}
	return append(args, key, val)
}

func toolMessageContentBytes(content json.RawMessage) int {
	if len(content) == 0 {
		return 0
	}
	var s string
	if json.Unmarshal(content, &s) == nil {
		return len(s)
	}
	return len(content)
}

func toolRelayContentFailed(content json.RawMessage) bool {
	if len(content) == 0 {
		return false
	}
	var s string
	if json.Unmarshal(content, &s) == nil {
		return toolRelayStringFailed(s)
	}
	var obj map[string]json.RawMessage
	if json.Unmarshal(content, &obj) != nil {
		return false
	}
	if raw, ok := obj["is_error"]; ok {
		var b bool
		if json.Unmarshal(raw, &b) == nil && b {
			return true
		}
	}
	if raw, ok := obj["error"]; ok && string(raw) != "null" && len(raw) > 0 {
		return true
	}
	return false
}

func toolRelayFailureHint(content json.RawMessage) string {
	if len(content) == 0 {
		return "tool_result_error"
	}
	var s string
	if json.Unmarshal(content, &s) == nil {
		return truncateStrRunes(strings.TrimSpace(s), 120)
	}
	var wrap struct {
		Error any `json:"error"`
	}
	if json.Unmarshal(content, &wrap) == nil && wrap.Error != nil {
		switch v := wrap.Error.(type) {
		case string:
			return truncateStrRunes(strings.TrimSpace(v), 120)
		default:
			b, _ := json.Marshal(wrap.Error)
			return truncateStrRunes(string(b), 120)
		}
	}
	return "tool_result_error"
}

func truncateStrRunes(s string, max int) string {
	if max <= 0 || s == "" {
		return ""
	}
	n := 0
	for i := range s {
		if n == max {
			return s[:i] + "…"
		}
		n++
	}
	return s
}

func toolRelayStringFailed(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	low := strings.ToLower(s)
	if strings.HasPrefix(low, "error:") || strings.HasPrefix(low, "error calling tool") {
		return true
	}
	if strings.Contains(low, `"is_error":true`) || strings.Contains(low, `"is_error": true`) {
		return true
	}
	return false
}
