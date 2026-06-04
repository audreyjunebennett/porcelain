package ingest

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/gwhttp"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/rag"
	gruntime "github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/runtime"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/vectorstore"
	"github.com/lynn/porcelain/chimera/internal/platform/requestid"
)

// HandleV1 implements POST /v1/ingest — manifest-only (application/json ingest.manifest).
func HandleV1(w http.ResponseWriter, r *http.Request, rt *gruntime.Runtime, log *slog.Logger) {
	rt.Sync()
	res, tokStore := rt.Snapshot()
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

	ct := strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Type")))
	if !strings.HasPrefix(ct, "application/json") {
		gwhttp.WriteJSONError(w, http.StatusBadRequest,
			"manifest ingest requires application/json body (ingest.manifest)", "invalid_request")
		return
	}
	var manifest rag.IngestManifest
	if err := json.NewDecoder(r.Body).Decode(&manifest); err != nil {
		gwhttp.WriteJSONError(w, http.StatusBadRequest, fmt.Sprintf("invalid manifest JSON: %v", err), "invalid_request")
		return
	}
	if err := rag.ValidateManifest(&manifest); err != nil {
		gwhttp.WriteJSONError(w, http.StatusBadRequest, err.Error(), "invalid_request")
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

	result, err := rt.RAG().IngestManifest(r.Context(), manifest, coords, rt.OperatorStore(), rag.ManifestIngestRequest{
		Coords:         coords,
		Manifest:       manifest,
		RequestID:      rid,
		IndexRunID:     indexRun,
		ConversationID: convID,
	})
	if err != nil {
		if log != nil {
			args := []any{"msg", "ingest.failed", "tenant", sess.TenantID, "source", manifest.Source, "err", err, "service", "gateway", "principal_id", sess.TenantID, "timeline_kind", "indexer"}
			if rid != "" {
				args = append(args, "request_id", rid)
			}
			log.Error("ingest failed", args...)
		}
		gwhttp.WriteJSONError(w, http.StatusBadGateway, err.Error(), "gateway_upstream")
		return
	}

	writeIngestResult(w, r, rt, log, sess.TenantID, coords, result, indexRun, convID, rid, manifest.Source, 0)
}

func writeIngestResult(w http.ResponseWriter, r *http.Request, rt *gruntime.Runtime, log *slog.Logger, tenant string, coords vectorstore.Coords, result rag.IngestResult, indexRun, convID, rid, source string, textBytes int) {
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
			"tenant", tenant, "source", source, "chunks", result.Chunks,
			"service", "gateway", "principal_id", tenant,
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
		lvl := slog.LevelInfo
		if indexRun != "" {
			lvl = slog.LevelDebug
		}
		log.Log(r.Context(), lvl, "ingest complete", args...)
	}
	if rec := rt.Metrics(); rec != nil {
		if ragSvc := rt.RAG(); ragSvc != nil {
			mid := strings.TrimSpace(ragSvc.EmbeddingModel())
			if mid != "" {
				est := textBytes / 4
				if est < 1 {
					est = result.Chunks * 128
				}
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

// readManifestBody reads a complete JSON manifest from the request body.
func readManifestBody(r *http.Request) (rag.IngestManifest, error) {
	var manifest rag.IngestManifest
	b, err := io.ReadAll(r.Body)
	if err != nil {
		return manifest, err
	}
	if err := json.Unmarshal(b, &manifest); err != nil {
		return manifest, err
	}
	return manifest, nil
}
