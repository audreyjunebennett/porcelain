// Package chimeraBrokeradmin is a minimal HTTP client for Chimera Broker chimera-broker-http management routes
// (GET/PUT /api/providers/{provider}). Used by the gateway admin BFF; browsers never call BiFrost directly.
package bifrostadmin

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	internalhttp "github.com/lynn/porcelain/chimera/internal/http"
)

// Client calls BiFrost management HTTP APIs on the same base URL as the OpenAI-compatible root
// (e.g. http://127.0.0.1:8080).
type Client struct {
	// BaseURL is the upstream root without a trailing slash, e.g. http://127.0.0.1:8080
	BaseURL string
	// BearerToken is sent as Authorization: Bearer … when non-empty (BiFrost may require it when governance auth is on).
	BearerToken string
	HTTPClient  *http.Client
}

func (c *Client) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return &http.Client{Timeout: 30 * time.Second}
}

func (c *Client) authHeader() string {
	return internalhttp.BearerAuthValue(c.BearerToken)
}

// GetProvider returns the raw JSON body from GET /api/providers/{provider} and the HTTP status code.
func (c *Client) GetProvider(ctx context.Context, provider string) (body []byte, status int, err error) {
	base := strings.TrimSuffix(strings.TrimSpace(c.BaseURL), "/")
	if base == "" {
		return nil, 0, fmt.Errorf("chimera-broker-admin: empty BaseURL")
	}
	p := strings.TrimSpace(provider)
	if p == "" {
		return nil, 0, fmt.Errorf("chimera-broker-admin: empty provider")
	}
	u := base + "/api/providers/" + p
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, 0, err
	}
	if h := c.authHeader(); h != "" {
		req.Header.Set("Authorization", h)
	}
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, 0, err
	}
	b, err := internalhttp.ReadAndCloseLimited(resp.Body, 8<<20)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return b, resp.StatusCode, nil
}

// PutProvider sends PUT /api/providers/{provider} with the given JSON body (full provider config per BiFrost API).
// respBody is a limited snippet of the response body (for error messages).
func (c *Client) PutProvider(ctx context.Context, provider string, jsonBody []byte) (status int, respBody []byte, err error) {
	base := strings.TrimSuffix(strings.TrimSpace(c.BaseURL), "/")
	if base == "" {
		return 0, nil, fmt.Errorf("chimera-broker-admin: empty BaseURL")
	}
	p := strings.TrimSpace(provider)
	if p == "" {
		return 0, nil, fmt.Errorf("chimera-broker-admin: empty provider")
	}
	u := base + "/api/providers/" + p
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, u, strings.NewReader(string(jsonBody)))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if h := c.authHeader(); h != "" {
		req.Header.Set("Authorization", h)
	}
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return 0, nil, err
	}
	b, rerr := internalhttp.ReadAndCloseLimited(resp.Body, 1<<20)
	if rerr != nil {
		return resp.StatusCode, b, rerr
	}
	return resp.StatusCode, b, nil
}
