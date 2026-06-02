package rag

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/rag/ragembed"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/adminui/handler"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/catalog"
	gruntime "github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/runtime"
	"github.com/lynn/porcelain/chimera/internal/config"
	"github.com/lynn/porcelain/internal/operatorapi"
)

const maxEmbeddingPutBodyBytes = 1 << 12

func handleEmbeddingGET(h *handler.Handler, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	h.RT.Sync()
	res, _ := h.RT.Snapshot()
	if res == nil || !res.RAG.Enabled {
		writeEmbeddingError(w, http.StatusServiceUnavailable, "RAG is not enabled")
		return
	}

	model := strings.TrimSpace(res.RAG.EmbeddingModel)
	dim := res.RAG.EmbeddingDim
	snap := catalogSnapshotForEmbedding(r.Context(), h.RT, h.Log)
	now := time.Now()
	fresh := snap != nil && snap.IsFresh(now, catalog.CatalogSnapshotFreshness)

	resp := operatorapi.RAGEmbeddingGetResponse{
		Model:          model,
		Dim:            dim,
		Status:         "ok",
		ModelInCatalog: false,
		CatalogStale:   !fresh,
		Candidates:     buildEmbeddingCandidates(snap),
	}
	if !fresh {
		resp.Status = embedStatusFromReason("embed_catalog_stale")
	} else if model != "" && snap != nil && snap.OK {
		resp.ModelInCatalog = snap.HasModel(model)
		if !resp.ModelInCatalog {
			resp.Status = embedStatusFromReason("embed_model_not_in_catalog")
		}
	} else if model == "" {
		resp.Status = embedStatusFromReason("embed_model_not_in_catalog")
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func handleEmbeddingPUT(h *handler.Handler, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	h.RT.Sync()
	res, _ := h.RT.Snapshot()
	if res == nil || !res.RAG.Enabled {
		writeEmbeddingError(w, http.StatusServiceUnavailable, "RAG is not enabled")
		return
	}

	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxEmbeddingPutBodyBytes))
	var body operatorapi.RAGEmbeddingPutRequest
	if err := dec.Decode(&body); err != nil {
		writeEmbeddingError(w, http.StatusBadRequest, "invalid json")
		return
	}
	model := strings.TrimSpace(body.Model)
	if model == "" {
		writeEmbeddingError(w, http.StatusBadRequest, "model required")
		return
	}

	snap := catalogSnapshotForEmbedding(r.Context(), h.RT, h.Log)
	now := time.Now()
	if snap == nil || !snap.IsFresh(now, catalog.CatalogSnapshotFreshness) {
		writeEmbeddingError(w, http.StatusServiceUnavailable, "model catalog stale or unavailable")
		return
	}
	if !snap.OK {
		writeEmbeddingError(w, http.StatusServiceUnavailable, strings.TrimSpace(snap.FetchErr))
		return
	}
	if !snap.HasModel(model) {
		writeEmbeddingError(w, http.StatusBadRequest, "model not in live catalog")
		return
	}

	dim, err := resolveEmbeddingDim(r.Context(), h.RT, res, model)
	if err != nil {
		writeEmbeddingError(w, http.StatusBadRequest, err.Error())
		return
	}

	gwPath := h.RT.GatewayPath()
	if strings.TrimSpace(gwPath) == "" {
		writeEmbeddingError(w, http.StatusInternalServerError, "gateway config path unavailable")
		return
	}
	if err := config.WriteGatewayEmbeddingModel(gwPath, model, dim); err != nil {
		if h.Log != nil {
			h.Log.Error("embedding model persist failed",
				"msg", "gateway.operator.rag.embedding.persist_failed",
				"model", model,
				"err", err,
			)
		}
		writeEmbeddingError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.RT.Sync()
	if h.Log != nil {
		h.Log.Info("embedding model updated",
			"msg", "gateway.operator.rag.embedding.updated",
			"type", "gateway.operator.rag.embedding.updated",
			"model", model,
			"dim", dim,
		)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(operatorapi.RAGEmbeddingPutResponse{
		OK:    true,
		Model: model,
		Dim:   dim,
	})
}

func writeEmbeddingError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": msg})
}

func catalogSnapshotForEmbedding(ctx context.Context, rt *gruntime.Runtime, log *slog.Logger) *catalog.CatalogSnapshot {
	if rt == nil {
		return nil
	}
	snap := rt.CatalogSnapshot()
	if snap != nil && snap.IsFresh(time.Now(), catalog.CatalogSnapshotFreshness) {
		return snap
	}
	return gruntime.RefreshAvailableModels(ctx, rt, log)
}

func embedStatusFromReason(reason string) string {
	switch strings.TrimSpace(reason) {
	case "embed_model_not_in_catalog", "embed_catalog_stale":
		return reason
	default:
		return "ok"
	}
}

func isEmbeddingLikelyModelID(id string) bool {
	lower := strings.ToLower(strings.TrimSpace(id))
	if lower == "" {
		return false
	}
	if strings.Contains(lower, "embed") {
		return true
	}
	for _, sub := range []string{"bge-", "e5-", "jina-embeddings"} {
		if strings.Contains(lower, sub) {
			return true
		}
	}
	return false
}

func buildEmbeddingCandidates(snap *catalog.CatalogSnapshot) []operatorapi.RAGEmbeddingCandidate {
	if snap == nil || !snap.OK || len(snap.ModelIDs) == 0 {
		return nil
	}
	likely := make([]operatorapi.RAGEmbeddingCandidate, 0, len(snap.ModelIDs))
	other := make([]operatorapi.RAGEmbeddingCandidate, 0, len(snap.ModelIDs))
	for _, raw := range snap.ModelIDs {
		id := strings.TrimSpace(raw)
		if id == "" {
			continue
		}
		c := operatorapi.RAGEmbeddingCandidate{ID: id, EmbeddingLikely: isEmbeddingLikelyModelID(id)}
		if dim, ok := config.KnownEmbeddingDim(id); ok {
			c.KnownDim = dim
		}
		if c.EmbeddingLikely {
			likely = append(likely, c)
		} else {
			other = append(other, c)
		}
	}
	sort.Slice(likely, func(i, j int) bool { return likely[i].ID < likely[j].ID })
	sort.Slice(other, func(i, j int) bool { return other[i].ID < other[j].ID })
	out := make([]operatorapi.RAGEmbeddingCandidate, 0, len(likely)+len(other))
	out = append(out, likely...)
	out = append(out, other...)
	return out
}

func resolveEmbeddingDim(ctx context.Context, rt *gruntime.Runtime, res *config.Resolved, model string) (int, error) {
	if dim, ok := config.KnownEmbeddingDim(model); ok {
		return dim, nil
	}
	if rt == nil || res == nil {
		return 0, errEmbeddingDimUnknown
	}
	url := res.RAG.EmbeddingURL(res.UpstreamBaseURL)
	probeCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	dim, err := ragembed.ProbeDim(probeCtx, url, rt.UpstreamAPIKey(), model, nil)
	if err != nil {
		return 0, err
	}
	if dim <= 0 {
		return 0, errEmbeddingDimUnknown
	}
	return dim, nil
}

var errEmbeddingDimUnknown = errString("could not determine embedding dimension for model")

type errString string

func (e errString) Error() string { return string(e) }

// BuildEmbeddingCandidatesForTest exports candidate ordering for unit tests.
func BuildEmbeddingCandidatesForTest(snap *catalog.CatalogSnapshot) []operatorapi.RAGEmbeddingCandidate {
	return buildEmbeddingCandidates(snap)
}

// ResolveEmbeddingDimForTest exports dim resolution for unit tests.
func ResolveEmbeddingDimForTest(ctx context.Context, rt *gruntime.Runtime, res *config.Resolved, model string) (int, error) {
	return resolveEmbeddingDim(ctx, rt, res, model)
}
