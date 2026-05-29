package conversationhistory

import (
	"bytes"
	"encoding/json"
	"strings"
)

// AssistantContentFromResponse extracts assistant text from JSON or SSE bodies.
func AssistantContentFromResponse(stream bool, body []byte) string {
	if len(body) == 0 {
		return ""
	}
	if stream {
		return assistantContentFromSSE(body)
	}
	return assistantContentFromJSON(body)
}

func assistantContentFromJSON(body []byte) string {
	if !json.Valid(body) {
		return ""
	}
	var root struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &root); err != nil || len(root.Choices) == 0 {
		return ""
	}
	return strings.TrimSpace(root.Choices[0].Message.Content)
}

func assistantContentFromSSE(body []byte) string {
	var b strings.Builder
	for _, line := range bytes.Split(body, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		payload := bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data:")))
		if len(payload) == 0 || bytes.Equal(payload, []byte("[DONE]")) {
			continue
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
		}
		if err := json.Unmarshal(payload, &chunk); err != nil {
			continue
		}
		if len(chunk.Choices) > 0 {
			b.WriteString(chunk.Choices[0].Delta.Content)
		}
	}
	return strings.TrimSpace(b.String())
}

// ResolvedModelFromJSON reads the model field from a chat completion JSON body.
func ResolvedModelFromJSON(body []byte) string {
	if len(body) == 0 || !json.Valid(body) {
		return ""
	}
	var root struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(body, &root); err != nil {
		return ""
	}
	return strings.TrimSpace(root.Model)
}

// ErrorFromJSON extracts user-visible error message and type from gateway/upstream JSON.
func ErrorFromJSON(body []byte) (message, errType string) {
	if len(body) == 0 || !json.Valid(body) {
		return "", ""
	}
	var root struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &root); err != nil {
		return "", ""
	}
	return strings.TrimSpace(root.Error.Message), strings.TrimSpace(root.Error.Type)
}

// UsageFromResponse extracts token usage from JSON or SSE bodies.
func UsageFromResponse(stream bool, body []byte) (prompt, completion, total int, ok bool) {
	if len(body) == 0 {
		return 0, 0, 0, false
	}
	if !stream {
		return usageFromChatCompletionJSON(body)
	}
	return usageFromOpenAIChatSSE(body)
}

func usageFromOpenAIChatSSE(buf []byte) (prompt, completion, total int, ok bool) {
	for _, line := range bytes.Split(buf, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		payload := bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data:")))
		if len(payload) == 0 || bytes.Equal(payload, []byte("[DONE]")) {
			continue
		}
		if p, c, tot, got := usageFromChatCompletionJSON(payload); got {
			prompt, completion, total = p, c, tot
			ok = true
		}
	}
	return prompt, completion, total, ok
}

func usageFromChatCompletionJSON(b []byte) (prompt, completion, total int, ok bool) {
	if len(b) == 0 || !json.Valid(b) {
		return 0, 0, 0, false
	}
	var root struct {
		Usage *struct {
			Prompt     int `json:"prompt_tokens"`
			Completion int `json:"completion_tokens"`
			Total      int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(b, &root); err != nil || root.Usage == nil {
		return 0, 0, 0, false
	}
	u := root.Usage
	total = u.Total
	if total <= 0 && (u.Prompt > 0 || u.Completion > 0) {
		total = u.Prompt + u.Completion
	}
	if total <= 0 && u.Prompt <= 0 && u.Completion <= 0 {
		return 0, 0, 0, false
	}
	return u.Prompt, u.Completion, total, true
}
