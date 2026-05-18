// Package embed wraps an OpenAI-compatible POST /v1/embeddings endpoint.
//
// The same client is used by ingest (chunks) and query-time retrieval. It is
// intentionally minimal: one HTTP call, one model, batched input array.
package embed

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client is an OpenAI-compatible embeddings client.
type Client struct {
	url    string
	apiKey string
	model  string
	hc     *http.Client
}

// New returns a Client. url is the absolute embeddings endpoint (e.g.
func New(url, apiKey, model string) *Client {
	return &Client{
		url:    strings.TrimSpace(url),
		apiKey: strings.TrimSpace(apiKey),
		model:  strings.TrimSpace(model),
		hc:     &http.Client{Timeout: 60 * time.Second},
	}
}

// WithHTTPClient swaps the underlying http.Client (tests).
func (c *Client) WithHTTPClient(hc *http.Client) *Client {
	c.hc = hc
	return c
}

// EmbedBatch returns one vector per input string in order.
func (c *Client) EmbedBatch(ctx context.Context, inputs []string) ([][]float32, error) {
	if len(inputs) == 0 {
		return nil, nil
	}
	if c.url == "" {
		return nil, fmt.Errorf("embed: empty url")
	}
	if c.model == "" {
		return nil, fmt.Errorf("embed: empty model")
	}
	body, err := json.Marshal(map[string]any{
		"model":           c.model,
		"input":           inputs,
		"encoding_format": "float",
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embed POST %s: %w", c.url, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("embed: status %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var parsed struct {
		Data []struct {
			Index     int       `json:"index"`
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error,omitempty"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("embed: decode: %w (body: %s)", err, truncate(string(raw), 200))
	}
	if parsed.Error != nil && parsed.Error.Message != "" {
		return nil, fmt.Errorf("embed: upstream error: %s", parsed.Error.Message)
	}
	if len(parsed.Data) != len(inputs) {
		return nil, fmt.Errorf("embed: got %d vectors, want %d", len(parsed.Data), len(inputs))
	}
	out := make([][]float32, len(inputs))
	for _, d := range parsed.Data {
		if d.Index < 0 || d.Index >= len(inputs) {
			return nil, fmt.Errorf("embed: out-of-range index %d", d.Index)
		}
		out[d.Index] = d.Embedding
	}
	for i, v := range out {
		if v == nil {
			return nil, fmt.Errorf("embed: missing vector at index %d", i)
		}
	}
	return out, nil
}

// EmbedOne is a convenience wrapper for single-string embedding.
func (c *Client) EmbedOne(ctx context.Context, s string) ([]float32, error) {
	v, err := c.EmbedBatch(ctx, []string{s})
	if err != nil {
		return nil, err
	}
	return v[0], nil
}

// Model returns the configured embedding model id (used by /v1/indexer/config).
func (c *Client) Model() string { return c.model }

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
