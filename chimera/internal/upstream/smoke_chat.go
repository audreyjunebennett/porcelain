package upstream

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

const smokeMaxRead = 2048

// SmokeChatCompletion POSTs a single-token chat completion to exercise the upstream with the given model.
func SmokeChatCompletion(ctx context.Context, baseURL, apiKey, model string, timeout time.Duration, log *slog.Logger) (status int, ok bool, detail string) {
	root := strings.TrimSuffix(baseURL, "/")
	url := root + "/v1/chat/completions"
	body := map[string]any{
		"model":       model,
		"messages":    []map[string]string{{"role": "user", "content": "."}},
		"max_tokens":  1,
		"temperature": 0,
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return 0, false, err.Error()
	}
	client := &http.Client{Timeout: timeout}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return 0, false, err.Error()
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	res, err := client.Do(req)
	if err != nil {
		if log != nil {
			log.Debug("smoke chat completion failed", "msg", "upstream.smoke_chat.failed", "err", err, "target", url)
		}
		return 0, false, err.Error()
	}
	defer res.Body.Close()
	b, err := io.ReadAll(io.LimitReader(res.Body, smokeMaxRead))
	if err != nil {
		return res.StatusCode, false, err.Error()
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		d := strings.TrimSpace(string(b))
		if d == "" {
			d = fmt.Sprintf("HTTP %d", res.StatusCode)
		}
		return res.StatusCode, false, d
	}
	return res.StatusCode, true, strings.TrimSpace(string(b))
}
