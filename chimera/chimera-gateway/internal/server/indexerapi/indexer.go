package indexerapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/gwhttp"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/operatorstore"
	gruntime "github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/runtime"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/scope"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/vectorstore"
)

// HandleWorkspaces returns persisted operator workspaces for the Bearer token tenant
// (GET /v1/indexer/workspaces). Same auth and RAG gating as HandleConfig.
func HandleWorkspaces(w http.ResponseWriter, r *http.Request, rt *gruntime.Runtime, _ *slog.Logger) {
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
	st := rt.OperatorStore()
	if st == nil {
		gwhttp.WriteJSONError(w, http.StatusServiceUnavailable, "Operator workspace store is not available", "gateway_config")
		return
	}
	ctx := r.Context()
	wss, err := listOperatorWorkspaces(ctx, st, sess.TenantID)
	if err != nil {
		gwhttp.WriteJSONError(w, http.StatusInternalServerError, err.Error(), "gateway_upstream")
		return
	}

	type pathOut struct {
		PathID int64  `json:"path_id"`
		Path   string `json:"path"`
	}
	type wsOut struct {
		WorkspaceID int64     `json:"workspace_id"`
		ProjectID   string    `json:"project_id"`
		FlavorID    string    `json:"flavor_id"`
		Paths       []pathOut `json:"paths"`
	}
	payload := struct {
		Object     string  `json:"object"`
		TenantID   string  `json:"tenant_id"`
		Workspaces []wsOut `json:"workspaces"`
	}{
		Object:     "indexer.workspaces",
		TenantID:   sess.TenantID,
		Workspaces: make([]wsOut, 0, len(wss)),
	}
	for _, row := range wss {
		paths := make([]pathOut, 0, len(row.Paths))
		for _, p := range row.Paths {
			paths = append(paths, pathOut{PathID: p.ID, Path: p.Path})
		}
		payload.Workspaces = append(payload.Workspaces, wsOut{
			WorkspaceID: row.ID,
			ProjectID:   row.ProjectID,
			FlavorID:    row.FlavorID,
			Paths:       paths,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

// HandleConfig returns the effective RAG / indexer settings for the authenticated tenant.
func HandleConfig(w http.ResponseWriter, r *http.Request, rt *gruntime.Runtime, _ *slog.Logger) {
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
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"object":                   "indexer.config",
		"gateway_version":          res.Semver,
		"chunk_size":               rt.RAG().ChunkSize(),
		"chunk_overlap":            rt.RAG().ChunkOverlap(),
		"top_k":                    rt.RAG().TopK(),
		"score_threshold":          res.RAG.ScoreThreshold,
		"embedding_model":          rt.RAG().EmbeddingModel(),
		"embedding_dim":            rt.RAG().EmbedDim(),
		"ingest_method":            "POST",
		"ingest_path":              "/v1/ingest",
		"max_ingest_bytes":         res.RAG.MaxIngestBytes,
		"max_whole_file_bytes":     res.RAG.MaxWholeFileBytes,
		"ingest_session_path":      "/v1/ingest/session",
		"ingest_chunk_path_tpl":    "/v1/ingest/session/{session_id}/chunk",
		"ingest_complete_path_tpl": "/v1/ingest/session/{session_id}/complete",
		"corpus_inventory_path":    "/v1/indexer/corpus/inventory",
		"required_headers":         []string{"Authorization"},
		"optional_headers":         []string{scope.HeaderProject, scope.HeaderFlavor, scope.HeaderIndexRun},
		"payload_fields": []string{
			"tenant_id", "project_id", "text", "source", "flavor_id", "created_at",
			"content_sha256", "client_content_hash",
		},
		"collection_naming": map[string]any{
			"scheme":  "chimera-<tenant>-<project>-<flavor>-<sha1prefix>",
			"scope":   res.RAG.CollectionScope,
			"example": vectorstore.CollectionName(vectorstore.Coords{TenantID: sess.TenantID, ProjectID: defaultOr(res.RAG.DefaultProject, "default"), FlavorID: res.RAG.DefaultFlavor}),
		},
		"defaults": map[string]any{
			"project_id": res.RAG.DefaultProject,
			"flavor_id":  res.RAG.DefaultFlavor,
		},
		"tenant_id":    sess.TenantID,
		"user_label":   strings.TrimSpace(sess.Label),
		"principal_id": sess.TenantID,
	})
}

// HandleHealth probes vector store and embedding readiness for the authenticated tenant.
// Degraded dependency checks return HTTP 200 with ok:false so the indexer can poll without
// treating the JSON body as a transport error; auth and RAG-disabled errors remain 503.
func HandleHealth(w http.ResponseWriter, r *http.Request, rt *gruntime.Runtime, log *slog.Logger) {
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
	resp := BuildStorageHealthResponse(r.Context(), rt, log, sess.TenantID, res.RAG.QdrantURL)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

// HandleStats returns live vectorstore stats for the scoped collection.
func HandleStats(w http.ResponseWriter, r *http.Request, rt *gruntime.Runtime, _ *slog.Logger) {
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
	coords := vectorstore.Coords{
		TenantID:  sess.TenantID,
		ProjectID: scope.ResolveProject(r.Header.Get(scope.HeaderProject), res.RAG.DefaultProject),
		FlavorID:  scope.ResolveFlavor(r.Header.Get(scope.HeaderFlavor), res.RAG.DefaultFlavor),
	}
	st, err := rt.RAG().StoreStats(r.Context(), coords)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"object":     "indexer.storage.stats",
			"collection": vectorstore.CollectionName(coords),
			"tenant_id":  coords.TenantID,
			"project_id": coords.ProjectID,
			"flavor_id":  coords.FlavorID,
			"points":     0,
			"vector_dim": rt.RAG().EmbedDim(),
			"available":  false,
			"detail":     err.Error(),
		})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"object":     "indexer.storage.stats",
		"collection": st.Collection,
		"tenant_id":  coords.TenantID,
		"project_id": coords.ProjectID,
		"flavor_id":  coords.FlavorID,
		"points":     st.Points,
		"vector_dim": st.VectorDim,
		"available":  true,
	})
}

// HandleCorpusInventory returns a paginated list of unique sources in the scoped corpus.
func HandleCorpusInventory(w http.ResponseWriter, r *http.Request, rt *gruntime.Runtime, _ *slog.Logger) {
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
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	limit := 256
	if s := strings.TrimSpace(r.URL.Query().Get("limit")); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > 2000 {
		limit = 2000
	}
	cursor := strings.TrimSpace(r.URL.Query().Get("cursor"))

	coords := vectorstore.Coords{
		TenantID:  sess.TenantID,
		ProjectID: scope.ResolveProject(r.Header.Get(scope.HeaderProject), res.RAG.DefaultProject),
		FlavorID:  scope.ResolveFlavor(r.Header.Get(scope.HeaderFlavor), res.RAG.DefaultFlavor),
	}
	entries, next, err := rt.RAG().CorpusInventory(r.Context(), coords, limit, cursor)
	if err != nil {
		gwhttp.WriteJSONError(w, http.StatusBadGateway, err.Error(), "gateway_upstream")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"object":      "indexer.corpus.inventory",
		"tenant_id":   coords.TenantID,
		"project_id":  coords.ProjectID,
		"flavor_id":   coords.FlavorID,
		"collection":  vectorstore.CollectionName(coords),
		"entries":     entries,
		"has_more":    next != "",
		"next_cursor": next,
	})
}

func defaultOr(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// listOperatorWorkspaces returns workspaces for tenantID, falling back to "" when empty.
func listOperatorWorkspaces(ctx context.Context, st *operatorstore.Store, tenantID string) ([]operatorstore.Workspace, error) {
	if st == nil {
		return nil, nil
	}
	wss, err := st.ListWorkspaces(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if len(wss) == 0 && strings.TrimSpace(tenantID) != "" {
		return st.ListWorkspaces(ctx, "")
	}
	return wss, nil
}
