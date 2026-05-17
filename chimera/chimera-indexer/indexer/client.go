package indexer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	internalhttp "github.com/lynn/porcelain/chimera/internal/http"
	"github.com/lynn/porcelain/chimera/internal/naming"
	"github.com/lynn/porcelain/internal/platform/requestid"
)

// GatewayClient talks to the Chimera Gateway indexer-facing surface:
// GET /v1/indexer/config, GET /v1/indexer/workspaces, POST /v1/ingest, chunked /v1/ingest/session/*,
// GET /v1/indexer/storage/health, and optionally GET /health.
type GatewayClient struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
	// IndexRunID is optional; when set and valid, sent as X-Chimera-Index-Run-Id on indexer HTTP calls.
	IndexRunID string
}

// NewGatewayClient constructs a client with a sane default timeout.
func NewGatewayClient(baseURL, token string, timeout time.Duration) *GatewayClient {
	if timeout <= 0 {
		timeout = defaultRequestTimeout
	}
	return &GatewayClient{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Token:   token,
		HTTP:    &http.Client{Timeout: timeout},
	}
}

// IndexerConfig mirrors the JSON returned by GET /v1/indexer/config. Only
// fields the v0.2 indexer reads are typed; unknown fields are tolerated.
type IndexerConfig struct {
	GatewayVersion string `json:"gateway_version"`
	IngestPath     string `json:"ingest_path"`
	EmbeddingModel string `json:"embedding_model"`
	EmbeddingDim   int    `json:"embedding_dim"`
	ChunkSize      int    `json:"chunk_size"`
	ChunkOverlap   int    `json:"chunk_overlap"`
	MaxIngestBytes int64  `json:"max_ingest_bytes"`
	// MaxWholeFileBytes is the largest body sent as a single POST /v1/ingest;
	// larger files use the chunked session API when advertised.
	MaxWholeFileBytes   int64  `json:"max_whole_file_bytes"`
	IngestSessionPath   string `json:"ingest_session_path"`
	CorpusInventoryPath string `json:"corpus_inventory_path"`
	Headers             struct {
		Project string `json:"project"`
		Flavor  string `json:"flavor"`
	} `json:"headers"`
	// Defaults are effective RAG ingest scope when optional headers are omitted
	// (mirrors gateway GET /v1/indexer/config "defaults").
	Defaults struct {
		ProjectID string `json:"project_id"`
		FlavorID  string `json:"flavor_id"`
	} `json:"defaults"`
	TenantID    string `json:"tenant_id"`
	UserLabel   string `json:"user_label"`
	PrincipalID string `json:"principal_id"`
}

// StorageStatsResponse mirrors GET /v1/indexer/storage/stats.
type StorageStatsResponse struct {
	Object     string `json:"object"`
	Collection string `json:"collection"`
	TenantID   string `json:"tenant_id"`
	ProjectID  string `json:"project_id"`
	FlavorID   string `json:"flavor_id"`
	Points     int64  `json:"points"`
	VectorDim  int    `json:"vector_dim"`
	Available  bool   `json:"available"`
	Detail     string `json:"detail"`
}

// FetchConfig calls GET /v1/indexer/config. Optional hdrs are merged into the
// request (e.g. X-Chimera-Project / X-Chimera-Flavor-Id from indexer v0.3
// defaults) so the gateway can describe the scoped collection.
func (c *GatewayClient) FetchConfig(ctx context.Context, hdrs map[string]string) (*IndexerConfig, error) {
	req, err := c.newRequest(ctx, http.MethodGet, "/v1/indexer/config", "", nil, hdrs)
	if err != nil {
		return nil, err
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, classify(res, "/v1/indexer/config")
	}
	var out IndexerConfig
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode indexer config: %w", err)
	}
	return &out, nil
}

// FetchStorageStats calls GET /v1/indexer/storage/stats scoped by hdrs project/flavor headers.
func (c *GatewayClient) FetchStorageStats(ctx context.Context, hdrs map[string]string) (*StorageStatsResponse, error) {
	req, err := c.newRequest(ctx, http.MethodGet, "/v1/indexer/storage/stats", "", nil, hdrs)
	if err != nil {
		return nil, err
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 65536))
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		return nil, &HTTPError{Path: "/v1/indexer/storage/stats", Status: res.StatusCode, Body: string(body)}
	}
	var out StorageStatsResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("decode storage stats: %w", err)
	}
	return &out, nil
}

// HealthStatus mirrors GET /v1/indexer/storage/health.
//
// The gateway returns one of two body shapes on this endpoint:
//
//  1. The healthy / Qdrant-degraded shape:
//     {"ok":true,"status":"ok"} or
//     {"ok":false,"status":"degraded","detail":"<store error>"}
//  2. The structured-error shape (e.g. RAG not enabled):
//     {"error":{"message":"RAG is not enabled","type":"gateway_config"}}
//
// CheckHealth tolerates both. RAGDisabled is set true when the structured
// error indicates the gateway has RAG turned off so the indexer can decide
// whether to keep polling forever or surface a fatal configuration error.
type HealthStatus struct {
	OK          bool   `json:"ok"`
	Status      string `json:"status"`
	Detail      string `json:"detail"`
	Message     string `json:"-"` // populated from the structured error message.
	ErrorType   string `json:"-"` // populated from the structured error type.
	RAGDisabled bool   `json:"-"`
	HTTPStatus  int    `json:"-"`
}

// rawErrorEnvelope mirrors writeJSONError on the server side.
type rawErrorEnvelope struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

// CheckHealth calls GET /v1/indexer/storage/health and returns the parsed
// payload. A 200 with `ok:false` is not an error; a non-200 with the
// healthy/degraded shape is also not an error (so the resume loop can poll
// forever). Truly unexpected statuses still bubble up as HTTPError.
func (c *GatewayClient) CheckHealth(ctx context.Context) (*HealthStatus, error) {
	req, err := c.newRequest(ctx, http.MethodGet, "/v1/indexer/storage/health", "", nil, nil)
	if err != nil {
		return nil, err
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
	out := &HealthStatus{HTTPStatus: res.StatusCode}
	// Try the structured-error shape first; if it parses and has a message,
	// surface it without erroring so the caller can keep polling.
	var env rawErrorEnvelope
	if jerr := json.Unmarshal(body, &env); jerr == nil && env.Error.Message != "" {
		out.OK = false
		out.Status = "degraded"
		out.Message = env.Error.Message
		out.ErrorType = env.Error.Type
		out.Detail = env.Error.Message
		out.RAGDisabled = env.Error.Type == "gateway_config" ||
			strings.Contains(strings.ToLower(env.Error.Message), "rag is not enabled")
		return out, nil
	}
	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusServiceUnavailable {
		return nil, &HTTPError{Path: "/v1/indexer/storage/health", Status: res.StatusCode, Body: string(body)}
	}
	if err := json.Unmarshal(body, out); err != nil {
		return nil, fmt.Errorf("decode health (status %d, body=%q): %w", res.StatusCode, truncate(string(body), 200), err)
	}
	return out, nil
}

// GatewayRootHealth summarizes GET /health (gateway-wide readiness).
type GatewayRootHealth struct {
	OK       bool
	Status   string `json:"status"`
	Degraded bool   `json:"degraded"`
}

// CheckGatewayRootHealth calls GET /health. A 200 with no degraded flag is OK;
// 503 or JSON with degraded=true is not OK. 5xx other than 503 returns an error.
func (c *GatewayClient) CheckGatewayRootHealth(ctx context.Context) (*GatewayRootHealth, error) {
	req, err := c.newRequest(ctx, http.MethodGet, "/health", "", nil, nil)
	if err != nil {
		return nil, err
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	b, err := io.ReadAll(io.LimitReader(res.Body, 8192))
	if err != nil {
		return nil, err
	}
	switch res.StatusCode {
	case http.StatusOK:
		var m map[string]any
		if json.Unmarshal(b, &m) != nil {
			return &GatewayRootHealth{OK: true, Status: "ok"}, nil
		}
		if dg, _ := m["degraded"].(bool); dg {
			st, _ := m["status"].(string)
			return &GatewayRootHealth{OK: false, Status: st, Degraded: true}, nil
		}
		st, _ := m["status"].(string)
		return &GatewayRootHealth{OK: true, Status: st}, nil
	case http.StatusServiceUnavailable:
		return &GatewayRootHealth{OK: false, Status: "degraded", Degraded: true}, nil
	default:
		if res.StatusCode >= 500 {
			return nil, &HTTPError{Path: "/health", Status: res.StatusCode, Body: string(b)}
		}
		return &GatewayRootHealth{OK: false, Status: fmt.Sprintf("http_%d", res.StatusCode)}, nil
	}
}

// CorpusInventoryEntry mirrors one element of GET /v1/indexer/corpus/inventory.
type CorpusInventoryEntry struct {
	Source            string `json:"source"`
	ContentSHA256     string `json:"content_sha256"`
	ClientContentHash string `json:"client_content_hash"`
}

// CorpusInventoryResponse is the JSON body from corpus inventory.
type CorpusInventoryResponse struct {
	Object     string                 `json:"object"`
	Entries    []CorpusInventoryEntry `json:"entries"`
	HasMore    bool                   `json:"has_more"`
	NextCursor string                 `json:"next_cursor"`
	TenantID   string                 `json:"tenant_id"`
	ProjectID  string                 `json:"project_id"`
	FlavorID   string                 `json:"flavor_id"`
}

// FetchCorpusInventoryPage calls GET /v1/indexer/corpus/inventory (or path
// from indexer config) with limit and optional opaque cursor.
func (c *GatewayClient) FetchCorpusInventoryPage(ctx context.Context, path string, limit int, cursor string, hdrs map[string]string) (*CorpusInventoryResponse, error) {
	p := strings.TrimSpace(path)
	if p == "" {
		p = "/v1/indexer/corpus/inventory"
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	q := url.Values{}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	if strings.TrimSpace(cursor) != "" {
		q.Set("cursor", cursor)
	}
	reqPath := p
	if enc := q.Encode(); enc != "" {
		reqPath = p + "?" + enc
	}
	req, err := c.newRequest(ctx, http.MethodGet, reqPath, "", nil, hdrs)
	if err != nil {
		return nil, err
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, classify(res, reqPath)
	}
	var out CorpusInventoryResponse
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode corpus inventory: %w", err)
	}
	return &out, nil
}

// CorpusInventoryRow is a merged view of one source path for reconciliation.
type CorpusInventoryRow struct {
	ContentSHA256     string
	ClientContentHash string
}

// FetchCorpusInventoryAll paginates until has_more is false, merging entries
// by source (later pages override earlier on collision).
func (c *GatewayClient) FetchCorpusInventoryAll(ctx context.Context, path string, hdrs map[string]string) (map[string]CorpusInventoryRow, error) {
	const pageSize = 512
	out := map[string]CorpusInventoryRow{}
	cursor := ""
	for {
		page, err := c.FetchCorpusInventoryPage(ctx, path, pageSize, cursor, hdrs)
		if err != nil {
			return nil, err
		}
		for _, e := range page.Entries {
			src := strings.TrimSpace(e.Source)
			if src == "" {
				continue
			}
			out[src] = CorpusInventoryRow{
				ContentSHA256:     strings.TrimSpace(e.ContentSHA256),
				ClientContentHash: strings.TrimSpace(e.ClientContentHash),
			}
		}
		if !page.HasMore || strings.TrimSpace(page.NextCursor) == "" {
			break
		}
		cursor = page.NextCursor
	}
	return out, nil
}

// IngestRequest is a single whole-file ingest sent as multipart/form-data per
// the gateway v0.2 contract: a "file" part holds the bytes, plus form fields
// for source (relative path) and content_hash.
type IngestRequest struct {
	Source      string // relative path; never absolute.
	ContentHash string // "sha256:<hex>".
	Project     string // optional X-Chimera-Project header.
	Flavor      string // optional X-Chimera-Flavor-Id header.
	Body        io.Reader
}

// IngestResponse is the gateway's ingest result (we read just enough to log
// progress and reconcile counts).
type IngestResponse struct {
	Object            string `json:"object"`
	TenantID          string `json:"tenant_id"`
	ProjectID         string `json:"project_id"`
	FlavorID          string `json:"flavor_id"`
	Source            string `json:"source"`
	ContentHash       string `json:"content_hash"`
	ContentSHA256     string `json:"content_sha256"`
	ClientContentHash string `json:"client_content_hash"`
	Chunks            int    `json:"chunks"`
	Collection        string `json:"collection"`
}

// Ingest sends one whole file to POST /v1/ingest. The caller is responsible
// for closing any io.ReadCloser they pass via req.Body.
func (c *GatewayClient) Ingest(ctx context.Context, req IngestRequest) (*IngestResponse, error) {
	if req.Source == "" || req.Body == nil {
		return nil, errors.New("ingest: source and body are required")
	}
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if err := mw.WriteField("source", req.Source); err != nil {
		return nil, err
	}
	if req.ContentHash != "" {
		if err := mw.WriteField("content_hash", req.ContentHash); err != nil {
			return nil, err
		}
	}
	fw, err := mw.CreateFormFile("file", filenameFromSource(req.Source))
	if err != nil {
		return nil, err
	}
	if _, err := io.Copy(fw, req.Body); err != nil {
		return nil, err
	}
	if err := mw.Close(); err != nil {
		return nil, err
	}

	headers := map[string]string{}
	if req.Project != "" {
		headers[naming.HeaderProjectTarget] = req.Project
	}
	if req.Flavor != "" {
		headers[naming.HeaderFlavorTarget] = req.Flavor
	}

	httpReq, err := c.newRequest(ctx, http.MethodPost, "/v1/ingest", mw.FormDataContentType(), &buf, headers)
	if err != nil {
		return nil, err
	}
	res, err := c.HTTP.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, classify(res, "/v1/ingest")
	}
	var out IngestResponse
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode ingest response: %w", err)
	}
	return &out, nil
}

type sessionStartResponse struct {
	Object        string `json:"object"`
	SessionID     string `json:"session_id"`
	MaxChunkBytes int64  `json:"max_chunk_bytes"`
	MaxTotalBytes int64  `json:"max_total_bytes"`
}

// SessionRetryPolicy configures bounded retries for each HTTP step in a chunked
// ingest session (start, each chunk PUT, complete). Zero values use the same
// defaults as whole-file ingest (see config defaults).
type SessionRetryPolicy struct {
	MaxAttempts int
	BaseDelay   time.Duration
	MaxDelay    time.Duration
}

func normalizeSessionRetryPolicy(p SessionRetryPolicy) SessionRetryPolicy {
	if p.MaxAttempts <= 0 {
		p.MaxAttempts = defaultRetryAttempts
	}
	if p.BaseDelay <= 0 {
		p.BaseDelay = defaultRetryBaseDelay
	}
	if p.MaxDelay <= 0 {
		p.MaxDelay = defaultRetryMaxDelay
	}
	return p
}

func sessionBackoff(ctx context.Context, attempt int, pol SessionRetryPolicy, rng *rand.Rand) error {
	d := Backoff(attempt, pol.BaseDelay, pol.MaxDelay, rng)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
	}
	return nil
}

// httpDoWithPolicy performs method+path with a fresh body reader per attempt,
// retrying transient failures up to pol.MaxAttempts.
func (c *GatewayClient) httpDoWithPolicy(ctx context.Context, method, path, contentType string, body []byte, hdrs map[string]string, pol SessionRetryPolicy, rng *rand.Rand) ([]byte, error) {
	pol = normalizeSessionRetryPolicy(pol)
	for attempt := 0; attempt < pol.MaxAttempts; attempt++ {
		req, err := c.newRequest(ctx, method, path, contentType, bytes.NewReader(body), hdrs)
		if err != nil {
			return nil, err
		}
		res, err := c.HTTP.Do(req)
		if err != nil {
			if !IsRetryable(err) || attempt == pol.MaxAttempts-1 {
				return nil, err
			}
			if err := sessionBackoff(ctx, attempt, pol, rng); err != nil {
				return nil, err
			}
			continue
		}
		respBody, rerr := internalhttp.ReadAndCloseLimited(res.Body, 8<<20)
		if rerr != nil {
			return nil, rerr
		}
		if res.StatusCode == http.StatusOK {
			return respBody, nil
		}
		he := &HTTPError{Path: path, Status: res.StatusCode, Body: string(respBody)}
		if !IsRetryable(he) || attempt == pol.MaxAttempts-1 {
			return nil, he
		}
		if err := sessionBackoff(ctx, attempt, pol, rng); err != nil {
			return nil, err
		}
	}
	return nil, fmt.Errorf("%s %s: exhausted retries", method, path)
}

// IngestChunked uploads a file via POST /v1/ingest/session + PUT .../chunk +
// POST .../complete (gateway v0.4). absPath is the local file to read.
// Each HTTP step retries transient errors according to pol (typically the
// same bounds as whole-file ingest).
func (c *GatewayClient) IngestChunked(ctx context.Context, absPath string, req IngestRequest, gw *IndexerConfig, pol SessionRetryPolicy) (*IngestResponse, error) {
	if gw == nil {
		return nil, errors.New("ingest chunked: gateway config is nil")
	}
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	startPath := strings.TrimSpace(gw.IngestSessionPath)
	if startPath == "" {
		startPath = "/v1/ingest/session"
	}
	if !strings.HasPrefix(startPath, "/") {
		startPath = "/" + startPath
	}
	hdrs := map[string]string{}
	if req.Project != "" {
		hdrs[naming.HeaderProjectTarget] = req.Project
	}
	if req.Flavor != "" {
		hdrs[naming.HeaderFlavorTarget] = req.Flavor
	}
	startPayload, err := json.Marshal(map[string]string{
		"source":       req.Source,
		"content_hash": req.ContentHash,
	})
	if err != nil {
		return nil, err
	}
	startBody, err := c.httpDoWithPolicy(ctx, http.MethodPost, startPath, "application/json", startPayload, hdrs, pol, rng)
	if err != nil {
		return nil, err
	}
	var sess sessionStartResponse
	if err := json.Unmarshal(startBody, &sess); err != nil {
		return nil, fmt.Errorf("decode ingest session start: %w", err)
	}
	if sess.SessionID == "" {
		return nil, fmt.Errorf("ingest session: missing session_id")
	}
	if sess.MaxChunkBytes <= 0 {
		return nil, fmt.Errorf("ingest session: invalid max_chunk_bytes")
	}

	f, err := os.Open(absPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	buf := make([]byte, sess.MaxChunkBytes)
	index := 0
	for {
		n, rerr := f.Read(buf)
		if n > 0 {
			chunk := append([]byte(nil), buf[:n]...)
			if err := c.putIngestChunk(ctx, sess.SessionID, index, chunk, hdrs, pol, rng); err != nil {
				return nil, err
			}
			index++
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return nil, rerr
		}
	}
	return c.completeIngestSession(ctx, sess.SessionID, hdrs, pol, rng)
}

func (c *GatewayClient) putIngestChunk(ctx context.Context, sessionID string, index int, data []byte, hdrs map[string]string, pol SessionRetryPolicy, rng *rand.Rand) error {
	path := "/v1/ingest/session/" + url.PathEscape(sessionID) + "/chunk"
	h := map[string]string{}
	for k, v := range hdrs {
		h[k] = v
	}
	h[naming.HeaderChunkIndexTarget] = strconv.Itoa(index)
	_, err := c.httpDoWithPolicy(ctx, http.MethodPut, path, "application/octet-stream", data, h, pol, rng)
	return err
}

func (c *GatewayClient) completeIngestSession(ctx context.Context, sessionID string, hdrs map[string]string, pol SessionRetryPolicy, rng *rand.Rand) (*IngestResponse, error) {
	path := "/v1/ingest/session/" + url.PathEscape(sessionID) + "/complete"
	b, err := c.httpDoWithPolicy(ctx, http.MethodPost, path, "application/json", []byte("{}"), hdrs, pol, rng)
	if err != nil {
		return nil, err
	}
	var out IngestResponse
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, fmt.Errorf("decode ingest session complete: %w", err)
	}
	return &out, nil
}

func (c *GatewayClient) newRequest(ctx context.Context, method, path, contentType string, body io.Reader, hdrs map[string]string) (*http.Request, error) {
	if c.BaseURL == "" {
		return nil, errors.New("gateway base URL is empty")
	}
	full, err := url.JoinPath(c.BaseURL, path)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, method, full, body)
	if err != nil {
		return nil, err
	}
	if c.Token != "" {
		req.Header.Set("Authorization", internalhttp.BearerAuthValue(c.Token))
	}
	if id := strings.TrimSpace(c.IndexRunID); requestid.Valid(id) {
		req.Header.Set(naming.HeaderIndexRunTarget, id)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	for k, v := range hdrs {
		req.Header.Set(k, v)
	}
	return req, nil
}

// HTTPError represents a non-2xx response. Retryable callers inspect Status.
type HTTPError struct {
	Path   string
	Status int
	Body   string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("%s: status %d: %s", e.Path, e.Status, truncate(e.Body, 200))
}

// IsRetryable reports whether the error is a transient HTTP error per the
// failure-handling section of docs/plans/indexer.plan.md (5xx, 408, 425, 429).
func IsRetryable(err error) bool {
	var he *HTTPError
	if !errors.As(err, &he) {
		return err != nil // network errors are retryable.
	}
	switch he.Status {
	case http.StatusRequestTimeout, http.StatusTooEarly, http.StatusTooManyRequests:
		return true
	}
	return he.Status >= 500
}

// IsFatal reports whether the error indicates the indexer must stop or
// require operator action (401/403). 4xx other than retryable codes are
// also returned as fatal so we don't loop forever on a bad request.
func IsFatal(err error) bool {
	var he *HTTPError
	if !errors.As(err, &he) {
		return false
	}
	if IsRetryable(err) {
		return false
	}
	return he.Status >= 400 && he.Status < 500
}

func classify(res *http.Response, path string) error {
	b, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
	return &HTTPError{Path: path, Status: res.StatusCode, Body: string(b)}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func filenameFromSource(source string) string {
	// Filename is metadata-only; the gateway's ingest handler uses the
	// "source" form field as the canonical relative path.
	if i := strings.LastIndex(source, "/"); i >= 0 {
		return source[i+1:]
	}
	return source
}
