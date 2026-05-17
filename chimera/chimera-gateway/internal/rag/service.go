// Package rag wires together the chunker, embedding client, and vector store
// for gateway v0.2 ingest + retrieval. The Service is created once per process
// (after RAG is verified enabled in gateway.yaml) and shared across handlers.
package rag

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/rag/chunk"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/rag/embed"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/vectorstore"
	"github.com/lynn/porcelain/internal/platform"
)

// queryPreviewMax bounds the query/text excerpt included in DEBUG logs so we
// don't echo entire prompts into the log stream.
const queryPreviewMax = 160

// conversationRAGSpanWindowMS is the default tier-4b UI join window (see log-conversations.md Phase 1).
const conversationRAGSpanWindowMS = 10000

func newSpanID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Service orchestrates ingest + retrieval against a single vector store +
// embedding client.
type Service struct {
	store        vectorstore.Store
	embedder     Embedder
	chunkSize    int
	chunkOverlap int
	topK         int
	scoreFloor   float32
	embedDim     int
	log          *slog.Logger
}

// Embedder is the embedding client contract (satisfied by *embed.Client).
type Embedder interface {
	EmbedBatch(ctx context.Context, inputs []string) ([][]float32, error)
	EmbedOne(ctx context.Context, s string) ([]float32, error)
	Model() string
}

// Options configures Service behavior.
type Options struct {
	Store          vectorstore.Store
	Embedder       Embedder
	ChunkSize      int
	ChunkOverlap   int
	TopK           int
	ScoreThreshold float32
	EmbeddingDim   int
	Log            *slog.Logger
}

// New constructs a Service. All store/embedder fields are required.
func New(o Options) (*Service, error) {
	if o.Store == nil {
		return nil, errors.New("rag: nil store")
	}
	if o.Embedder == nil {
		return nil, errors.New("rag: nil embedder")
	}
	if o.ChunkSize <= 0 {
		o.ChunkSize = 512
	}
	if o.ChunkOverlap < 0 || o.ChunkOverlap >= o.ChunkSize {
		o.ChunkOverlap = 128
	}
	if o.TopK <= 0 {
		o.TopK = 8
	}
	if o.EmbeddingDim <= 0 {
		o.EmbeddingDim = 1536
	}
	return &Service{
		store:        o.Store,
		embedder:     o.Embedder,
		chunkSize:    o.ChunkSize,
		chunkOverlap: o.ChunkOverlap,
		topK:         o.TopK,
		scoreFloor:   o.ScoreThreshold,
		embedDim:     o.EmbeddingDim,
		log:          o.Log,
	}, nil
}

// IngestRequest is one document to ingest.
type IngestRequest struct {
	Coords      vectorstore.Coords
	Source      string // relative path or document key
	Text        string
	ContentHash string // optional, client-supplied
	// Optional HTTP correlation (gateway sets these from inbound requests).
	RequestID      string
	ConversationID string
	IndexRunID     string
}

// IngestResult summarizes what was written.
type IngestResult struct {
	Collection        string
	Source            string
	Chunks            int
	ContentHash       string // canonical server-side digest (same as ContentSHA256)
	ContentSHA256     string // SHA-256 over UTF-8 bytes of ingested text (authoritative for v0.4+)
	ClientContentHash string // optional echo of client-supplied hash (diagnostics only)
}

// Ingest chunks → embeds → upserts the document. It ensures the collection
// exists. When ContentHash is empty, a SHA-256 of UTF-8 bytes is computed.
func (s *Service) Ingest(ctx context.Context, req IngestRequest) (IngestResult, error) {
	res := IngestResult{Source: strings.TrimSpace(req.Source)}
	if res.Source == "" {
		return res, errors.New("ingest: empty source")
	}
	if strings.TrimSpace(req.Text) == "" {
		return res, errors.New("ingest: empty text")
	}
	if req.Coords.TenantID == "" {
		return res, errors.New("ingest: empty tenant_id")
	}

	collection := vectorstore.CollectionName(req.Coords)
	res.Collection = collection
	if err := s.store.EnsureCollection(ctx, collection, s.embedDim); err != nil {
		return res, fmt.Errorf("ensure collection %s: %w", collection, err)
	}

	chunks := chunk.Split(req.Text, s.chunkSize, s.chunkOverlap)
	if len(chunks) == 0 {
		return res, errors.New("ingest: no chunks produced")
	}

	inputs := make([]string, 0, len(chunks))
	for _, c := range chunks {
		inputs = append(inputs, c.Text)
	}
	vectors, err := s.embedder.EmbedBatch(ctx, inputs)
	if err != nil {
		return res, fmt.Errorf("embed: %w", err)
	}
	if len(vectors) != len(chunks) {
		return res, fmt.Errorf("embed returned %d vectors for %d chunks", len(vectors), len(chunks))
	}
	for i, v := range vectors {
		if len(v) != s.embedDim {
			return res, fmt.Errorf("embed dim mismatch at chunk %d: got %d, expect %d", i, len(v), s.embedDim)
		}
	}

	// Re-ingest is upsert: delete old points for this source first, then
	// upsert. Errors from delete on a fresh collection are tolerated.
	if err := s.store.DeleteBySource(ctx, collection, res.Source); err != nil && s.log != nil {
		args := []any{"msg", "rag.ingest.delete_pre_failed", "source", res.Source, "err", err}
		args = appendGatewayCorrelation(args, req.RequestID, req.ConversationID, req.IndexRunID, req.Coords.TenantID)
		s.log.Debug("delete-by-source pre-ingest failed (likely empty collection)", args...)
	}

	sum := sha256.Sum256([]byte(req.Text))
	serverHash := "sha256:" + hex.EncodeToString(sum[:])
	clientHash := strings.TrimSpace(req.ContentHash)

	now := time.Now().Unix()
	pts := make([]vectorstore.Point, 0, len(chunks))
	for i, c := range chunks {
		pts = append(pts, vectorstore.Point{
			ID:     vectorstore.PointID(req.Coords, res.Source, i),
			Vector: vectors[i],
			Payload: vectorstore.Payload{
				TenantID:          req.Coords.TenantID,
				ProjectID:         req.Coords.ProjectID,
				FlavorID:          req.Coords.FlavorID,
				Text:              c.Text,
				Source:            res.Source,
				CreatedAt:         now,
				ContentSHA256:     serverHash,
				ClientContentHash: clientHash,
			},
		})
	}
	if err := s.store.Upsert(ctx, collection, pts); err != nil {
		return res, fmt.Errorf("upsert: %w", err)
	}
	res.Chunks = len(chunks)
	res.ClientContentHash = clientHash
	res.ContentSHA256 = serverHash
	res.ContentHash = serverHash
	if s.log != nil {
		args := []any{
			"msg", "rag.ingest.trace",
			"tenant", req.Coords.TenantID,
			"project", req.Coords.ProjectID,
			"flavor", req.Coords.FlavorID,
			"source", res.Source,
			"chunks", res.Chunks,
			"collection", collection,
			"content_hash", res.ContentHash,
			"embed_dim", s.embedDim,
			"embed_model", s.embedder.Model(),
			"text_bytes", len(req.Text),
		}
		args = appendGatewayCorrelation(args, req.RequestID, req.ConversationID, req.IndexRunID, req.Coords.TenantID)
		s.log.Log(ctx, platform.LevelTrace, "rag ingest", args...)
	}
	return res, nil
}

// RetrieveRequest fetches top-k chunks for a query string.
type RetrieveRequest struct {
	Coords vectorstore.Coords
	Query  string
	TopK   int // <= 0 uses Service default
	// Optional correlation from the gateway chat handler.
	RequestID      string
	ConversationID string
	// TurnIndex is the 1-based chat turn for lifecycle logs (default 1 when unset).
	TurnIndex int
	// LifecycleLog receives conversation.rag.span before outbound embedding / search when non-nil.
	LifecycleLog *slog.Logger
}

// Retrieve embeds the query then runs a top-k search filtered by coords.
func (s *Service) Retrieve(ctx context.Context, req RetrieveRequest) ([]vectorstore.Hit, error) {
	if strings.TrimSpace(req.Query) == "" {
		return nil, nil
	}
	if req.Coords.TenantID == "" {
		return nil, errors.New("retrieve: empty tenant_id")
	}
	k := req.TopK
	if k <= 0 {
		k = s.topK
	}
	collection := vectorstore.CollectionName(req.Coords)
	ti := req.TurnIndex
	if ti <= 0 {
		ti = 1
	}
	spanLog := req.LifecycleLog
	if spanLog == nil {
		spanLog = s.log
	}
	if spanLog != nil {
		args := []any{
			"msg", "conversation.rag.span",
			"collection", collection,
			"span_id", newSpanID(),
			"window_ms", conversationRAGSpanWindowMS,
			"turn_index", ti,
			"timeline_kind", "chimera-vectorstore",
		}
		if req.LifecycleLog == nil {
			args = appendGatewayCorrelation(args, req.RequestID, req.ConversationID, "", req.Coords.TenantID)
		}
		spanLog.Info("conversation RAG span", args...)
	}
	if s.log != nil {
		args := []any{
			"msg", "rag.query",
			"tenant", req.Coords.TenantID,
			"project", req.Coords.ProjectID,
			"flavor", req.Coords.FlavorID,
			"collection", collection,
			"top_k", k,
			"score_threshold", s.scoreFloor,
			"query_bytes", len(req.Query),
			"query", previewText(req.Query),
		}
		args = appendGatewayCorrelation(args, req.RequestID, req.ConversationID, "", req.Coords.TenantID)
		args = append(args, "timeline_kind", "chimera-vectorstore")
		s.log.Debug("rag search query", args...)
	}
	embedStart := time.Now()
	vec, err := s.embedder.EmbedOne(ctx, req.Query)
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}
	if s.log != nil {
		args := []any{
			"msg", "rag.embed",
			"tenant", req.Coords.TenantID,
			"project", req.Coords.ProjectID,
			"flavor", req.Coords.FlavorID,
			"collection", collection,
			"embed_dim", len(vec),
			"embed_model", s.embedder.Model(),
			"elapsed_ms", time.Since(embedStart).Milliseconds(),
		}
		args = appendGatewayCorrelation(args, req.RequestID, req.ConversationID, "", req.Coords.TenantID)
		args = append(args, "timeline_kind", "chimera-vectorstore")
		s.log.Debug("rag embedding retrieved", args...)
	}
	hits, err := s.store.Search(ctx, collection, vec, k, s.scoreFloor, &req.Coords)
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}
	if s.log != nil {
		for i, h := range hits {
			args := []any{
				"msg", "rag.hit",
				"tenant", req.Coords.TenantID,
				"project", req.Coords.ProjectID,
				"flavor", req.Coords.FlavorID,
				"collection", collection,
				"rank", i + 1,
				"hits", len(hits),
				"top_k", k,
				"score", h.Score,
				"score_threshold", s.scoreFloor,
				"point_id", h.ID,
				"source", h.Payload.Source,
				"text", previewText(h.Payload.Text),
			}
			args = appendGatewayCorrelation(args, req.RequestID, req.ConversationID, "", req.Coords.TenantID)
			args = append(args, "timeline_kind", "chimera-vectorstore")
			s.log.Debug("rag comparison", args...)
		}
	}
	return hits, nil
}

func appendGatewayCorrelation(args []any, requestID, conversationID, indexRunID, principalID string) []any {
	if requestID != "" {
		args = append(args, "request_id", requestID)
	}
	if conversationID != "" {
		args = append(args, "conversation_id", conversationID)
	}
	if principalID != "" {
		args = append(args, "principal_id", principalID)
	}
	if indexRunID != "" {
		args = append(args, "index_run_id", indexRunID)
	}
	args = append(args, "service", "gateway")
	return args
}

// previewText returns a single-line, length-bounded excerpt of s suitable for
// inclusion in DEBUG/TRACE log records.
func previewText(s string) string {
	t := strings.TrimSpace(s)
	if t == "" {
		return ""
	}
	t = strings.ReplaceAll(t, "\r", " ")
	t = strings.ReplaceAll(t, "\n", " ")
	if len(t) > queryPreviewMax {
		t = t[:queryPreviewMax] + "…"
	}
	return t
}

// EmbedDim is the configured embedding dimension (used by /v1/indexer/config).
func (s *Service) EmbedDim() int { return s.embedDim }

// ChunkSize / ChunkOverlap accessors for /v1/indexer/config.
func (s *Service) ChunkSize() int    { return s.chunkSize }
func (s *Service) ChunkOverlap() int { return s.chunkOverlap }
func (s *Service) TopK() int         { return s.topK }

// StoreHealth is exposed for /v1/indexer/storage/health.
func (s *Service) StoreHealth(ctx context.Context) error { return s.store.Health(ctx) }

// StoreStats is exposed for /v1/indexer/storage/stats.
func (s *Service) StoreStats(ctx context.Context, c vectorstore.Coords) (vectorstore.Stats, error) {
	collection := vectorstore.CollectionName(c)
	return s.store.Stats(ctx, collection)
}

// CorpusInventoryEntry is one deduplicated source row for GET /v1/indexer/corpus/inventory.
type CorpusInventoryEntry struct {
	Source            string `json:"source"`
	ContentSHA256     string `json:"content_sha256"`
	ClientContentHash string `json:"client_content_hash,omitempty"`
}

// CorpusInventory returns unique sources from one scroll page of the corpus
// (deduped within the page). nextCursor is empty when the store reports no more
// points.
func (s *Service) CorpusInventory(ctx context.Context, c vectorstore.Coords, limit int, cursor string) ([]CorpusInventoryEntry, string, error) {
	collection := vectorstore.CollectionName(c)
	batch, err := s.store.ScrollPoints(ctx, collection, &c, limit, cursor)
	if err != nil {
		return nil, "", err
	}
	seen := map[string]struct{}{}
	out := make([]CorpusInventoryEntry, 0, len(batch.Points))
	for _, p := range batch.Points {
		src := strings.TrimSpace(p.Payload.Source)
		if src == "" {
			continue
		}
		if _, ok := seen[src]; ok {
			continue
		}
		seen[src] = struct{}{}
		out = append(out, CorpusInventoryEntry{
			Source:            src,
			ContentSHA256:     strings.TrimSpace(p.Payload.ContentSHA256),
			ClientContentHash: strings.TrimSpace(p.Payload.ClientContentHash),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Source < out[j].Source })
	return out, batch.NextCursor, nil
}

// EmbeddingModel returns the configured embedding model id.
func (s *Service) EmbeddingModel() string { return s.embedder.Model() }

// Compile-time guard so embed.Client is a valid Embedder.
var _ Embedder = (*embed.Client)(nil)
