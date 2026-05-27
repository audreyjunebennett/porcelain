package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/conversationwitness"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/gatewaymetrics"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/routing"
	"github.com/lynn/porcelain/chimera/internal/providerlimits"
	"github.com/lynn/porcelain/chimera/internal/tokencount"
	"github.com/lynn/porcelain/internal/naming"
)

// upstreamStreamUsageTailBytes caps how much of a streaming SSE body we retain
// after the proxy finishes, only for operator logs (usage extraction + excerpt).
const upstreamStreamUsageTailBytes = 512 * 1024

var retryStatuses = map[int]struct{}{
	http.StatusRequestEntityTooLarge: {}, // 413: virtual model tries next fallback (same payload)
	http.StatusNotFound:              {}, // 404: upstream OpenAI-compat "model not found" → try next fallback
	http.StatusTooManyRequests:       {},
	http.StatusInternalServerError:   {},
	http.StatusBadGateway:            {},
	http.StatusServiceUnavailable:    {},
	http.StatusGatewayTimeout:        {},
}

// fallbackFailureRecord is one failed upstream attempt during virtual-model routing.
type fallbackFailureRecord struct {
	UpstreamModel string `json:"upstream_model"`
	Status        int    `json:"status"`
	Summary       string `json:"summary,omitempty"`
	RateLimit     bool   `json:"rate_limit,omitempty"`
}

func excerptOpenAIStyleErrorMessage(body []byte, max int) string {
	if len(body) == 0 {
		return ""
	}
	var wrap struct {
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &wrap) == nil && wrap.Error != nil && strings.TrimSpace(wrap.Error.Message) != "" {
		return truncateRunes(strings.TrimSpace(wrap.Error.Message), max)
	}
	return truncateRunes(string(body), max)
}

func excerptUpstreamErrorForLog(body []byte, errMsg string, max int) string {
	if strings.TrimSpace(errMsg) != "" {
		return truncateRunes(errMsg, max)
	}
	return excerptOpenAIStyleErrorMessage(body, max)
}

func upstreamErrorText(jsonBody []byte, errMsg string) string {
	fields := parseUpstreamErrorFields(jsonBody, errMsg)
	if fields.Message != "" {
		return fields.Message
	}
	if len(jsonBody) > 0 {
		return strings.TrimSpace(string(jsonBody))
	}
	return strings.TrimSpace(errMsg)
}

type upstreamErrorFields struct {
	Message string
	Type    string
	Code    string
}

func parseUpstreamErrorFields(jsonBody []byte, errMsg string) upstreamErrorFields {
	var out upstreamErrorFields
	if msg := strings.TrimSpace(errMsg); msg != "" {
		out.Message = msg
	}
	if len(jsonBody) == 0 {
		return out
	}
	var wrap struct {
		Error *struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    any    `json:"code"`
		} `json:"error"`
	}
	if json.Unmarshal(jsonBody, &wrap) != nil || wrap.Error == nil {
		return out
	}
	if out.Message == "" {
		out.Message = strings.TrimSpace(wrap.Error.Message)
	}
	out.Type = strings.TrimSpace(wrap.Error.Type)
	switch c := wrap.Error.Code.(type) {
	case string:
		out.Code = strings.TrimSpace(c)
	case float64:
		out.Code = strconv.FormatInt(int64(c), 10)
	}
	return out
}

func errorTextSignalsRateLimit(text string) bool {
	lower := strings.ToLower(strings.TrimSpace(text))
	if lower == "" {
		return false
	}
	for _, phrase := range []string{
		"rate limit",
		"rate_limit",
		"ratelimit",
		"too many requests",
		"requests per minute",
		"requests per day",
		"rpm limit",
		"tpm limit",
		"tokens per minute",
		"tokens per day",
	} {
		if strings.Contains(lower, phrase) {
			return true
		}
	}
	return false
}

func upstreamErrorTypeSignalsRateLimit(errType, errCode string) bool {
	lt := strings.ToLower(strings.TrimSpace(errType))
	lc := strings.ToLower(strings.TrimSpace(errCode))
	if lt == "rate_limit_exceeded" || lt == "rate_limit_error" || lt == "insufficient_quota" {
		return true
	}
	if strings.Contains(lt, "rate") && strings.Contains(lt, "limit") {
		return true
	}
	return lc == "rate_limit_exceeded"
}

var contextOverflowErrorCodes = map[string]struct{}{
	"request_too_large":       {},
	"context_length_exceeded": {},
}

// upstreamErrorIndicatesContextOverflow reports HTTP 400/422 responses where the upstream rejected
// the request because the prompt or total context exceeded the model window.
func upstreamErrorIndicatesContextOverflow(status int, jsonBody []byte, errMsg string) bool {
	if status != http.StatusBadRequest && status != http.StatusUnprocessableEntity {
		return false
	}
	fields := parseUpstreamErrorFields(jsonBody, errMsg)
	if _, ok := contextOverflowErrorCodes[strings.ToLower(fields.Code)]; ok {
		return true
	}
	lt := strings.ToLower(strings.TrimSpace(fields.Type))
	return lt == "request_too_large" || lt == "context_length_exceeded"
}

// upstreamErrorIndicatesRateLimit reports whether an upstream response should be treated
// as a provider rate limit (HTTP 429, or HTTP 400 with rate-limit-shaped error content).
func upstreamErrorIndicatesRateLimit(status int, jsonBody []byte, errMsg string) bool {
	if status == http.StatusTooManyRequests {
		return true
	}
	if status != http.StatusBadRequest {
		return false
	}
	fields := parseUpstreamErrorFields(jsonBody, errMsg)
	if upstreamErrorTypeSignalsRateLimit(fields.Type, fields.Code) {
		return true
	}
	return errorTextSignalsRateLimit(upstreamErrorText(jsonBody, errMsg))
}

func shouldRetryVirtualModelFallback(status int, jsonBody []byte, errMsg string) bool {
	if _, ok := retryStatuses[status]; ok {
		return true
	}
	if upstreamErrorIndicatesContextOverflow(status, jsonBody, errMsg) {
		return true
	}
	return upstreamErrorIndicatesRateLimit(status, jsonBody, errMsg)
}

func allAttemptsRateLimited(attempts []fallbackFailureRecord) bool {
	if len(attempts) == 0 {
		return false
	}
	for _, a := range attempts {
		if !a.RateLimit {
			return false
		}
	}
	return true
}

func clientModelFromBody(body map[string]json.RawMessage) string {
	if body == nil {
		return ""
	}
	m, ok := body["model"]
	if !ok || len(m) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(m, &s); err != nil {
		return ""
	}
	return strings.TrimSpace(s)
}

// maxTokensFromBody returns client max_tokens when set. When the client omits max_tokens, 0 is
// returned so context admission checks prompt size only (see context-window-admission plan).
func maxTokensFromBody(body map[string]json.RawMessage) int64 {
	if body == nil {
		return 0
	}
	raw, ok := body["max_tokens"]
	if !ok || len(raw) == 0 {
		return 0
	}
	var n json.Number
	if err := json.Unmarshal(raw, &n); err != nil {
		var i int64
		if err2 := json.Unmarshal(raw, &i); err2 != nil || i < 0 {
			return 0
		}
		return i
	}
	v, err := n.Int64()
	if err != nil || v < 0 {
		return 0
	}
	return v
}

func requestAdmission(body map[string]json.RawMessage, out []byte, est int) providerlimits.RequestAdmission {
	return providerlimits.RequestAdmission{
		EstPromptTokens: int64(est),
		MaxTokens:       maxTokensFromBody(body),
		BodyBytes:       int64(len(out)),
	}
}

func providerLimitsDenyMessage(reason providerlimits.Reason, fallbackChain bool) string {
	dim := string(reason)
	if fallbackChain {
		return "Every model in the fallback chain would exceed configured provider limits (" + dim + ")."
	}
	return "Configured provider/model limits would be exceeded for this request (" + dim + ")."
}

func providerLimitsBlockedLogArgs(upstreamModel string, d providerlimits.Decision, admission providerlimits.RequestAdmission, guard *providerlimits.Guard) []any {
	args := []any{
		"msg", naming.MsgChatProviderLimitsBlocked,
		"upstreamModel", upstreamModel,
		"reason", string(d.Reason),
		"detail", d.Detail,
		"outgoingTokens", admission.EstPromptTokens,
		"max_tokens", admission.MaxTokens,
		"body_bytes", admission.BodyBytes,
	}
	if guard == nil {
		return args
	}
	eff := guard.EffectiveFor(upstreamModel)
	switch d.Reason {
	case providerlimits.ReasonContext:
		if cap, ok := eff.EffectiveContextCap(); ok {
			args = append(args, "context_cap", cap)
		}
	case providerlimits.ReasonBodySize:
		if eff.MaxBodyBytes != nil {
			args = append(args, "max_body_bytes", *eff.MaxBodyBytes)
		}
	}
	return args
}

func conversationRoutingLogArgs(clientModel, upstreamModel string, attempt, chainLen int, stream bool, outgoingTokens int) []any {
	args := []any{
		"msg", naming.MsgConversationRoutingResolved,
		"upstreamModel", upstreamModel,
		"attempt", attempt,
		"chainLen", chainLen,
		"stream", stream,
	}
	if clientModel != "" {
		args = append(args, "clientModel", clientModel)
	}
	if outgoingTokens > 0 {
		args = append(args, "outgoingTokens", outgoingTokens)
	}
	return args
}

func finishReasonFromChatJSON(stream bool, respBody []byte) string {
	if len(respBody) == 0 {
		return ""
	}
	if stream {
		return lastFinishReasonFromSSE(respBody)
	}
	var root struct {
		Choices []struct {
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &root); err != nil || len(root.Choices) == 0 {
		return ""
	}
	return strings.TrimSpace(root.Choices[0].FinishReason)
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

func appendFallbackFailure(dst *[]fallbackFailureRecord, upstreamModel string, status int, jsonBody []byte, errMsg string) {
	*dst = append(*dst, fallbackFailureRecord{
		UpstreamModel: upstreamModel,
		Status:        status,
		Summary:       excerptUpstreamErrorForLog(jsonBody, errMsg, 240),
		RateLimit:     upstreamErrorIndicatesRateLimit(status, jsonBody, errMsg),
	})
}

func buildFallbackExhaustedMessage(attempts []fallbackFailureRecord) string {
	var b strings.Builder
	b.WriteString("Every model in the fallback chain failed")
	if len(attempts) > 0 {
		b.WriteString(" (")
		b.WriteString(strconv.Itoa(len(attempts)))
		b.WriteString(" attempt(s)): ")
		for i, a := range attempts {
			if i > 0 {
				b.WriteString("; ")
			}
			b.WriteString(a.UpstreamModel)
			b.WriteString(": HTTP ")
			b.WriteString(strconv.Itoa(a.Status))
			if a.Summary != "" {
				b.WriteString(" — ")
				b.WriteString(truncateRunes(a.Summary, 120))
			}
		}
	}
	b.WriteString(".")
	return b.String()
}

func fallbackAttemptObjects(attempts []fallbackFailureRecord) []map[string]any {
	attemptObjs := make([]map[string]any, len(attempts))
	for i, a := range attempts {
		m := map[string]any{
			"upstream_model": a.UpstreamModel,
			"status":         a.Status,
		}
		if a.Summary != "" {
			m["summary"] = a.Summary
		}
		if a.RateLimit {
			m["rate_limit"] = true
		}
		attemptObjs[i] = m
	}
	return attemptObjs
}

func buildFallbackExhaustedJSONBody(attempts []fallbackFailureRecord) []byte {
	root := map[string]any{
		"error": map[string]any{
			"message": buildFallbackExhaustedMessage(attempts),
			"type":    "gateway_fallback_exhausted",
			"details": map[string]any{
				"attempts": fallbackAttemptObjects(attempts),
			},
		},
	}
	b, err := json.Marshal(root)
	if err != nil {
		return []byte(`{"error":{"message":"Every model in the fallback chain failed.","type":"gateway_fallback_exhausted"}}`)
	}
	return b
}

func buildRateLimitExhaustedMessage(attempts []fallbackFailureRecord) string {
	var b strings.Builder
	b.WriteString("Every model in the fallback chain was rate-limited")
	if len(attempts) > 0 {
		b.WriteString(" (")
		b.WriteString(strconv.Itoa(len(attempts)))
		b.WriteString(" attempt(s)): ")
		for i, a := range attempts {
			if i > 0 {
				b.WriteString("; ")
			}
			b.WriteString(a.UpstreamModel)
			b.WriteString(": HTTP ")
			b.WriteString(strconv.Itoa(a.Status))
			if a.Summary != "" {
				b.WriteString(" — ")
				b.WriteString(truncateRunes(a.Summary, 120))
			}
		}
	}
	b.WriteString(".")
	return b.String()
}

func buildRateLimitExhaustedJSONBody(attempts []fallbackFailureRecord, chainLen int) []byte {
	modelsTried := make([]string, len(attempts))
	for i, a := range attempts {
		modelsTried[i] = a.UpstreamModel
	}
	root := map[string]any{
		"error": map[string]any{
			"message": buildRateLimitExhaustedMessage(attempts),
			"type":    "gateway_rate_limit_exhausted",
			"details": map[string]any{
				"attempts":      fallbackAttemptObjects(attempts),
				"models_tried":  modelsTried,
				"attempt_count": len(attempts),
				"chain_len":     chainLen,
				"cause":         "rate_limit",
			},
		},
	}
	b, err := json.Marshal(root)
	if err != nil {
		return []byte(`{"error":{"message":"Every model in the fallback chain was rate-limited.","type":"gateway_rate_limit_exhausted"}}`)
	}
	return b
}

func logRateLimitRouting(log *slog.Logger, upstreamModel string, attempt, chainLen int, willRetry bool, jsonBody []byte, errMsg string) {
	if log == nil {
		return
	}
	summary := excerptUpstreamErrorForLog(jsonBody, errMsg, 200)
	args := []any{
		"msg", "chat.routing.rate_limit",
		"upstreamModel", upstreamModel,
		"attempt", attempt,
		"chainLen", chainLen,
		"willRetry", willRetry,
	}
	if summary != "" {
		args = append(args, "upstreamErrorExcerpt", summary)
	}
	log.Info("upstream rate limit", args...)
}

func logModelNotFoundRouting(log *slog.Logger, upstreamModel string, attempt, chainLen int, willRetry bool, jsonBody []byte) {
	if log == nil {
		return
	}
	summary := excerptOpenAIStyleErrorMessage(jsonBody, 200)
	args := []any{
		"msg", "chat.routing.model_not_found",
		"upstreamModel", upstreamModel,
		"attempt", attempt,
		"chainLen", chainLen,
		"willRetry", willRetry,
	}
	if summary != "" {
		args = append(args, "upstreamErrorExcerpt", summary)
	}
	log.Info("upstream model not found (HTTP 404)", args...)
	if !willRetry {
		log.Info("conversation fallback: upstream model not found",
			"msg", naming.MsgConversationFallbackModelNotFound,
			"upstreamModel", upstreamModel,
			"attempt", attempt,
			"chainLen", chainLen,
			"willRetry", willRetry)
		return
	}
	log.Info("conversation routing: model not found, trying next",
		"msg", naming.MsgConversationFallbackModelNotFound,
		"upstreamModel", upstreamModel,
		"attempt", attempt,
		"chainLen", chainLen,
		"willRetry", willRetry)
}

// hasMoreFallbackCandidates reports whether any chain entry after afterIdx is not excluded for 413
// (and thus is eligible to try). excluded413 may be nil.
func hasMoreFallbackCandidates(chain []string, afterIdx int, excluded413 map[string]struct{}) bool {
	for j := afterIdx + 1; j < len(chain); j++ {
		if excluded413 != nil {
			if _, skip := excluded413[chain[j]]; skip {
				continue
			}
		}
		return true
	}
	return false
}

// ProxyResult mirrors src/chat.ts proxyChatCompletion outcomes.
type ProxyResult struct {
	Stream        bool
	Status        int
	JSONBody      []byte
	ErrMessage    string
	DeliveryBytes int64 // client-visible body bytes when known (stream tee or JSON body)
}

// ProxyOpts carries optional hooks for gateway features (e.g. conversation merge persistence).
type ProxyOpts struct {
	// UpstreamRequestID is forwarded as X-Request-Id on the chimera-broker hop
	// so broker logs can expose the gateway request id when the platform supports it.
	UpstreamRequestID string
	// OnUpstreamJSONSuccess runs before the caller writes a successful non-streaming JSON body
	// (status 2xx). Streaming completions do not invoke this hook.
	OnUpstreamJSONSuccess func(statusCode int, upstreamModel string, jsonBody []byte)
	// OnChatDelivery runs once after the proxied chat completion finishes writing to the client
	// (success, error JSON, or stream end). elapsedMs is wall time for the proxy operation.
	OnChatDelivery func(status int, stream bool, bytesToClient int64, elapsedMs int64)
	// SuppressChatDelivery skips the automatic OnChatDelivery callback in proxyChatCompletionPayload
	// (used by WithVirtualModelFallback, which invokes OnChatDelivery once for the overall exchange).
	SuppressChatDelivery bool
	// WitnessEmitPayloadSample enables conversation.payload.sample at trace (or debug when forced in config).
	WitnessEmitPayloadSample bool
	// WitnessPayloadSampleMaxRunes caps each of head and tail in payload samples (default 256).
	WitnessPayloadSampleMaxRunes int
}

func notifyUpstreamJSONSuccess(opts *ProxyOpts, statusCode int, upstreamModel string, jsonBody []byte) {
	if opts == nil || opts.OnUpstreamJSONSuccess == nil || len(jsonBody) == 0 || !statusOK(statusCode) {
		return
	}
	opts.OnUpstreamJSONSuccess(statusCode, upstreamModel, jsonBody)
}

func estTokensFromPayload(out []byte) int {
	n, err := tokencount.Count(string(out))
	if err != nil {
		return 0
	}
	return n
}

// prepareChatPayload builds the proxied JSON body and its estimated token count for upstreamModel.
func prepareChatPayload(upstreamModel string, stream bool, body map[string]json.RawMessage) ([]byte, int, error) {
	payload := cloneRawMap(body)
	payload["model"] = mustRawJSON(upstreamModel)
	payload["stream"] = mustRawJSON(stream)
	out, err := json.Marshal(payload)
	if err != nil {
		return nil, 0, err
	}
	return out, estTokensFromPayload(out), nil
}

func recordUpstreamMetrics(rec gatewaymetrics.Recorder, upstreamModel string, status, est int) {
	if rec == nil {
		return
	}
	rec.RecordBrokerResponse(time.Now().UTC(), upstreamModel, status, est)
}

func notifyChatDelivery(opts *ProxyOpts, status int, stream bool, bytes int64, elapsedMs int64) {
	if opts == nil || opts.OnChatDelivery == nil || opts.SuppressChatDelivery {
		return
	}
	opts.OnChatDelivery(status, stream, bytes, elapsedMs)
}

// ProxyChatCompletion forwards POST /v1/chat/completions to upstream. When guard is non-nil,
// admission is checked before the HTTP request; denial returns HTTP 429 with JSON in JSONBody.
func ProxyChatCompletion(ctx context.Context, w http.ResponseWriter, baseURL, apiKey, upstreamModel string, stream bool, body map[string]json.RawMessage, timeout time.Duration, log *slog.Logger, rec gatewaymetrics.Recorder, guard *providerlimits.Guard, opts *ProxyOpts) ProxyResult {
	t0 := time.Now()
	out, est, err := prepareChatPayload(upstreamModel, stream, body)
	if err != nil {
		notifyChatDelivery(opts, http.StatusInternalServerError, stream, 0, time.Since(t0).Milliseconds())
		return ProxyResult{Status: 500, ErrMessage: "marshal request"}
	}
	if guard != nil {
		admission := requestAdmission(body, out, est)
		d, gerr := guard.Allow(ctx, upstreamModel, admission)
		if gerr != nil && log != nil {
			log.Warn("provider limits admission query failed; allowing request", "msg", "chat.provider_limits.query_failed", "err", gerr, "upstreamModel", upstreamModel)
		}
		if !d.Allowed {
			if log != nil {
				log.Info("chat blocked by provider limits", providerLimitsBlockedLogArgs(upstreamModel, d, admission, guard)...)
			}
			b, _ := json.Marshal(map[string]any{
				"error": map[string]any{
					"message": providerLimitsDenyMessage(d.Reason, false),
					"type":    "gateway_provider_limits",
				},
			})
			notifyChatDelivery(opts, http.StatusTooManyRequests, stream, int64(len(b)), time.Since(t0).Milliseconds())
			return ProxyResult{Status: http.StatusTooManyRequests, JSONBody: b}
		}
	}
	if log != nil && opts != nil && opts.WitnessEmitPayloadSample {
		mc := opts.WitnessPayloadSampleMaxRunes
		if mc <= 0 {
			mc = 256
		}
		conversationwitness.LogPayloadSample(log, true, mc, "request", out)
	}
	if log != nil {
		clientModel := clientModelFromBody(body)
		log.Info("conversation routed", conversationRoutingLogArgs(clientModel, upstreamModel, 1, 1, stream, est)...)
	}
	return proxyChatCompletionPayload(ctx, w, baseURL, apiKey, upstreamModel, stream, out, est, timeout, log, rec, opts)
}

func proxyChatCompletionPayload(ctx context.Context, w http.ResponseWriter, baseURL, apiKey, upstreamModel string, stream bool, out []byte, est int, timeout time.Duration, log *slog.Logger, rec gatewaymetrics.Recorder, opts *ProxyOpts) (res ProxyResult) {
	t0 := time.Now()
	if opts != nil && opts.OnChatDelivery != nil && !opts.SuppressChatDelivery {
		defer func() {
			st := res.Status
			if res.ErrMessage != "" && st == 0 {
				st = http.StatusServiceUnavailable
			}
			if st == 0 && !res.Stream {
				st = http.StatusOK
			}
			if st == 0 && res.Stream {
				st = http.StatusOK
			}
			b := res.DeliveryBytes
			if !res.Stream && b == 0 && len(res.JSONBody) > 0 {
				b = int64(len(res.JSONBody))
			}
			opts.OnChatDelivery(st, res.Stream, b, time.Since(t0).Milliseconds())
		}()
	}

	url := strings.TrimSuffix(baseURL, "/") + "/v1/chat/completions"
	path := "/v1/chat/completions"

	if log != nil {
		n, errTok := tokencount.Count(string(out))
		reqEx := truncateRunes(string(out), 320)
		if errTok == nil {
			log.Info("chimera-broker chat relay",
				"msg", "chat.chimera-broker.request",
				"path", path,
				"upstreamModel", upstreamModel,
				"stream", stream,
				"target", url,
				"outgoingTokens", n,
				"requestBodyExcerpt", reqEx,
			)
		} else {
			log.Info("chimera-broker chat relay",
				"msg", "chat.chimera-broker.request",
				"path", path,
				"upstreamModel", upstreamModel,
				"stream", stream,
				"target", url,
				"requestBodyExcerpt", reqEx,
			)
			log.Debug("outgoing token count failed", "msg", "chat.chimera-broker.outgoing_tokens_count_failed", "err", errTok)
		}
	}

	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, url, bytes.NewReader(out))
	if err != nil {
		res = ProxyResult{Status: 503, ErrMessage: err.Error()}
		return
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	if opts != nil && strings.TrimSpace(opts.UpstreamRequestID) != "" {
		req.Header.Set("X-Request-Id", strings.TrimSpace(opts.UpstreamRequestID))
	}

	if log != nil {
		log.Info("conversation broker started", "msg", naming.MsgConversationBrokerStarted,
			"upstreamModel", upstreamModel, "stream", stream, "outgoingTokens", est)
	}

	resHTTP, err := http.DefaultClient.Do(req)
	if err != nil {
		if log != nil {
			log.Warn("upstream chat fetch failed", "msg", "chat.chimera-broker.error", "err", err, "target", url, "upstreamModel", upstreamModel, "stream", stream)
			log.Warn("conversation broker failed", "msg", naming.MsgConversationBrokerFailed,
				"upstreamModel", upstreamModel, "statusCode", http.StatusServiceUnavailable, "err", err.Error())
		}
		recordUpstreamMetrics(rec, upstreamModel, http.StatusServiceUnavailable, est)
		res = ProxyResult{Status: 503, ErrMessage: err.Error()}
		return
	}
	defer resHTTP.Body.Close()

	if !statusOK(resHTTP.StatusCode) && !stream {
		b, _ := io.ReadAll(resHTTP.Body)
		logUpstreamChatResponse(log, url, resHTTP.StatusCode, upstreamModel, stream, b, resHTTP.Header, opts)
		recordUpstreamMetrics(rec, upstreamModel, resHTTP.StatusCode, est)
		res = ProxyResult{Status: resHTTP.StatusCode, JSONBody: b, DeliveryBytes: int64(len(b))}
		return
	}

	if !statusOK(resHTTP.StatusCode) && stream {
		b, err := io.ReadAll(resHTTP.Body)
		var wrap []byte
		if err == nil && json.Valid(b) {
			wrap = b
		} else {
			text := string(b)
			if text == "" {
				text = "upstream error on streaming request"
			}
			wrap, _ = json.Marshal(map[string]any{
				"error": map[string]any{
					"message": text,
					"type":    "gateway_upstream",
				},
			})
		}
		logUpstreamChatResponse(log, url, resHTTP.StatusCode, upstreamModel, stream, wrap, resHTTP.Header, opts)
		recordUpstreamMetrics(rec, upstreamModel, resHTTP.StatusCode, est)
		res = ProxyResult{Status: resHTTP.StatusCode, JSONBody: wrap, DeliveryBytes: int64(len(wrap))}
		return
	}

	if stream && resHTTP.Body != nil {
		h := w.Header()
		ct := resHTTP.Header.Get("Content-Type")
		if ct == "" {
			ct = "text/event-stream; charset=utf-8"
		}
		h.Set("Content-Type", ct)
		h.Set("Cache-Control", "no-cache")
		h.Set("Connection", "keep-alive")
		if x := resHTTP.Header.Get("X-Request-Id"); x != "" {
			h.Set("X-Request-Id", x)
		}
		w.WriteHeader(http.StatusOK)
		var cw countWriter
		cw.w = w
		var tail streamUsageTail
		upstream := io.TeeReader(resHTTP.Body, &tail)
		if f, ok := w.(http.Flusher); ok {
			_, _ = io.Copy(&flushWriter{w: &cw, f: f}, upstream)
		} else {
			_, _ = io.Copy(&cw, upstream)
		}
		logUpstreamChatResponse(log, url, http.StatusOK, upstreamModel, stream, tail.bytes(), resHTTP.Header, opts)
		recordUpstreamMetrics(rec, upstreamModel, http.StatusOK, est)
		res = ProxyResult{Stream: true, Status: http.StatusOK, DeliveryBytes: cw.n}
		return
	}

	b, err := io.ReadAll(resHTTP.Body)
	if err != nil {
		logUpstreamChatResponse(log, url, resHTTP.StatusCode, upstreamModel, stream, nil, resHTTP.Header, opts)
		recordUpstreamMetrics(rec, upstreamModel, http.StatusServiceUnavailable, est)
		res = ProxyResult{Status: 503, ErrMessage: err.Error()}
		return
	}
	logUpstreamChatResponse(log, url, resHTTP.StatusCode, upstreamModel, stream, b, resHTTP.Header, opts)
	recordUpstreamMetrics(rec, upstreamModel, resHTTP.StatusCode, est)
	notifyUpstreamJSONSuccess(opts, resHTTP.StatusCode, upstreamModel, b)
	res = ProxyResult{Status: resHTTP.StatusCode, JSONBody: b, DeliveryBytes: int64(len(b))}
	return
}

// countWriter wraps an io.Writer and records the number of bytes written.
type countWriter struct {
	w io.Writer
	n int64
}

func (c *countWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	return n, err
}

func truncateRunes(s string, max int) string {
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

func formatResponseHeadersForLog(h http.Header, maxLen int) string {
	if h == nil || len(h) == 0 || maxLen <= 0 {
		return ""
	}
	type pair struct {
		k, v string
	}
	var pairs []pair
	for k, vv := range h {
		lk := strings.ToLower(k)
		if lk == "set-cookie" {
			continue
		}
		v := strings.Join(vv, ",")
		if lk == "authorization" {
			v = "[redacted]"
		}
		pairs = append(pairs, pair{k, v})
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].k < pairs[j].k })
	var b strings.Builder
	for _, p := range pairs {
		if b.Len() > 0 {
			b.WriteString("; ")
		}
		b.WriteString(p.k)
		b.WriteString(": ")
		b.WriteString(p.v)
		if b.Len() >= maxLen {
			break
		}
	}
	out := b.String()
	if len(out) > maxLen {
		out = out[:maxLen] + "…"
	}
	return out
}

// streamUsageTail keeps the last upstreamStreamUsageTailBytes of a streaming body
// so we can parse OpenAI-style SSE usage without buffering the full response.
type streamUsageTail struct {
	buf []byte
}

func (s *streamUsageTail) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if len(s.buf)+len(p) <= upstreamStreamUsageTailBytes {
		s.buf = append(s.buf, p...)
		return len(p), nil
	}
	merged := append(s.buf, p...)
	if len(merged) > upstreamStreamUsageTailBytes {
		merged = merged[len(merged)-upstreamStreamUsageTailBytes:]
	}
	s.buf = merged
	return len(p), nil
}

func (s *streamUsageTail) bytes() []byte { return s.buf }

// usageFromOpenAIChatSSE scans OpenAI-style SSE lines (data: …) for the last JSON
// chunk that includes a top-level "usage" object.
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

func logUpstreamChatResponse(log *slog.Logger, url string, statusCode int, upstreamModel string, stream bool, respBody []byte, respHeader http.Header, opts *ProxyOpts) {
	if log == nil {
		return
	}
	if statusOK(statusCode) && len(respBody) > 0 {
		conversationwitness.LogResponseWitness(log, stream, respBody)
		if opts != nil && opts.WitnessEmitPayloadSample {
			mc := opts.WitnessPayloadSampleMaxRunes
			if mc <= 0 {
				mc = 256
			}
			conversationwitness.LogResponsePayloadSample(log, true, mc, stream, respBody)
		}
	}
	path := "/v1/chat/completions"
	args := []any{
		"msg", "chat.chimera-broker.response",
		"route", "POST /v1/chat/completions (chimera-broker)",
		"path", path,
		"target", url,
		"statusCode", statusCode,
		"upstreamModel", upstreamModel,
		"stream", stream,
		"responseBytes", len(respBody),
	}
	if statusCode == http.StatusNotFound {
		args = append(args, "upstreamErrorKind", "model_not_found")
	}
	if respHeader != nil {
		if hdr := formatResponseHeadersForLog(respHeader, 700); hdr != "" {
			args = append(args, "responseHeaders", hdr)
		}
	}
	if len(respBody) > 0 {
		var ex string
		if stream && len(respBody) > 400 {
			raw := string(respBody)
			if tailStart := len(raw) - 1200; tailStart > 0 {
				ex = truncateRunes(raw[tailStart:], 400)
			} else {
				ex = truncateRunes(raw, 400)
			}
		} else {
			ex = truncateRunes(string(respBody), 400)
		}
		if ex != "" {
			args = append(args, "responseBodyExcerpt", ex)
			if !statusOK(statusCode) {
				if sum := excerptOpenAIStyleErrorMessage(respBody, 220); sum != "" {
					args = append(args, "upstreamErrorExcerpt", sum)
				}
			}
		}
		if statusOK(statusCode) {
			if fr := finishReasonFromChatJSON(stream, respBody); fr != "" {
				args = append(args, "finish_reason", fr)
			}
		}
		if p, c, tot, okU := usageFromChatCompletionJSON(respBody); okU {
			args = append(args,
				"usagePromptTokens", p,
				"usageCompletionTokens", c,
				"usageTotalTokens", tot,
			)
		} else if stream {
			if p, c, tot, okS := usageFromOpenAIChatSSE(respBody); okS {
				args = append(args,
					"usagePromptTokens", p,
					"usageCompletionTokens", c,
					"usageTotalTokens", tot,
				)
			}
		}
	}
	if statusOK(statusCode) {
		lc := []any{
			"msg", naming.MsgConversationBrokerCompleted,
			"upstreamModel", upstreamModel,
			"statusCode", statusCode,
			"stream", stream,
			"responseBytes", len(respBody),
		}
		if p, c, tot, okU := usageFromChatCompletionJSON(respBody); okU {
			lc = append(lc, "usagePromptTokens", p, "usageCompletionTokens", c, "usageTotalTokens", tot)
		} else if stream {
			if p, c, tot, okS := usageFromOpenAIChatSSE(respBody); okS {
				lc = append(lc, "usagePromptTokens", p, "usageCompletionTokens", c, "usageTotalTokens", tot)
			}
		}
		log.Info("conversation broker completed", lc...)
	} else {
		log.Warn("conversation broker failed", "msg", naming.MsgConversationBrokerFailed,
			"upstreamModel", upstreamModel, "statusCode", statusCode, "stream", stream,
			"err", upstreamErrSummary(statusCode, respBody))
	}
	log.Info("chimera-broker chat response", args...)
}

func upstreamErrSummary(statusCode int, respBody []byte) string {
	if len(respBody) == 0 {
		return http.StatusText(statusCode)
	}
	return truncateRunes(string(respBody), 200)
}

type flushWriter struct {
	w io.Writer
	f http.Flusher
}

func (fw *flushWriter) Write(p []byte) (int, error) {
	n, err := fw.w.Write(p)
	if n > 0 {
		fw.f.Flush()
	}
	return n, err
}

func statusOK(code int) bool {
	return code >= 200 && code < 300
}

func cloneRawMap(m map[string]json.RawMessage) map[string]json.RawMessage {
	out := make(map[string]json.RawMessage, len(m)+2)
	for k, v := range m {
		out[k] = v
	}
	return out
}

func mustRawJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage("null")
	}
	return b
}

// WithVirtualModelFallback implements src/chat.ts chatWithVirtualModelFallback.
func WithVirtualModelFallback(ctx context.Context, w http.ResponseWriter, initialUpstream string, fallbackChain []string, baseURL, apiKey string, stream bool, body map[string]json.RawMessage, timeout time.Duration, log *slog.Logger, rec gatewaymetrics.Recorder, guard *providerlimits.Guard, opts *ProxyOpts) {
	t0 := time.Now()
	clientModel := clientModelFromBody(body)
	deliver := func(st int, stream bool, nb int64) {
		if opts != nil && opts.OnChatDelivery != nil {
			opts.OnChatDelivery(st, stream, nb, time.Since(t0).Milliseconds())
		}
	}
	var innerOpts *ProxyOpts
	if opts != nil {
		cp := *opts
		cp.SuppressChatDelivery = true
		cp.OnChatDelivery = nil
		innerOpts = &cp
	} else {
		innerOpts = &ProxyOpts{SuppressChatDelivery: true}
	}

	start := routing.StartingFallbackIndex(initialUpstream, fallbackChain)
	var chain []string
	if len(fallbackChain) > 0 {
		chain = fallbackChain[start:]
	} else if initialUpstream != "" {
		chain = []string{initialUpstream}
	}

	if len(chain) == 0 {
		writeJSONError(w, http.StatusServiceUnavailable, map[string]any{
			"message": "No upstream models configured (routing.fallback_chain empty and no initial model).",
			"type":    "gateway_config",
		})
		deliver(http.StatusServiceUnavailable, false, 0)
		return
	}

	var attemptFailures []fallbackFailureRecord
	// Upstream ids that returned HTTP 413 on this request are not tried again (duplicate ids in chain).
	excluded413 := make(map[string]struct{})

	for i, upstreamModel := range chain {
		if _, skip := excluded413[upstreamModel]; skip {
			if log != nil {
				log.Debug("virtual model skipping model (413 earlier this request)", "msg", "chat.routing.virtual_model_skipped", "upstreamModel", upstreamModel, "index", i)
			}
			continue
		}
		if log != nil {
			attemptArgs := []any{
				"msg", "chat.routing.attempt",
				"attempt", i + 1, "upstreamModel", upstreamModel, "chainLen", len(chain),
			}
			if len(chain) > 1 {
				log.Info("routing attempt", attemptArgs...)
			} else {
				log.Debug("routing attempt", attemptArgs...)
			}
		}
		out, est, err := prepareChatPayload(upstreamModel, stream, body)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, map[string]any{"message": "marshal request", "type": "gateway_internal"})
			deliver(http.StatusInternalServerError, false, 0)
			return
		}
		if guard != nil {
			admission := requestAdmission(body, out, est)
			d, gerr := guard.Allow(ctx, upstreamModel, admission)
			if gerr != nil && log != nil {
				log.Warn("provider limits admission query failed; allowing attempt", "msg", "chat.provider_limits.query_failed", "err", gerr, "upstreamModel", upstreamModel)
			}
			if !d.Allowed {
				if log != nil {
					skipArgs := providerLimitsBlockedLogArgs(upstreamModel, d, admission, guard)
					log.Info("skipping upstream model (provider limits)", skipArgs...)
				}
				if i < len(chain)-1 {
					continue
				}
				writeJSONError(w, http.StatusTooManyRequests, map[string]any{
					"message": providerLimitsDenyMessage(d.Reason, true),
					"type":    "gateway_provider_limits",
				})
				deliver(http.StatusTooManyRequests, false, 0)
				return
			}
		}
		r := proxyChatCompletionPayload(ctx, w, baseURL, apiKey, upstreamModel, stream, out, est, timeout, log, rec, innerOpts)
		if r.Status == http.StatusRequestEntityTooLarge ||
			upstreamErrorIndicatesContextOverflow(r.Status, r.JSONBody, r.ErrMessage) {
			excluded413[upstreamModel] = struct{}{}
		}
		if r.Stream {
			if log != nil {
				if len(chain) > 1 {
					log.Info("routing resolved",
						"msg", "chat.routing.resolved",
						"upstreamModel", upstreamModel, "attempt", i+1, "chainLen", len(chain), "stream", true)
				}
				log.Info("conversation routed", conversationRoutingLogArgs(clientModel, upstreamModel, i+1, len(chain), true, est)...)
			}
			deliver(http.StatusOK, true, r.DeliveryBytes)
			return
		}
		if r.ErrMessage != "" {
			if shouldRetryVirtualModelFallback(r.Status, nil, r.ErrMessage) && hasMoreFallbackCandidates(chain, i, excluded413) {
				appendFallbackFailure(&attemptFailures, upstreamModel, r.Status, nil, r.ErrMessage)
				if upstreamErrorIndicatesRateLimit(r.Status, nil, r.ErrMessage) {
					logRateLimitRouting(log, upstreamModel, i+1, len(chain), true, nil, r.ErrMessage)
				}
				if log != nil {
					log.Info("retrying next fallback model", "msg", "chat.routing.fallback", "upstreamModel", upstreamModel, "status", r.Status, "willRetry", true)
					log.Info("conversation fallback attempted", "msg", naming.MsgConversationFallbackAttempted,
						"upstreamModel", upstreamModel, "prev_status", r.Status, "attempt", i+1, "chainLen", len(chain))
				}
				continue
			}
			if shouldRetryVirtualModelFallback(r.Status, nil, r.ErrMessage) && !hasMoreFallbackCandidates(chain, i, excluded413) {
				appendFallbackFailure(&attemptFailures, upstreamModel, r.Status, nil, r.ErrMessage)
				break
			}
			writeJSONError(w, r.Status, map[string]any{"message": r.ErrMessage, "type": "gateway_upstream"})
			deliver(r.Status, false, 0)
			return
		}
		if r.JSONBody != nil {
			if shouldRetryVirtualModelFallback(r.Status, r.JSONBody, "") && hasMoreFallbackCandidates(chain, i, excluded413) {
				appendFallbackFailure(&attemptFailures, upstreamModel, r.Status, r.JSONBody, "")
				if r.Status == http.StatusNotFound {
					logModelNotFoundRouting(log, upstreamModel, i+1, len(chain), true, r.JSONBody)
				} else if upstreamErrorIndicatesRateLimit(r.Status, r.JSONBody, "") {
					logRateLimitRouting(log, upstreamModel, i+1, len(chain), true, r.JSONBody, "")
					if log != nil {
						log.Info("retrying next fallback model", "msg", "chat.routing.fallback", "upstreamModel", upstreamModel, "status", r.Status, "willRetry", true)
						log.Info("conversation fallback attempted", "msg", naming.MsgConversationFallbackAttempted,
							"upstreamModel", upstreamModel, "prev_status", r.Status, "attempt", i+1, "chainLen", len(chain))
					}
				} else if log != nil {
					log.Info("retrying next fallback model", "msg", "chat.routing.fallback", "upstreamModel", upstreamModel, "status", r.Status, "willRetry", true)
					log.Info("conversation fallback attempted", "msg", naming.MsgConversationFallbackAttempted,
						"upstreamModel", upstreamModel, "prev_status", r.Status, "attempt", i+1, "chainLen", len(chain))
				}
				continue
			}
			if !shouldRetryVirtualModelFallback(r.Status, r.JSONBody, "") {
				if log != nil {
					if len(chain) > 1 {
						log.Info("routing resolved",
							"msg", "chat.routing.resolved",
							"upstreamModel", upstreamModel, "attempt", i+1, "chainLen", len(chain), "statusCode", r.Status, "stream", false)
					}
					log.Info("conversation routed", conversationRoutingLogArgs(clientModel, upstreamModel, i+1, len(chain), false, est)...)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(r.Status)
				_, _ = w.Write(r.JSONBody)
				deliver(r.Status, false, int64(len(r.JSONBody)))
				return
			}
			appendFallbackFailure(&attemptFailures, upstreamModel, r.Status, r.JSONBody, "")
			if r.Status == http.StatusNotFound {
				logModelNotFoundRouting(log, upstreamModel, i+1, len(chain), false, r.JSONBody)
			} else if upstreamErrorIndicatesRateLimit(r.Status, r.JSONBody, "") {
				logRateLimitRouting(log, upstreamModel, i+1, len(chain), false, r.JSONBody, "")
			}
			break
		}
	}

	if len(attemptFailures) > 0 {
		var wrap []byte
		statusCode := http.StatusServiceUnavailable
		if allAttemptsRateLimited(attemptFailures) {
			wrap = buildRateLimitExhaustedJSONBody(attemptFailures, len(chain))
			statusCode = http.StatusBadRequest
		} else {
			wrap = buildFallbackExhaustedJSONBody(attemptFailures)
		}
		attemptsJSON, _ := json.Marshal(attemptFailures)
		if log != nil {
			log.Warn("conversation fallback exhausted", "msg", naming.MsgConversationFallbackExhausted,
				"chainLen", len(chain), "excluded_413_count", len(excluded413),
				"attemptCount", len(attemptFailures), "attempts", string(attemptsJSON),
				"rateLimitExhausted", allAttemptsRateLimited(attemptFailures))
			log.Warn("chimera broker fallback chain exhausted", "msg", "chat.chimera-broker.fallback_chain_exhausted",
				"attemptCount", len(attemptFailures), "attempts", string(attemptsJSON))
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		_, _ = w.Write(wrap)
		deliver(statusCode, false, int64(len(wrap)))
		return
	}

	if log != nil {
		log.Warn("conversation fallback exhausted", "msg", naming.MsgConversationFallbackExhausted,
			"chainLen", len(chain), "excluded_413_count", len(excluded413))
	}
	writeJSONError(w, http.StatusServiceUnavailable, map[string]any{
		"message": "Exhausted fallback chain without a successful completion.",
		"type":    "gateway_exhausted",
	})
	deliver(http.StatusServiceUnavailable, false, 0)
}

func writeJSONError(w http.ResponseWriter, code int, errObj map[string]any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": errObj})
}
