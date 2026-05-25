// Package embedprobe probes OpenAI-compatible embedding endpoints for indexer health.
package embedprobe

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

// Probe posts a minimal embedding request and validates vector dimension.
func Probe(ctx context.Context, url, model string, wantDim int) error {
	url = strings.TrimSpace(url)
	if url == "" {
		return fmt.Errorf("empty embedding url")
	}
	model = strings.TrimSpace(model)
	if model == "" {
		return fmt.Errorf("empty embedding model")
	}
	body, err := json.Marshal(map[string]any{
		"model":           model,
		"input":           []string{"health"},
		"encoding_format": "float",
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	hc := &http.Client{Timeout: 15 * time.Second}
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("embed probe status %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var parsed struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return fmt.Errorf("embed probe decode: %w", err)
	}
	if len(parsed.Data) == 0 || len(parsed.Data[0].Embedding) == 0 {
		return fmt.Errorf("embed probe returned no vectors")
	}
	if wantDim > 0 && len(parsed.Data[0].Embedding) != wantDim {
		return fmt.Errorf("embed probe dim %d want %d", len(parsed.Data[0].Embedding), wantDim)
	}
	return nil
}
