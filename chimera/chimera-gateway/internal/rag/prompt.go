package rag

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/vectorstore"
)

// LastUserText extracts the visible text of the last "user" message in an
// OpenAI-style messages array. It supports string content as well as the
// multimodal array form ({type:"text", text:"..."}). Returns "" when no user
// message is found.
func LastUserText(rawMessages json.RawMessage) string {
	if len(rawMessages) == 0 {
		return ""
	}
	var msgs []struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(rawMessages, &msgs); err != nil {
		return ""
	}
	for i := len(msgs) - 1; i >= 0; i-- {
		if !strings.EqualFold(msgs[i].Role, "user") {
			continue
		}
		return contentAsString(msgs[i].Content)
	}
	return ""
}

func contentAsString(raw json.RawMessage) string {
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

// FormatRetrievedContext renders hits as a single delimited markdown section
// suitable for prepending as a system message. Empty hits returns "".
func FormatRetrievedContext(hits []vectorstore.Hit) string {
	if len(hits) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("### Retrieved context\n\n")
	for i, h := range hits {
		src := h.Payload.Source
		if src == "" {
			src = "unknown"
		}
		hdr := fmt.Sprintf("%d. `%s`", i+1, src)
		if lr := formatLineRange(h.Payload.StartLine, h.Payload.EndLine); lr != "" {
			hdr += " " + lr
		}
		fmt.Fprintf(&sb, "%s (score=%.3f)\n\n", hdr, h.Score)
		text := strings.TrimSpace(h.Payload.Text)
		if h.Payload.StartsMidLine && text != "" {
			if idx := strings.IndexByte(text, '\n'); idx >= 0 {
				text = "…" + text[:idx] + text[idx:]
			} else {
				text = "…" + text
			}
		}
		// keep each chunk readable but bounded
		if len(text) > 4000 {
			text = text[:4000] + "…"
		}
		sb.WriteString(text)
		sb.WriteString("\n\n")
	}
	return strings.TrimRight(sb.String(), "\n") + "\n"
}

func formatLineRange(start, end int) string {
	if start <= 0 {
		return ""
	}
	if end <= 0 || end < start {
		end = start
	}
	if start == end {
		return fmt.Sprintf("L%d", start)
	}
	return fmt.Sprintf("L%d–%d", start, end)
}

// InjectSystemMessage prepends a system role message containing context to the
// messages array in body. If body["messages"] is missing or invalid, it is
// left unchanged. Returns the (possibly new) raw messages slice.
func InjectSystemMessage(body map[string]json.RawMessage, content string) {
	if strings.TrimSpace(content) == "" {
		return
	}
	raw, ok := body["messages"]
	if !ok {
		return
	}
	var msgs []json.RawMessage
	if err := json.Unmarshal(raw, &msgs); err != nil {
		return
	}
	sys := map[string]any{"role": "system", "content": content}
	sysRaw, err := json.Marshal(sys)
	if err != nil {
		return
	}
	combined := append([]json.RawMessage{json.RawMessage(sysRaw)}, msgs...)
	out, err := json.Marshal(combined)
	if err != nil {
		return
	}
	body["messages"] = out
}
