// Package qdrant implements vectorstore.Store against Qdrant's HTTP REST API
// (default port 6333). It is intentionally minimal and stateless: callers
// construct one Client per process and share it.
//
// Reference: https://qdrant.tech/documentation/concepts/collections/
package qdrant

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/vectorstore"
)

// Client is a thread-safe Qdrant HTTP REST client implementing vectorstore.Store.
type Client struct {
	baseURL string
	apiKey  string
	hc      *http.Client
}

// New returns a Client. baseURL must include scheme (e.g. http://127.0.0.1:6333).
// apiKey is sent as the api-key header when non-empty.
func New(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		apiKey:  strings.TrimSpace(apiKey),
		hc:      &http.Client{Timeout: 30 * time.Second},
	}
}

// WithHTTPClient swaps the underlying http.Client (tests).
func (c *Client) WithHTTPClient(hc *http.Client) *Client {
	c.hc = hc
	return c
}

func (c *Client) do(ctx context.Context, method, path string, body any, out any) error {
	var rdr io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("qdrant marshal: %w", err)
		}
		rdr = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, rdr)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.apiKey != "" {
		req.Header.Set("api-key", c.apiKey)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("qdrant %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("qdrant %s %s: status %d: %s", method, path, resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("qdrant decode %s: %w", path, err)
	}
	return nil
}

// EnsureCollection creates the collection (cosine, dim) if missing; no-op on existing.
func (c *Client) EnsureCollection(ctx context.Context, name string, dim int) error {
	if dim <= 0 {
		return errors.New("qdrant ensure: dim must be > 0")
	}
	var info struct {
		Result struct {
			Config struct {
				Params struct {
					Vectors json.RawMessage `json:"vectors"`
				} `json:"params"`
			} `json:"config"`
		} `json:"result"`
	}
	err := c.do(ctx, http.MethodGet, "/collections/"+name, nil, &info)
	if err == nil {
		return nil // exists
	}
	// Try create. We don't differentiate 404 vs other errors strictly: if the
	// PUT also fails, surface that error.
	body := map[string]any{
		"vectors": map[string]any{
			"size":     dim,
			"distance": "Cosine",
		},
	}
	if err := c.do(ctx, http.MethodPut, "/collections/"+name, body, nil); err != nil {
		return err
	}
	// Best-effort payload index on tenant_id / project_id for filter perf.
	for _, field := range []string{"tenant_id", "project_id", "flavor_id", "source", "content_sha256", "client_content_hash"} {
		_ = c.do(ctx, http.MethodPut, "/collections/"+name+"/index", map[string]any{
			"field_name":   field,
			"field_schema": "keyword",
		}, nil)
	}
	return nil
}

type qdrantPoint struct {
	ID      string                 `json:"id"`
	Vector  []float32              `json:"vector"`
	Payload map[string]interface{} `json:"payload"`
}

func toQPayload(p vectorstore.Payload) map[string]interface{} {
	m := map[string]interface{}{
		"tenant_id":  p.TenantID,
		"project_id": p.ProjectID,
		"text":       p.Text,
		"source":     p.Source,
	}
	if p.FlavorID != "" {
		m["flavor_id"] = p.FlavorID
	}
	if p.CreatedAt != 0 {
		m["created_at"] = p.CreatedAt
	}
	if strings.TrimSpace(p.ContentSHA256) != "" {
		m["content_sha256"] = p.ContentSHA256
	}
	if strings.TrimSpace(p.ClientContentHash) != "" {
		m["client_content_hash"] = p.ClientContentHash
	}
	return m
}

func fromQPayload(m map[string]any) vectorstore.Payload {
	p := vectorstore.Payload{}
	if v, ok := m["tenant_id"].(string); ok {
		p.TenantID = v
	}
	if v, ok := m["project_id"].(string); ok {
		p.ProjectID = v
	}
	if v, ok := m["flavor_id"].(string); ok {
		p.FlavorID = v
	}
	if v, ok := m["text"].(string); ok {
		p.Text = v
	}
	if v, ok := m["source"].(string); ok {
		p.Source = v
	}
	if v, ok := m["created_at"].(float64); ok {
		p.CreatedAt = int64(v)
	}
	if v, ok := m["content_sha256"].(string); ok {
		p.ContentSHA256 = v
	}
	if v, ok := m["client_content_hash"].(string); ok {
		p.ClientContentHash = v
	}
	return p
}

// Upsert writes/overwrites the given points (synchronous wait=true).
func (c *Client) Upsert(ctx context.Context, collection string, points []vectorstore.Point) error {
	if len(points) == 0 {
		return nil
	}
	qp := make([]qdrantPoint, 0, len(points))
	for _, p := range points {
		qp = append(qp, qdrantPoint{ID: p.ID, Vector: p.Vector, Payload: toQPayload(p.Payload)})
	}
	body := map[string]any{"points": qp}
	return c.do(ctx, http.MethodPut, "/collections/"+collection+"/points?wait=true", body, nil)
}

// Search runs a top-k vector search filtered by tenant/project/flavor when set.
func (c *Client) Search(ctx context.Context, collection string, vector []float32, topK int, scoreThreshold float32, filter *vectorstore.Coords) ([]vectorstore.Hit, error) {
	body := map[string]any{
		"vector":       vector,
		"limit":        topK,
		"with_payload": true,
	}
	if scoreThreshold > 0 {
		body["score_threshold"] = scoreThreshold
	}
	if filter != nil {
		conds := []map[string]any{}
		if filter.TenantID != "" {
			conds = append(conds, kvKeyword("tenant_id", filter.TenantID))
		}
		if filter.ProjectID != "" {
			conds = append(conds, kvKeyword("project_id", filter.ProjectID))
		}
		if filter.FlavorID != "" {
			conds = append(conds, kvKeyword("flavor_id", filter.FlavorID))
		}
		if len(conds) > 0 {
			body["filter"] = map[string]any{"must": conds}
		}
	}
	var resp struct {
		Result []struct {
			ID      any                    `json:"id"`
			Score   float32                `json:"score"`
			Payload map[string]interface{} `json:"payload"`
		} `json:"result"`
	}
	if err := c.do(ctx, http.MethodPost, "/collections/"+collection+"/points/search", body, &resp); err != nil {
		// Treat "collection not found" as empty results to keep retrieval graceful.
		if strings.Contains(err.Error(), "Not found") || strings.Contains(err.Error(), "doesn't exist") || strings.Contains(err.Error(), "status 404") {
			return nil, nil
		}
		return nil, err
	}
	out := make([]vectorstore.Hit, 0, len(resp.Result))
	for _, r := range resp.Result {
		out = append(out, vectorstore.Hit{
			ID:      fmt.Sprint(r.ID),
			Score:   r.Score,
			Payload: fromQPayload(r.Payload),
		})
	}
	return out, nil
}

func kvKeyword(field, value string) map[string]any {
	return map[string]any{"key": field, "match": map[string]any{"value": value}}
}

// Health pings the Qdrant root; returns nil on a 2xx response.
func (c *Client) Health(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/", nil)
	if err != nil {
		return err
	}
	if c.apiKey != "" {
		req.Header.Set("api-key", c.apiKey)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("qdrant /: status %d", resp.StatusCode)
	}
	return nil
}

type scrollCursorWrap struct {
	O json.RawMessage `json:"o"`
}

func encodeScrollCursor(raw json.RawMessage) string {
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "null" {
		return ""
	}
	b, err := json.Marshal(scrollCursorWrap{O: raw})
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func decodeScrollCursor(s string) (json.RawMessage, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	dec, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("corpus inventory cursor: %w", err)
	}
	var wrap scrollCursorWrap
	if err := json.Unmarshal(dec, &wrap); err != nil {
		return nil, fmt.Errorf("corpus inventory cursor: %w", err)
	}
	if len(wrap.O) == 0 || strings.TrimSpace(string(wrap.O)) == "null" {
		return nil, nil
	}
	return wrap.O, nil
}

// ScrollPoints implements vectorstore.Store for corpus inventory pagination.
func (c *Client) ScrollPoints(ctx context.Context, collection string, filter *vectorstore.Coords, limit int, cursor string) (vectorstore.ScrollBatch, error) {
	if limit <= 0 {
		limit = 256
	}
	if limit > 2000 {
		limit = 2000
	}
	body := map[string]any{
		"limit":        limit,
		"with_payload": true,
		"with_vector":  false,
	}
	if filter != nil {
		conds := []map[string]any{}
		if filter.TenantID != "" {
			conds = append(conds, kvKeyword("tenant_id", filter.TenantID))
		}
		if filter.ProjectID != "" {
			conds = append(conds, kvKeyword("project_id", filter.ProjectID))
		}
		if filter.FlavorID != "" {
			conds = append(conds, kvKeyword("flavor_id", filter.FlavorID))
		}
		if len(conds) > 0 {
			body["filter"] = map[string]any{"must": conds}
		}
	}
	off, err := decodeScrollCursor(cursor)
	if err != nil {
		return vectorstore.ScrollBatch{}, err
	}
	if len(off) > 0 && strings.TrimSpace(string(off)) != "null" {
		var offVal any
		if err := json.Unmarshal(off, &offVal); err != nil {
			return vectorstore.ScrollBatch{}, fmt.Errorf("corpus inventory cursor offset: %w", err)
		}
		body["offset"] = offVal
	}
	var resp struct {
		Result struct {
			Points []struct {
				ID      any                    `json:"id"`
				Payload map[string]interface{} `json:"payload"`
			} `json:"points"`
			NextPageOffset json.RawMessage `json:"next_page_offset"`
		} `json:"result"`
	}
	path := "/collections/" + collection + "/points/scroll"
	if err := c.do(ctx, http.MethodPost, path, body, &resp); err != nil {
		if strings.Contains(err.Error(), "Not found") || strings.Contains(err.Error(), "doesn't exist") || strings.Contains(err.Error(), "status 404") {
			return vectorstore.ScrollBatch{}, nil
		}
		return vectorstore.ScrollBatch{}, err
	}
	out := make([]vectorstore.PointPayload, 0, len(resp.Result.Points))
	for _, pt := range resp.Result.Points {
		pl := fromQPayload(pt.Payload)
		out = append(out, vectorstore.PointPayload{
			ID:      fmt.Sprint(pt.ID),
			Payload: pl,
		})
	}
	next := encodeScrollCursor(resp.Result.NextPageOffset)
	return vectorstore.ScrollBatch{Points: out, NextCursor: next}, nil
}

// Stats returns approximate point count + vector dim.
func (c *Client) Stats(ctx context.Context, collection string) (vectorstore.Stats, error) {
	var resp struct {
		Result struct {
			PointsCount int64 `json:"points_count"`
			Config      struct {
				Params struct {
					// "vectors" can be either {size: N, distance: ...} or {<name>: {...}}
					Vectors json.RawMessage `json:"vectors"`
				} `json:"params"`
			} `json:"config"`
		} `json:"result"`
	}
	if err := c.do(ctx, http.MethodGet, "/collections/"+collection, nil, &resp); err != nil {
		return vectorstore.Stats{Collection: collection}, err
	}
	dim := 0
	var simple struct {
		Size int `json:"size"`
	}
	if json.Unmarshal(resp.Result.Config.Params.Vectors, &simple) == nil && simple.Size > 0 {
		dim = simple.Size
	} else {
		// named-vectors form: take first entry.
		var named map[string]struct {
			Size int `json:"size"`
		}
		if json.Unmarshal(resp.Result.Config.Params.Vectors, &named) == nil {
			for _, v := range named {
				if v.Size > 0 {
					dim = v.Size
					break
				}
			}
		}
	}
	return vectorstore.Stats{Collection: collection, Points: resp.Result.PointsCount, VectorDim: dim}, nil
}

// DeleteBySource removes all points whose payload.source matches.
func (c *Client) DeleteBySource(ctx context.Context, collection, source string) error {
	body := map[string]any{
		"filter": map[string]any{"must": []map[string]any{kvKeyword("source", source)}},
	}
	return c.do(ctx, http.MethodPost, "/collections/"+collection+"/points/delete?wait=true", body, nil)
}
