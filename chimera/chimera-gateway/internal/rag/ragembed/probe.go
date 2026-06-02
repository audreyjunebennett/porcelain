package ragembed

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// ProbeDim calls the embeddings endpoint once and returns the vector length for model.
func ProbeDim(ctx context.Context, url, apiKey, model string, hc *http.Client) (int, error) {
	url = strings.TrimSpace(url)
	model = strings.TrimSpace(model)
	if url == "" {
		return 0, fmt.Errorf("embed probe: empty url")
	}
	if model == "" {
		return 0, fmt.Errorf("embed probe: empty model")
	}
	if hc == nil {
		hc = &http.Client{Timeout: 60 * time.Second}
	}
	c := New(url, apiKey, model).WithHTTPClient(hc)
	vec, err := c.EmbedOne(ctx, "dim-probe")
	if err != nil {
		return 0, err
	}
	if len(vec) <= 0 {
		return 0, fmt.Errorf("embed probe: empty vector")
	}
	return len(vec), nil
}

// ParseEmbeddingVectorLen extracts vector length from a raw /v1/embeddings JSON body.
func ParseEmbeddingVectorLen(raw []byte) (int, error) {
	var parsed struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error,omitempty"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return 0, err
	}
	if parsed.Error != nil && parsed.Error.Message != "" {
		return 0, fmt.Errorf("embed probe: upstream error: %s", parsed.Error.Message)
	}
	if len(parsed.Data) == 0 || len(parsed.Data[0].Embedding) == 0 {
		return 0, fmt.Errorf("embed probe: no embedding vector")
	}
	return len(parsed.Data[0].Embedding), nil
}
