package server

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/lynn/claudia-gateway/internal/platform/requestid"
	"github.com/lynn/claudia-gateway/internal/rag"
	"github.com/lynn/claudia-gateway/internal/vectorstore"
)

const ingestSessionTTL = 15 * time.Minute

// ingestSessionStore holds in-flight chunked uploads (gateway v0.4). Sessions
// expire after ingestSessionTTL without activity (checked on each operation).
type ingestSessionStore struct {
	mu       sync.Mutex
	sessions map[string]*ingestSession
}

type ingestSession struct {
	tenantID       string
	coords         vectorstore.Coords
	source         string
	clientHash     string
	indexRunID     string
	conversationID string
	buf            bytes.Buffer
	nextChunk      int
	maxTotal       int64
	maxChunk       int64
	createdAt      time.Time
}

func newIngestSessionStore() *ingestSessionStore {
	return &ingestSessionStore{sessions: map[string]*ingestSession{}}
}

func (st *ingestSessionStore) pruneLocked(now time.Time) {
	for id, s := range st.sessions {
		if now.Sub(s.createdAt) > ingestSessionTTL {
			delete(st.sessions, id)
		}
	}
}

func pickMaxChunkBytes(maxTotal int64) int64 {
	if maxTotal <= 0 {
		return 64 * 1024
	}
	maxChunk := maxTotal / 16
	if maxChunk < 64*1024 {
		maxChunk = 64 * 1024
	}
	if maxChunk > 1024*1024 {
		maxChunk = 1024 * 1024
	}
	if maxChunk > maxTotal {
		maxChunk = maxTotal
	}
	return maxChunk
}

func randomSessionID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// handleV1IngestSessionStart handles POST /v1/ingest/session (exact path).
func handleV1IngestSessionStart(w http.ResponseWriter, r *http.Request, rt *Runtime, log *slog.Logger) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	rt.Sync()
	res, tokStore, _ := rt.Snapshot()
	token := bearerToken(r.Header.Get("Authorization"))
	sess := tokStore.Validate(token)
	if token == "" || sess == nil {
		writeJSONError(w, http.StatusUnauthorized, "Unauthorized", "invalid_api_key")
		return
	}
	if !res.RAG.Enabled || rt.RAG() == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "RAG is not enabled", "gateway_config")
		return
	}
	var body struct {
		Source      string `json:"source"`
		ContentHash string `json:"content_hash"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON body", "invalid_request")
		return
	}
	source := strings.TrimSpace(body.Source)
	if source == "" {
		writeJSONError(w, http.StatusBadRequest, "missing source", "invalid_request")
		return
	}
	maxTotal := res.RAG.MaxIngestBytes
	maxChunk := pickMaxChunkBytes(maxTotal)
	id, err := randomSessionID()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "session id", "internal")
		return
	}
	indexRun := strings.TrimSpace(r.Header.Get(headerIndexRun))
	if indexRun != "" && !requestid.Valid(indexRun) {
		indexRun = ""
	}
	convID := optionalConversationIDFromHeader(r)
	rec := &ingestSession{
		tenantID:       sess.TenantID,
		source:         source,
		clientHash:     strings.TrimSpace(body.ContentHash),
		indexRunID:     indexRun,
		conversationID: convID,
		maxTotal:       maxTotal,
		maxChunk:       maxChunk,
		createdAt:      time.Now(),
		coords: vectorstore.Coords{
			TenantID:  sess.TenantID,
			ProjectID: resolveProject(r.Header.Get(headerProject), res.RAG.DefaultProject),
			FlavorID:  resolveFlavor(r.Header.Get(headerFlavor), res.RAG.DefaultFlavor),
		},
	}
	store := rt.ingestSessions
	store.mu.Lock()
	store.pruneLocked(time.Now())
	store.sessions[id] = rec
	store.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"object":             "ingest.session",
		"session_id":         id,
		"max_chunk_bytes":    maxChunk,
		"max_total_bytes":    maxTotal,
		"chunk_method":       "PUT",
		"chunk_path":         fmt.Sprintf("/v1/ingest/session/%s/chunk", id),
		"complete_method":    "POST",
		"complete_path":      fmt.Sprintf("/v1/ingest/session/%s/complete", id),
		"chunk_index_header": "X-Claudia-Chunk-Index",
	})
}

// handleV1IngestSessionTail handles /v1/ingest/session/{id}/chunk and .../complete.
func handleV1IngestSessionTail(w http.ResponseWriter, r *http.Request, rt *Runtime, log *slog.Logger) {
	p := strings.TrimPrefix(r.URL.Path, "/v1/ingest/session/")
	parts := strings.Split(p, "/")
	if len(parts) != 2 {
		http.NotFound(w, r)
		return
	}
	id, tail := parts[0], parts[1]
	switch tail {
	case "chunk":
		if r.Method != http.MethodPut {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		handleIngestSessionChunk(w, r, rt, log, id)
	case "complete":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		handleIngestSessionComplete(w, r, rt, log, id)
	default:
		http.NotFound(w, r)
	}
}

func handleIngestSessionChunk(w http.ResponseWriter, r *http.Request, rt *Runtime, log *slog.Logger, id string) {
	rt.Sync()
	res, tokStore, _ := rt.Snapshot()
	token := bearerToken(r.Header.Get("Authorization"))
	sess := tokStore.Validate(token)
	if token == "" || sess == nil {
		writeJSONError(w, http.StatusUnauthorized, "Unauthorized", "invalid_api_key")
		return
	}
	if !res.RAG.Enabled || rt.RAG() == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "RAG is not enabled", "gateway_config")
		return
	}
	idxStr := strings.TrimSpace(r.Header.Get("X-Claudia-Chunk-Index"))
	idx, err := strconv.Atoi(idxStr)
	if err != nil || idx < 0 {
		writeJSONError(w, http.StatusBadRequest, "invalid X-Claudia-Chunk-Index", "invalid_request")
		return
	}

	store := rt.ingestSessions
	store.mu.Lock()
	store.pruneLocked(time.Now())
	rec, ok := store.sessions[id]
	if !ok {
		store.mu.Unlock()
		writeJSONError(w, http.StatusNotFound, "unknown or expired session", "invalid_request")
		return
	}
	if rec.tenantID != sess.TenantID {
		store.mu.Unlock()
		writeJSONError(w, http.StatusForbidden, "session tenant mismatch", "invalid_request")
		return
	}
	if idx != rec.nextChunk {
		store.mu.Unlock()
		writeJSONError(w, http.StatusBadRequest,
			fmt.Sprintf("expected chunk index %d, got %d", rec.nextChunk, idx), "invalid_request")
		return
	}
	maxChunk := rec.maxChunk
	store.mu.Unlock()

	r.Body = http.MaxBytesReader(w, r.Body, maxChunk)
	chunk, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "chunk read error", "invalid_request")
		return
	}
	if len(chunk) == 0 {
		writeJSONError(w, http.StatusBadRequest, "empty chunk", "invalid_request")
		return
	}

	store.mu.Lock()
	rec, ok = store.sessions[id]
	if !ok || rec.tenantID != sess.TenantID {
		store.mu.Unlock()
		writeJSONError(w, http.StatusNotFound, "unknown or expired session", "invalid_request")
		return
	}
	if int64(rec.buf.Len()+len(chunk)) > rec.maxTotal {
		delete(store.sessions, id)
		store.mu.Unlock()
		writeJSONError(w, http.StatusRequestEntityTooLarge,
			fmt.Sprintf("session exceeds max_total_bytes=%d", rec.maxTotal), "request_too_large")
		return
	}
	rec.buf.Write(chunk)
	rec.nextChunk++
	rec.createdAt = time.Now()
	received := rec.nextChunk
	bufLen := rec.buf.Len()
	store.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"object":          "ingest.session.chunk",
		"session_id":      id,
		"received_chunks": received,
		"bytes_buffered":  bufLen,
	})
}

func handleIngestSessionComplete(w http.ResponseWriter, r *http.Request, rt *Runtime, log *slog.Logger, id string) {
	rt.Sync()
	res, tokStore, _ := rt.Snapshot()
	token := bearerToken(r.Header.Get("Authorization"))
	sess := tokStore.Validate(token)
	if token == "" || sess == nil {
		writeJSONError(w, http.StatusUnauthorized, "Unauthorized", "invalid_api_key")
		return
	}
	if !res.RAG.Enabled || rt.RAG() == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "RAG is not enabled", "gateway_config")
		return
	}

	store := rt.ingestSessions
	store.mu.Lock()
	store.pruneLocked(time.Now())
	rec, ok := store.sessions[id]
	if !ok {
		store.mu.Unlock()
		writeJSONError(w, http.StatusNotFound, "unknown or expired session", "invalid_request")
		return
	}
	if rec.tenantID != sess.TenantID {
		store.mu.Unlock()
		writeJSONError(w, http.StatusForbidden, "session tenant mismatch", "invalid_request")
		return
	}
	if rec.buf.Len() == 0 {
		delete(store.sessions, id)
		store.mu.Unlock()
		writeJSONError(w, http.StatusBadRequest, "no chunks uploaded", "invalid_request")
		return
	}
	text := rec.buf.String()
	source := rec.source
	clientHash := rec.clientHash
	coords := rec.coords
	indexRunID := rec.indexRunID
	convID := rec.conversationID
	delete(store.sessions, id)
	store.mu.Unlock()

	rid := requestid.FromContext(r.Context())
	result, err := rt.RAG().Ingest(r.Context(), rag.IngestRequest{
		Coords:         coords,
		Source:         source,
		Text:           text,
		ContentHash:    clientHash,
		RequestID:      rid,
		IndexRunID:     indexRunID,
		ConversationID: convID,
	})
	if err != nil {
		if log != nil {
			args := []any{
				"msg", "ingest.chunked.error",
				"tenant", sess.TenantID, "source", source, "err", err,
				"service", "gateway", "principal_id", sess.TenantID,
				"timeline_kind", "indexer",
			}
			if rid != "" {
				args = append(args, "request_id", rid)
			}
			if indexRunID != "" {
				args = append(args, "index_run_id", indexRunID)
			}
			if convID != "" {
				args = append(args, "conversation_id", convID)
			}
			log.Error("chunked ingest failed", args...)
		}
		writeJSONError(w, http.StatusBadGateway, err.Error(), "gateway_upstream")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	out := map[string]any{
		"object":         "ingest.result",
		"source":         result.Source,
		"chunks":         result.Chunks,
		"collection":     result.Collection,
		"tenant_id":      coords.TenantID,
		"project_id":     coords.ProjectID,
		"flavor_id":      coords.FlavorID,
		"content_hash":   result.ContentHash,
		"content_sha256": result.ContentSHA256,
	}
	if result.ClientContentHash != "" {
		out["client_content_hash"] = result.ClientContentHash
	}
	if log != nil {
		args := []any{
			"msg", "ingest.complete",
			"tenant", sess.TenantID, "source", source, "chunks", result.Chunks,
			"service", "gateway", "principal_id", sess.TenantID,
			"timeline_kind", "indexer",
		}
		if rid != "" {
			args = append(args, "request_id", rid)
		}
		if indexRunID != "" {
			args = append(args, "index_run_id", indexRunID)
		}
		if convID != "" {
			args = append(args, "conversation_id", convID)
		}
		log.Info("ingest complete", args...)
	}
	_ = json.NewEncoder(w).Encode(out)
}
