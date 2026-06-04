package rag

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/operatorstore"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/vectorstore"
	"github.com/lynn/porcelain/chimera/internal/config"
)

// ExpansionService provides manifest segment expansion APIs.
type ExpansionService struct {
	rag        *Service
	op         *operatorstore.Store
	stale      StaleSourceStore
	coherence  string
	toolingOn  bool
	cacheTTL   time.Duration
	cacheMax   int
	mu         sync.Mutex
	contextLRU map[string]contextCacheEntry
}

type contextCacheEntry struct {
	text string
	at   time.Time
}

// StaleSourceStore reports whether an indexed digest is stale on disk.
type StaleSourceStore interface {
	IsStale(tenantID, projectID, flavorID, source, indexedSHA string) bool
}

// ExpansionOptions configures ExpansionService.
type ExpansionOptions struct {
	RAG           *Service
	OperatorStore *operatorstore.Store
	StaleStore    StaleSourceStore
	Resolved      *config.Resolved
}

// NewExpansionService constructs an expansion helper.
func NewExpansionService(o ExpansionOptions) *ExpansionService {
	mode := "warn"
	ttl := 300 * time.Second
	max := 256
	tooling := true
	if o.Resolved != nil {
		mode = normalizeCoherenceMode(o.Resolved.RAG.CoherenceMode)
		tooling = o.Resolved.RAG.ToolingEnabled
		if o.Resolved.RAG.ExpansionCacheTTLSeconds > 0 {
			ttl = time.Duration(o.Resolved.RAG.ExpansionCacheTTLSeconds) * time.Second
		}
		if o.Resolved.RAG.ExpansionCacheMaxEntries > 0 {
			max = o.Resolved.RAG.ExpansionCacheMaxEntries
		}
	}
	return &ExpansionService{
		rag:        o.RAG,
		op:         o.OperatorStore,
		stale:      o.StaleStore,
		coherence:  mode,
		toolingOn:  tooling,
		cacheTTL:   ttl,
		cacheMax:   max,
		contextLRU: map[string]contextCacheEntry{},
	}
}

func (e *ExpansionService) enabled() error {
	if e == nil || e.rag == nil {
		return fmt.Errorf("rag service unavailable")
	}
	if !e.toolingOn {
		return fmt.Errorf("rag tooling disabled")
	}
	if e.op == nil {
		return fmt.Errorf("operator store unavailable")
	}
	return nil
}

func (e *ExpansionService) checkStale(coords vectorstore.Coords, source, contentSHA string) error {
	if e == nil || e.coherence != "strict" || e.stale == nil {
		return nil
	}
	if e.stale.IsStale(coords.TenantID, coords.ProjectID, coords.FlavorID, source, contentSHA) {
		return fmt.Errorf("indexed content is stale for %q; re-index workspace before expansion", source)
	}
	return nil
}

// ListSegments returns corpus segment metadata for a file version.
func (e *ExpansionService) ListSegments(ctx context.Context, coords vectorstore.Coords, source, contentSHA256 string) ([]operatorstore.CorpusSegmentRow, error) {
	if err := e.enabled(); err != nil {
		return nil, err
	}
	if err := e.checkStale(coords, source, contentSHA256); err != nil {
		return nil, err
	}
	return e.op.ListCorpusSegments(ctx, coords.TenantID, coords.ProjectID, coords.FlavorID, source, contentSHA256)
}

// AdjacentChunks returns neighbor segments and their Qdrant text payloads.
func (e *ExpansionService) AdjacentChunks(ctx context.Context, coords vectorstore.Coords, pointID string, radius int) ([]SegmentHit, error) {
	if err := e.enabled(); err != nil {
		return nil, err
	}
	row, err := e.op.CorpusSegmentByPointID(ctx, coords.TenantID, coords.ProjectID, coords.FlavorID, pointID)
	if err != nil {
		return nil, err
	}
	if err := e.checkStale(coords, row.Source, row.ContentSHA256); err != nil {
		return nil, err
	}
	rows, err := e.op.ListCorpusSegmentsAdjacent(ctx, coords.TenantID, coords.ProjectID, coords.FlavorID,
		row.Source, row.ContentSHA256, row.ChunkIndex, radius)
	if err != nil {
		return nil, err
	}
	return e.attachTexts(ctx, coords, rows)
}

// SegmentHit is a segment row plus chunk text from Qdrant.
type SegmentHit struct {
	Segment operatorstore.CorpusSegmentRow `json:"segment"`
	Text    string                         `json:"text"`
}

// ContextAround merges segment texts covering a line window.
func (e *ExpansionService) ContextAround(ctx context.Context, coords vectorstore.Coords, source, contentSHA256 string, line, before, after int) (string, error) {
	if err := e.enabled(); err != nil {
		return "", err
	}
	if line < 1 {
		line = 1
	}
	if before < 0 {
		before = 0
	}
	if after < 0 {
		after = 0
	}
	if err := e.checkStale(coords, source, contentSHA256); err != nil {
		return "", err
	}
	cacheKey := fmt.Sprintf("%s\x00%s\x00%s\x00%s\x00%d\x00%d\x00%d",
		coords.TenantID, coords.ProjectID, coords.FlavorID, source, line, before, after)
	if txt, ok := e.cacheGet(cacheKey); ok {
		return txt, nil
	}
	rows, err := e.op.ListCorpusSegments(ctx, coords.TenantID, coords.ProjectID, coords.FlavorID, source, contentSHA256)
	if err != nil {
		return "", err
	}
	lo := line - before
	hi := line + after
	var picked []operatorstore.CorpusSegmentRow
	for _, r := range rows {
		if r.EndLine >= lo && r.StartLine <= hi {
			picked = append(picked, r)
		}
	}
	hits, err := e.attachTexts(ctx, coords, picked)
	if err != nil {
		return "", err
	}
	var sb strings.Builder
	for i, h := range hits {
		if i > 0 {
			sb.WriteString("\n\n")
		}
		txt := strings.TrimSpace(h.Text)
		if h.Segment.StartsMidLine && txt != "" {
			if idx := strings.IndexByte(txt, '\n'); idx >= 0 {
				txt = "…" + txt[:idx] + txt[idx:]
			} else {
				txt = "…" + txt
			}
		}
		sb.WriteString(txt)
	}
	out := strings.TrimSpace(sb.String())
	e.cachePut(cacheKey, out)
	return out, nil
}

func (e *ExpansionService) attachTexts(ctx context.Context, coords vectorstore.Coords, rows []operatorstore.CorpusSegmentRow) ([]SegmentHit, error) {
	if len(rows) == 0 {
		return nil, nil
	}
	coll := vectorstore.CollectionName(coords)
	ids := make([]string, len(rows))
	for i, r := range rows {
		ids[i] = r.VectorPointID
	}
	pts, err := e.rag.GetPointPayloads(ctx, coll, ids)
	if err != nil {
		return nil, err
	}
	byID := map[string]string{}
	for _, p := range pts {
		byID[p.ID] = p.Payload.Text
	}
	out := make([]SegmentHit, 0, len(rows))
	for _, r := range rows {
		out = append(out, SegmentHit{Segment: r, Text: byID[r.VectorPointID]})
	}
	return out, nil
}

func (e *ExpansionService) cacheGet(key string) (string, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	ent, ok := e.contextLRU[key]
	if !ok || time.Since(ent.at) > e.cacheTTL {
		delete(e.contextLRU, key)
		return "", false
	}
	return ent.text, true
}

func (e *ExpansionService) cachePut(key, text string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(e.contextLRU) >= e.cacheMax {
		for k := range e.contextLRU {
			delete(e.contextLRU, k)
			break
		}
	}
	e.contextLRU[key] = contextCacheEntry{text: text, at: time.Now()}
}

func normalizeCoherenceMode(m string) string {
	switch strings.ToLower(strings.TrimSpace(m)) {
	case "off", "strict":
		return strings.ToLower(strings.TrimSpace(m))
	default:
		return "warn"
	}
}

// WorkspaceToolDefinitions documents gateway expansion tools (Phase 7).
func WorkspaceToolDefinitions() []map[string]any {
	return []map[string]any{
		{
			"name":        "workspace_context_around",
			"description": "Return merged indexed text around a line in a workspace file.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"source":       map[string]any{"type": "string"},
					"line":         map[string]any{"type": "integer"},
					"before_lines": map[string]any{"type": "integer"},
					"after_lines":  map[string]any{"type": "integer"},
					"content_sha256": map[string]any{"type": "string"},
				},
				"required": []string{"source", "line"},
			},
		},
		{
			"name":        "workspace_adjacent_chunks",
			"description": "Return neighboring manifest chunks for a vector point id.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"point_id": map[string]any{"type": "string"},
					"radius":   map[string]any{"type": "integer"},
				},
				"required": []string{"point_id"},
			},
		},
		{
			"name":        "workspace_read_lines",
			"description": "Alias for workspace_context_around using start_line/end_line.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"source":          map[string]any{"type": "string"},
					"start_line":      map[string]any{"type": "integer"},
					"end_line":        map[string]any{"type": "integer"},
					"content_sha256": map[string]any{"type": "string"},
				},
				"required": []string{"source", "start_line", "end_line"},
			},
		},
	}
}
