package freecatalog

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

const defaultUserAgent = "chimera-gateway-free-catalog/1.0 (+https://github.com/lynn/porcelain)"

// FetchURL downloads a document with a browser-like User-Agent and returns the raw body.
func FetchURL(ctx context.Context, client *http.Client, url string) ([]byte, error) {
	if client == nil {
		client = &http.Client{Timeout: 45 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", defaultUserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	b, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("GET %s: status %d", url, res.StatusCode)
	}
	return b, nil
}
