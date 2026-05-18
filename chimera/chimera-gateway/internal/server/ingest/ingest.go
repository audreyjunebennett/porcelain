package ingest

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/gwhttp"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/rag"
	gruntime "github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/runtime"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/vectorstore"
	"github.com/lynn/porcelain/chimera/internal/platform/requestid"
)

// HandleV1 implements POST /v1/ingest (gateway v0.2). One document per
// request: either multipart/form-data with a "file" part (and optional
// "source", "content_hash" form fields) or JSON {"text", "source",
// "content_hash"}. Tenant is derived from the bearer token; project/flavor
// from headers (with token / config defaults).
func HandleV1(w http.ResponseWriter, r *http.Request, rt *gruntime.Runtime, log *slog.Logger) {
	rt.Sync()
	res, tokStore, _ := rt.Snapshot()
	token := gwhttp.BearerToken(r.Header.Get("Authorization"))
	sess := tokStore.Validate(token)
	if token == "" || sess == nil {
		gwhttp.WriteJSONError(w, http.StatusUnauthorized, "Unauthorized", "invalid_api_key")
		return
	}
	if !res.RAG.Enabled || rt.RAG() == nil {
		gwhttp.WriteJSONError(w, http.StatusServiceUnavailable, "RAG is not enabled", "gateway_config")
		return
	}
	if r.ContentLength > res.RAG.MaxIngestBytes && res.RAG.MaxIngestBytes > 0 {
		gwhttp.WriteJSONError(w, http.StatusRequestEntityTooLarge,
			fmt.Sprintf("body exceeds rag.ingest.max_bytes=%d", res.RAG.MaxIngestBytes), "request_too_large")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, res.RAG.MaxIngestBytes)

	source, text, contentHash, err := readIngestBody(r)
	if err != nil {
		gwhttp.WriteJSONError(w, http.StatusBadRequest, err.Error(), "invalid_request")
		return
	}
	if strings.TrimSpace(text) == "" {
		gwhttp.WriteJSONError(w, http.StatusBadRequest, "empty document text", "invalid_request")
		return
	}
	if strings.TrimSpace(source) == "" {
		gwhttp.WriteJSONError(w, http.StatusBadRequest, "missing source", "invalid_request")
		return
	}

	coords := vectorstore.Coords{
		TenantID:  sess.TenantID,
		ProjectID: ResolveProject(r.Header.Get(HeaderProject), res.RAG.DefaultProject),
		FlavorID:  ResolveFlavor(r.Header.Get(HeaderFlavor), res.RAG.DefaultFlavor),
	}

	indexRun := strings.TrimSpace(r.Header.Get(HeaderIndexRun))
	if indexRun != "" && !requestid.Valid(indexRun) {
		indexRun = ""
	}

	convID := OptionalConversationIDFromHeader(r)

	rid := requestid.FromContext(r.Context())
	result, err := rt.RAG().Ingest(r.Context(), rag.IngestRequest{
		Coords:         coords,
		Source:         source,
		Text:           text,
		ContentHash:    contentHash,
		RequestID:      rid,
		IndexRunID:     indexRun,
		ConversationID: convID,
	})
	if err != nil {
		if log != nil {
			args := []any{"msg", "ingest.failed", "tenant", sess.TenantID, "source", source, "err", err, "service", "gateway", "principal_id", sess.TenantID, "timeline_kind", "indexer"}
			if rid != "" {
				args = append(args, "request_id", rid)
			}
			if indexRun != "" {
				args = append(args, "index_run_id", indexRun)
			}
			if convID != "" {
				args = append(args, "conversation_id", convID)
			}
			log.Error("ingest failed", args...)
		}
		gwhttp.WriteJSONError(w, http.StatusBadGateway, err.Error(), "gateway_upstream")
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
		if indexRun != "" {
			args = append(args, "index_run_id", indexRun)
		}
		if convID != "" {
			args = append(args, "conversation_id", convID)
		}
		log.Info("ingest complete", args...)
	}
	if rec := rt.Metrics(); rec != nil {
		if rag := rt.RAG(); rag != nil {
			mid := strings.TrimSpace(rag.EmbeddingModel())
			if mid != "" {
				est := len(text) / 4
				if est < 1 {
					est = 1
				}
				if est > 2_000_000 {
					est = 2_000_000
				}
				rec.RecordBrokerResponse(time.Now().UTC(), mid, 200, est)
			}
		}
	}
	_ = json.NewEncoder(w).Encode(out)
}

// readIngestBody returns (source, text, contentHash, err) regardless of input
// shape (multipart or JSON).
func readIngestBody(r *http.Request) (string, string, string, error) {
	ct, params, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	switch {
	case strings.HasPrefix(ct, "multipart/"):
		return readMultipartIngest(r, params)
	case ct == "application/json":
		var doc struct {
			Text        string `json:"text"`
			Source      string `json:"source"`
			ContentHash string `json:"content_hash"`
		}
		dec := json.NewDecoder(r.Body)
		if err := dec.Decode(&doc); err != nil {
			return "", "", "", fmt.Errorf("invalid JSON body: %w", err)
		}
		return strings.TrimSpace(doc.Source), doc.Text, strings.TrimSpace(doc.ContentHash), nil
	default:
		return "", "", "", errors.New("unsupported Content-Type; use application/json or multipart/form-data")
	}
}

func readMultipartIngest(r *http.Request, params map[string]string) (string, string, string, error) {
	mr := multipart.NewReader(r.Body, params["boundary"])
	var (
		source, contentHash string
		textBuf             strings.Builder
		gotFile             bool
	)
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", "", "", fmt.Errorf("multipart: %w", err)
		}
		name := part.FormName()
		switch name {
		case "file":
			if source == "" {
				source = part.FileName()
			}
			body, err := io.ReadAll(part)
			if err != nil {
				_ = part.Close()
				return "", "", "", fmt.Errorf("read file part: %w", err)
			}
			textBuf.Write(body)
			gotFile = true
		case "text":
			body, err := io.ReadAll(part)
			if err != nil {
				_ = part.Close()
				return "", "", "", fmt.Errorf("read text part: %w", err)
			}
			textBuf.Write(body)
		case "source":
			body, _ := io.ReadAll(part)
			source = strings.TrimSpace(string(body))
		case "content_hash":
			body, _ := io.ReadAll(part)
			contentHash = strings.TrimSpace(string(body))
		}
		_ = part.Close()
	}
	if !gotFile && textBuf.Len() == 0 {
		return "", "", "", errors.New("multipart body must include a 'file' or 'text' part")
	}
	return strings.TrimSpace(source), textBuf.String(), contentHash, nil
}
