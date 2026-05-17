package conversationmerge

import (
	"encoding/json"
	"strings"
)

// AssistantTextFromCompletionJSON extracts assistant message text from a chat completion JSON body.
func AssistantTextFromCompletionJSON(b []byte) string {
	if len(b) == 0 || !json.Valid(b) {
		return ""
	}
	var root struct {
		Choices []struct {
			Message struct {
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if json.Unmarshal(b, &root) != nil || len(root.Choices) == 0 {
		return ""
	}
	return messageContentAsString(root.Choices[0].Message.Content)
}

func messageContentAsString(raw json.RawMessage) string {
	raw = bytesTrim(raw)
	if len(raw) == 0 {
		return ""
	}
	if raw[0] == '"' {
		var s string
		if json.Unmarshal(raw, &s) == nil {
			return s
		}
	}
	if raw[0] == '[' {
		var parts []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}
		if json.Unmarshal(raw, &parts) == nil {
			var sb strings.Builder
			for _, p := range parts {
				if strings.EqualFold(p.Type, "text") || p.Type == "" {
					if sb.Len() > 0 {
						sb.WriteString("\n")
					}
					sb.WriteString(p.Text)
				}
			}
			return sb.String()
		}
	}
	return ""
}

func bytesTrim(b []byte) []byte {
	for len(b) > 0 && (b[0] == ' ' || b[0] == '\n' || b[0] == '\t' || b[0] == '\r') {
		b = b[1:]
	}
	for len(b) > 0 {
		c := b[len(b)-1]
		if c != ' ' && c != '\n' && c != '\t' && c != '\r' {
			break
		}
		b = b[:len(b)-1]
	}
	return b
}
