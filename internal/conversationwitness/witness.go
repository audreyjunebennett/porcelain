// Package conversationwitness implements Phase 8 request/response witness logging
// (counts and sizes only at Info; redacted payload excerpts at Trace / gated debug).
package conversationwitness

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"regexp"
	"strings"
	"unicode/utf8"
)

// payloadSampleLogLevel is slog's trace level (-8). Named constant so we compile on Go 1.22
// (slog.LevelTrace was added in Go 1.23).
const payloadSampleLogLevel = slog.Level(-8)

var (
	reBearer   = regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9\-_\.\+/=]{6,}\b`)
	reSkOpenAI = regexp.MustCompile(`\bsk-[A-Za-z0-9]{10,}\b`)
)

// RedactSecrets removes common secret patterns from a string for operator-safe excerpts.
func RedactSecrets(s string) string {
	if s == "" {
		return s
	}
	out := reBearer.ReplaceAllString(s, `Bearer ***`)
	out = reSkOpenAI.ReplaceAllString(out, `sk-***`)
	return out
}

// SplitHeadTail returns up to maxRunes runes from the start and end of s (after redaction).
func SplitHeadTail(s string, maxRunes int) (head, tail string) {
	if maxRunes <= 0 || s == "" {
		return "", ""
	}
	runes := []rune(s)
	if len(runes) <= maxRunes*2 {
		return s, ""
	}
	head = string(runes[:maxRunes])
	tail = string(runes[len(runes)-maxRunes:])
	return head, tail
}

// LogPayloadSample emits conversation.payload.sample at slog.LevelTrace when emit is true.
func LogPayloadSample(log *slog.Logger, emit bool, maxRunes int, kind string, payload []byte) {
	if log == nil || !emit || len(payload) == 0 {
		return
	}
	if maxRunes <= 0 {
		maxRunes = 256
	}
	red := RedactSecrets(string(payload))
	head, tail := SplitHeadTail(red, maxRunes)
	log.Log(context.Background(), payloadSampleLogLevel, "conversation payload sample",
		"msg", "conversation.payload.sample",
		"kind", kind,
		"head", head,
		"tail", tail,
		"redacted", true,
		"timeline_kind", "upstream",
	)
}

// LogRequestWitness logs conversation.request.witness from the chat completion JSON body map.
func LogRequestWitness(log *slog.Logger, body map[string]json.RawMessage) {
	if log == nil || body == nil {
		return
	}
	msgCount, roleJSON, promptEst, toolDecls, ok := requestWitnessStats(body)
	if !ok {
		return
	}
	log.Info("conversation request witness", "msg", "conversation.request.witness",
		"message_count", msgCount,
		"role_counts", roleJSON,
		"prompt_char_estimate", promptEst,
		"tool_decl_count", toolDecls,
		"timeline_kind", "upstream",
	)
}

func requestWitnessStats(body map[string]json.RawMessage) (msgCount int, roleCountsJSON string, promptChars int, toolDecls int, ok bool) {
	msgs, okm := body["messages"]
	if !okm || len(msgs) == 0 || string(msgs) == "null" {
		return 0, "{}", 0, countTools(body), true
	}
	var arr []json.RawMessage
	if err := json.Unmarshal(msgs, &arr); err != nil {
		return 0, "{}", 0, countTools(body), true
	}
	roles := make(map[string]int)
	for _, raw := range arr {
		var probe struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		}
		if err := json.Unmarshal(raw, &probe); err != nil {
			continue
		}
		r := strings.TrimSpace(strings.ToLower(probe.Role))
		if r == "" {
			r = "(empty)"
		}
		roles[r]++
		promptChars += estimateMessageContentChars(probe.Content)
	}
	rc, err := json.Marshal(roles)
	if err != nil {
		rc = []byte("{}")
	}
	return len(arr), string(rc), promptChars, countTools(body), true
}

func countTools(body map[string]json.RawMessage) int {
	tools, ok := body["tools"]
	if !ok || len(tools) == 0 || string(tools) == "null" {
		return 0
	}
	var arr []json.RawMessage
	if err := json.Unmarshal(tools, &arr); err != nil {
		return 0
	}
	return len(arr)
}

func estimateMessageContentChars(content json.RawMessage) int {
	if len(content) == 0 || string(content) == "null" {
		return 0
	}
	var s string
	if json.Unmarshal(content, &s) == nil {
		return utf8.RuneCountInString(s)
	}
	return utf8.RuneCountInString(string(content))
}

// LogResponseWitness logs conversation.response.witness for upstream JSON or SSE tail bytes.
func LogResponseWitness(log *slog.Logger, stream bool, respBody []byte) {
	if log == nil || len(respBody) == 0 {
		return
	}
	comp, fr, chunks, ok := responseWitnessStats(stream, respBody)
	if !ok {
		return
	}
	log.Info("conversation response witness", "msg", "conversation.response.witness",
		"completion_char_estimate", comp,
		"finish_reason", fr,
		"chunk_count", chunks,
		"timeline_kind", "upstream",
	)
}

// LogResponsePayloadSample emits a redacted response excerpt at trace when enabled.
func LogResponsePayloadSample(log *slog.Logger, emit bool, maxRunes int, stream bool, respBody []byte) {
	if log == nil || !emit || len(respBody) == 0 {
		return
	}
	kind := "response"
	if stream {
		kind = "response_stream"
	}
	LogPayloadSample(log, true, maxRunes, kind, respBody)
}

func responseWitnessStats(stream bool, respBody []byte) (completionChars int, finishReason string, chunkCount int, ok bool) {
	if stream {
		return responseWitnessFromSSE(respBody)
	}
	var root struct {
		Choices []struct {
			FinishReason string          `json:"finish_reason"`
			Message      json.RawMessage `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &root); err != nil || len(root.Choices) == 0 {
		return 0, "", 0, false
	}
	ch := root.Choices[0]
	fr := strings.TrimSpace(ch.FinishReason)
	var msg struct {
		Content json.RawMessage `json:"content"`
	}
	comp := 0
	if len(ch.Message) > 0 && json.Unmarshal(ch.Message, &msg) == nil {
		comp = estimateMessageContentChars(msg.Content)
	}
	if comp == 0 {
		if _, c, _, got := usageFromChatCompletionJSON(respBody); got && c > 0 {
			comp = c * 4
		}
	}
	return comp, fr, 1, true
}

func responseWitnessFromSSE(buf []byte) (completionChars int, finishReason string, chunkCount int, ok bool) {
	chunkCount = countSSEDataChunks(buf)
	if chunkCount == 0 {
		return 0, "", 0, false
	}
	finishReason = lastFinishReasonFromSSE(buf)
	if _, c, _, got := usageFromOpenAIChatSSE(buf); got && c > 0 {
		completionChars = c * 4
	} else {
		den := chunkCount
		if den < 1 {
			den = 1
		}
		completionChars = utf8.RuneCountInString(string(buf)) / den
		if completionChars > 500000 {
			completionChars = 500000
		}
	}
	return completionChars, finishReason, chunkCount, true
}

func countSSEDataChunks(buf []byte) int {
	n := 0
	for _, line := range bytes.Split(buf, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		payload := bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data:")))
		if len(payload) == 0 || bytes.Equal(payload, []byte("[DONE]")) {
			continue
		}
		if json.Valid(payload) {
			n++
		}
	}
	return n
}

func lastFinishReasonFromSSE(buf []byte) string {
	var last string
	for _, line := range bytes.Split(buf, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		payload := bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data:")))
		if len(payload) == 0 || bytes.Equal(payload, []byte("[DONE]")) || !json.Valid(payload) {
			continue
		}
		var probe struct {
			Choices []struct {
				FinishReason string `json:"finish_reason"`
				Delta        *struct {
					FinishReason string `json:"finish_reason"`
				} `json:"delta"`
			} `json:"choices"`
		}
		if json.Unmarshal(payload, &probe) != nil {
			continue
		}
		for _, ch := range probe.Choices {
			if ch.FinishReason != "" {
				last = ch.FinishReason
			}
			if ch.Delta != nil && ch.Delta.FinishReason != "" {
				last = ch.Delta.FinishReason
			}
		}
	}
	return strings.TrimSpace(last)
}

// usageFromChatCompletionJSON mirrors internal/chat minimal usage parse for witness fallback.
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
