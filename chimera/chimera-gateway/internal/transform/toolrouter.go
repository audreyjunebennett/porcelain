package transform

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

const toolRouterSystemPrompt = `You are a gateway tool-router. The upstream assistant will receive the user's latest message and a subset of tools you select confidence for.

You are given:
1) The full JSON array of tool definitions (OpenAI "tools" / function format).
2) The user's most recent user-role message (the prompt).

For EACH tool, estimate how likely it is (confidence 0.0–1.0) that the upstream model will NEED to call that tool on THIS turn to answer the user well. Be conservative: if unsure, assign a lower score.

Respond with ONLY valid JSON (no markdown fences, no prose). Use one of these shapes:
- [ {"name":"<tool function name>","confidence":0.0}, ... ]
- { "tools": [ {"name":"<tool function name>","confidence":0.0}, ... ] }

Every tool in the input list must appear exactly once by its function "name".`

// Config drives the tool-slimming transformer (see docs/plans/version-v0.1.1.md).
type Config struct {
	Enabled      bool
	RouterModels []string
	Threshold    float64
	BaseURL      string
	APIKey       string
	HTTPTimeout  time.Duration
	Log          *slog.Logger
	// OnAttempt is called after each router model attempt (err is nil on successful HTTP+parse).
	OnAttempt func(routerModel string, err error)
}

// ToolRouterSummary captures one ApplyToolRouter pass for conversation.tool.router (Phase 7).
type ToolRouterSummary struct {
	Ran         bool // enabled, router models configured, and a non-null tools key was present
	ToolsBefore int  // tools array length when parsed; 0 if unparsed or empty
	ToolsAfter  int  // tools array length on the returned body
	RouterModel string
	Err         error // scoring / network / parse failure; nil when slimming succeeded or was a no-op
	Applied     bool  // tools list was reduced (subset kept)
}

type toolScore struct {
	Name       string  `json:"name"`
	Confidence float64 `json:"confidence"`
}

// ApplyToolRouter returns a shallow copy of body with tools possibly replaced. On any failure
// or when disabled, returns body unchanged (fail-open: full tool list upstream).
func ApplyToolRouter(ctx context.Context, body map[string]json.RawMessage, cfg Config) (map[string]json.RawMessage, ToolRouterSummary) {
	var sum ToolRouterSummary
	if body == nil {
		return nil, sum
	}
	if !cfg.Enabled || len(cfg.RouterModels) == 0 {
		return body, sum
	}
	th := cfg.Threshold
	if th < 0 {
		th = 0
	}
	if th > 1 {
		th = 1
	}
	cfg.Threshold = th
	toolsRaw, ok := body["tools"]
	if !ok || len(toolsRaw) == 0 || string(toolsRaw) == "null" {
		return body, sum
	}
	sum.Ran = true
	var toolsArr []json.RawMessage
	if err := json.Unmarshal(toolsRaw, &toolsArr); err != nil {
		sum.Err = err
		return body, sum
	}
	if len(toolsArr) == 0 {
		sum.Err = errors.New("empty tools array")
		return body, sum
	}
	sum.ToolsBefore = len(toolsArr)
	sum.ToolsAfter = len(toolsArr)
	toolNames := make([]string, 0, len(toolsArr))
	for _, t := range toolsArr {
		var meta struct {
			Type     string          `json:"type"`
			Function json.RawMessage `json:"function"`
		}
		if err := json.Unmarshal(t, &meta); err != nil {
			return body, sum
		}
		name := ""
		if meta.Type == "function" && len(meta.Function) > 0 {
			var fn struct {
				Name string `json:"name"`
			}
			_ = json.Unmarshal(meta.Function, &fn)
			name = strings.TrimSpace(fn.Name)
		}
		if name == "" {
			// Unknown tool shape; do not risk dropping tools.
			return body, sum
		}
		toolNames = append(toolNames, name)
	}
	msgs, ok := body["messages"]
	if !ok || len(msgs) == 0 {
		return body, sum
	}
	userText := lastUserMessageText(msgs)
	if userText == "" {
		userText = "(empty user message)"
	}
	scores, usedModel, err := callRouterModels(ctx, cfg, string(toolsRaw), userText)
	if err != nil || len(scores) == 0 {
		if cfg.Log != nil {
			cfg.Log.Debug("tool router skipped or failed; passing all tools", "msg", "chat.tool_router.skipped", "err", err)
		}
		sum.Err = err
		return body, sum
	}
	kept := filterToolsByConfidence(toolsArr, toolNames, scores, cfg.Threshold)
	if len(kept) == 0 || len(kept) == len(toolsArr) {
		return body, sum
	}
	out := cloneRawMap(body)
	b, err := json.Marshal(kept)
	if err != nil {
		sum.Err = err
		return body, sum
	}
	out["tools"] = json.RawMessage(b)
	sum.ToolsAfter = len(kept)
	sum.RouterModel = usedModel
	sum.Applied = true
	if cfg.Log != nil {
		cfg.Log.Debug("tool router slimmed tools", "msg", "chat.tool_router.applied",
			"routerModel", usedModel,
			"before", len(toolsArr),
			"after", len(kept),
			"threshold", cfg.Threshold,
		)
	}
	return out, sum
}

func cloneRawMap(m map[string]json.RawMessage) map[string]json.RawMessage {
	out := make(map[string]json.RawMessage, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func lastUserMessageText(messages json.RawMessage) string {
	var arr []map[string]json.RawMessage
	if err := json.Unmarshal(messages, &arr); err != nil {
		return ""
	}
	for i := len(arr) - 1; i >= 0; i-- {
		var role string
		if r, ok := arr[i]["role"]; ok {
			_ = json.Unmarshal(r, &role)
		}
		if strings.TrimSpace(role) != "user" {
			continue
		}
		c, ok := arr[i]["content"]
		if !ok {
			return ""
		}
		var s string
		if json.Unmarshal(c, &s) == nil {
			return strings.TrimSpace(s)
		}
		// Multimodal / unknown: compact JSON as context.
		return strings.TrimSpace(string(c))
	}
	return ""
}

func callRouterModels(ctx context.Context, cfg Config, toolsJSON, userText string) (map[string]float64, string, error) {
	to := cfg.HTTPTimeout
	if to <= 0 {
		to = 45 * time.Second
	}
	userPayload := "Tools JSON:\n" + toolsJSON + "\n\nUser message:\n" + userText
	var lastErr error
	for _, model := range cfg.RouterModels {
		model = strings.TrimSpace(model)
		if model == "" {
			continue
		}
		scores, err := oneRouterCall(ctx, cfg.BaseURL, cfg.APIKey, model, userPayload, to)
		if err != nil {
			lastErr = err
			if cfg.OnAttempt != nil {
				cfg.OnAttempt(model, err)
			}
			if cfg.Log != nil {
				cfg.Log.Debug("tool router model attempt failed", "msg", "chat.tool_router.model_attempt_failed", "routerModel", model, "err", err)
			}
			continue
		}
		if cfg.OnAttempt != nil {
			cfg.OnAttempt(model, nil)
		}
		return scores, model, nil
	}
	return nil, "", lastErr
}

func oneRouterCall(ctx context.Context, baseURL, apiKey, model, userPayload string, timeout time.Duration) (map[string]float64, error) {
	reqBody := map[string]any{
		"model":       model,
		"temperature": 0,
		"stream":      false,
		"messages": []map[string]any{
			{"role": "system", "content": toolRouterSystemPrompt},
			{"role": "user", "content": userPayload},
		},
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	url := strings.TrimSuffix(baseURL, "/") + "/v1/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("upstream status %d: %s", res.StatusCode, truncateForErr(string(b), 300))
	}
	var envelope struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(b, &envelope); err != nil {
		return nil, err
	}
	if len(envelope.Choices) == 0 {
		return nil, errors.New("no choices in router response")
	}
	content := strings.TrimSpace(envelope.Choices[0].Message.Content)
	list, err := parseScoreList(content)
	if err != nil {
		return nil, err
	}
	out := make(map[string]float64, len(list))
	for _, row := range list {
		n := strings.TrimSpace(row.Name)
		if n == "" {
			continue
		}
		out[n] = row.Confidence
	}
	if len(out) == 0 {
		return nil, errors.New("empty score list")
	}
	return out, nil
}

func parseScoreList(content string) ([]toolScore, error) {
	s := stripMarkdownJSON(content)
	var direct []toolScore
	if err := json.Unmarshal([]byte(s), &direct); err == nil && len(direct) > 0 {
		return direct, nil
	}
	var wrapped struct {
		Tools []toolScore `json:"tools"`
	}
	if err := json.Unmarshal([]byte(s), &wrapped); err == nil && len(wrapped.Tools) > 0 {
		return wrapped.Tools, nil
	}
	return nil, fmt.Errorf("could not parse router JSON")
}

func stripMarkdownJSON(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		s = strings.TrimPrefix(s, "```")
		s = strings.TrimSpace(s)
		s = strings.TrimPrefix(s, "json")
		s = strings.TrimSpace(s)
		if idx := strings.LastIndex(s, "```"); idx >= 0 {
			s = strings.TrimSpace(s[:idx])
		}
	}
	return strings.TrimSpace(s)
}

func filterToolsByConfidence(toolsArr []json.RawMessage, toolNames []string, scores map[string]float64, threshold float64) []json.RawMessage {
	if len(toolsArr) != len(toolNames) {
		return toolsArr
	}
	kept := make([]json.RawMessage, 0, len(toolsArr))
	for i, t := range toolsArr {
		name := toolNames[i]
		conf, ok := scores[name]
		if !ok {
			// Missing score for a tool: keep (conservative).
			kept = append(kept, t)
			continue
		}
		if conf >= threshold {
			kept = append(kept, t)
		}
	}
	// If nothing passes threshold, fail-open to full list upstream.
	if len(kept) == 0 {
		return toolsArr
	}
	return kept
}

func truncateForErr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
