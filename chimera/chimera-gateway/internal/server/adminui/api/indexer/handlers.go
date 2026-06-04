package indexer

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/operatorstore"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/adminui/handler"
	"github.com/lynn/porcelain/chimera/chimera-indexer/adapter"
	"github.com/lynn/porcelain/internal/operatorapi"
	"gopkg.in/yaml.v3"
)

const maxIndexerConfigYAMLBytes = 512 << 10

// operatorIndexerTenantID is the tenant scope for UI-managed workspaces until the admin session carries a principal id.
func operatorIndexerTenantID() string { return "" }

func validateIndexerRootDir(rootPath string) (absRoot string, msg string, status int) {
	rootPath = strings.TrimSpace(rootPath)
	if rootPath == "" {
		return "", "path required", http.StatusBadRequest
	}
	abs, err := filepath.Abs(rootPath)
	if err != nil {
		return "", err.Error(), http.StatusBadRequest
	}
	st, err := os.Stat(abs)
	if err != nil || !st.IsDir() {
		return "", "path must be an existing directory", http.StatusBadRequest
	}
	return abs, "", 0
}

// stripRootsFromSupervisedYAML returns YAML bytes with roots cleared (tuning-only file on disk).
func stripRootsFromSupervisedYAML(raw []byte) ([]byte, error) {
	var fc adapter.FileConfig
	if err := yaml.Unmarshal(raw, &fc); err != nil {
		return nil, err
	}
	fc.Roots = nil
	return yaml.Marshal(&fc)
}

func workspacesAPIPayload(ctx context.Context, st *operatorstore.Store, tenantID string) (roots []map[string]any, nested []map[string]any, err error) {
	if st == nil {
		return nil, nil, nil
	}
	wss, err := st.ListWorkspaces(ctx, tenantID)
	if err != nil {
		return nil, nil, err
	}
	for _, w := range wss {
		pathObjs := make([]map[string]any, 0, len(w.Paths))
		for _, p := range w.Paths {
			pathObjs = append(pathObjs, map[string]any{
				"id":   p.ID,
				"path": p.Path,
			})
			wsIDStr := strconv.FormatInt(w.ID, 10)
			pidStr := strconv.FormatInt(p.ID, 10)
			roots = append(roots, map[string]any{
				"path_id":          pidStr,
				"workspace_row_id": wsIDStr,
				"workspace_id":     wsIDStr,
				"path":             p.Path,
				"project_id":       w.ProjectID,
				"flavor_id":        w.FlavorID,
			})
		}
		nested = append(nested, map[string]any{
			"id":         w.ID,
			"project_id": w.ProjectID,
			"flavor_id":  w.FlavorID,
			"paths":      pathObjs,
			"created_at": w.CreatedAt.UTC().Format(time.RFC3339Nano),
			"updated_at": w.UpdatedAt.UTC().Format(time.RFC3339Nano),
		})
	}
	return roots, nested, nil
}

// listIndexerOperatorWorkspaces returns workspaces for the authenticated token tenant.
// If that tenant has no rows, falls back to tenant_id "" (Phase 1 UI stored legacy rows with an empty tenant id).
func listIndexerOperatorWorkspaces(ctx context.Context, st *operatorstore.Store, tenantID string) ([]operatorstore.Workspace, error) {
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

func handleIndexerConfigGET(h *handler.Handler, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	h.RT.Sync()
	res, _ := h.RT.Snapshot()
	if res == nil {
		http.Error(w, "no config", http.StatusInternalServerError)
		return
	}
	path := strings.TrimSpace(res.IndexerSupervisedConfigPath)
	if path == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "indexer supervised config path not configured"})
		return
	}
	if err := adapter.EnsureSupervisedConfigFile(path); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
		return
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
		return
	}
	stripped, err := stripRootsFromSupervisedYAML(raw)
	yamlOut := string(stripped)
	if err != nil {
		yamlOut = string(raw)
	}
	ctx := r.Context()
	st := h.RT.OperatorStore()
	rootsFlat, workspacesNested, err := workspacesAPIPayload(ctx, st, operatorIndexerTenantID())
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(operatorapi.IndexerConfigResponse{
		Path:              path,
		YAML:              yamlOut,
		Roots:             rootsFlat,
		Workspaces:        workspacesNested,
		SupervisedEnabled: res.IndexerSupervisedEnabled,
		OperatorStore:     st != nil,
	})
}

func handleIndexerConfigPUT(h *handler.Handler, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	h.RT.Sync()
	res, _ := h.RT.Snapshot()
	if res == nil {
		http.Error(w, "no config", http.StatusInternalServerError)
		return
	}
	path := strings.TrimSpace(res.IndexerSupervisedConfigPath)
	if path == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "indexer supervised config path not configured"})
		return
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxIndexerConfigYAMLBytes+1<<12))
	var body struct {
		YAML string `json:"yaml"`
	}
	if err := dec.Decode(&body); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "invalid json"})
		return
	}
	y := strings.TrimSpace(body.YAML)
	if len(y) > maxIndexerConfigYAMLBytes {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "yaml too large"})
		return
	}
	var fc adapter.FileConfig
	if err := yaml.Unmarshal([]byte(y), &fc); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": fmt.Sprintf("invalid indexer yaml: %v", err)})
		return
	}
	fc.Roots = nil
	out, err := yaml.Marshal(&fc)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
		return
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(operatorapi.IndexerConfigPutResponse{OK: true, Path: path})
}

func handleIndexerWorkspacesGET(h *handler.Handler, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	st := h.RT.OperatorStore()
	if st == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "operator store unavailable"})
		return
	}
	_, nested, err := workspacesAPIPayload(r.Context(), st, operatorIndexerTenantID())
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(operatorapi.IndexerWorkspacesResponse{Workspaces: nested})
}

func handleIndexerWorkspacesPOST(h *handler.Handler, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	st := h.RT.OperatorStore()
	if st == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "operator store unavailable"})
		return
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	var body struct {
		ProjectID string   `json:"project_id"`
		FlavorID  string   `json:"flavor_id"`
		Paths     []string `json:"paths"`
	}
	if err := dec.Decode(&body); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "invalid json"})
		return
	}
	var absPaths []string
	for _, p := range body.Paths {
		abs, msg, stCode := validateIndexerRootDir(p)
		if stCode != 0 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(stCode)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": msg})
			return
		}
		absPaths = append(absPaths, abs)
	}
	uiTenant := operatorIndexerTenantID()
	ws, err := st.CreateWorkspace(r.Context(), uiTenant, strings.TrimSpace(body.ProjectID), strings.TrimSpace(body.FlavorID), absPaths)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
		return
	}
	if h.Log != nil {
		h.Log.Info("operator workspace created",
			"msg", "gateway.operator.workspace.created",
			"type", "gateway.operator.workspace.created",
			"workspace_id", ws.ID,
			"project_id", ws.ProjectID,
			"flavor_id", ws.FlavorID,
			"path_count", len(absPaths),
		)
	}
	roots, nested, _ := workspacesAPIPayload(r.Context(), st, operatorIndexerTenantID())
	var wsMap map[string]any
	for _, x := range nested {
		if id, ok := x["id"].(int64); ok && id == ws.ID {
			wsMap = x
			break
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(operatorapi.IndexerWorkspaceCreateResponse{
		OK:        true,
		Workspace: wsMap,
		Roots:     roots,
	})
}

func handleIndexerWorkspacePUT(h *handler.Handler, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "invalid workspace id"})
		return
	}
	st := h.RT.OperatorStore()
	if st == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "operator store unavailable"})
		return
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	var body struct {
		ProjectID string `json:"project_id"`
		FlavorID  string `json:"flavor_id"`
	}
	if err := dec.Decode(&body); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "invalid json"})
		return
	}
	if err := st.UpdateWorkspaceProjectFlavor(r.Context(), operatorIndexerTenantID(), id, strings.TrimSpace(body.ProjectID), strings.TrimSpace(body.FlavorID)); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
		return
	}
	if h.Log != nil {
		h.Log.Info("operator workspace updated",
			"msg", "gateway.operator.workspace.updated",
			"type", "gateway.operator.workspace.updated",
			"workspace_id", id,
			"project_id", strings.TrimSpace(body.ProjectID),
			"flavor_id", strings.TrimSpace(body.FlavorID),
		)
	}
	roots, _, _ := workspacesAPIPayload(r.Context(), st, operatorIndexerTenantID())
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(operatorapi.IndexerRootsResponse{OK: true, Roots: roots})
}

func handleIndexerWorkspaceDELETE(h *handler.Handler, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "invalid workspace id"})
		return
	}
	st := h.RT.OperatorStore()
	if st == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "operator store unavailable"})
		return
	}
	ws, err := st.GetWorkspace(r.Context(), operatorIndexerTenantID(), id)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
		return
	}
	if ws == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "workspace not found"})
		return
	}
	if err := purgeWorkspaceCorpusIfEnabled(r.Context(), h.RT, ws, strings.TrimSpace(h.SessionPrincipal(r)), h.Log); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
		return
	}
	if err := st.DeleteWorkspace(r.Context(), operatorIndexerTenantID(), id); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
		return
	}
	if h.Log != nil {
		h.Log.Info("operator workspace deleted",
			"msg", "gateway.operator.workspace.deleted",
			"type", "gateway.operator.workspace.deleted",
			"workspace_id", id,
		)
	}
	roots, _, _ := workspacesAPIPayload(r.Context(), st, operatorIndexerTenantID())
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(operatorapi.IndexerRootsResponse{OK: true, Roots: roots})
}

func handleIndexerWorkspacePathPOST(h *handler.Handler, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	wsID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || wsID < 1 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "invalid workspace id"})
		return
	}
	st := h.RT.OperatorStore()
	if st == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "operator store unavailable"})
		return
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	var body struct {
		Path string `json:"path"`
	}
	if err := dec.Decode(&body); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "invalid json"})
		return
	}
	abs, msg, stCode := validateIndexerRootDir(body.Path)
	if stCode != 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(stCode)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": msg})
		return
	}
	if _, err := st.AddPath(r.Context(), operatorIndexerTenantID(), wsID, abs); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
		return
	}
	if h.Log != nil {
		h.Log.Info("operator workspace path added",
			"msg", "gateway.operator.workspace.path_added",
			"type", "gateway.operator.workspace.path_added",
			"workspace_id", wsID,
			"path", abs,
		)
	}
	roots, _, _ := workspacesAPIPayload(r.Context(), st, operatorIndexerTenantID())
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(operatorapi.IndexerRootsResponse{OK: true, Roots: roots})
}

func handleIndexerWorkspacePathPUT(h *handler.Handler, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	pathID, err := strconv.ParseInt(r.PathValue("pathid"), 10, 64)
	if err != nil || pathID < 1 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "invalid path id"})
		return
	}
	st := h.RT.OperatorStore()
	if st == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "operator store unavailable"})
		return
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	var body struct {
		Path      *string `json:"path"`
		ProjectID *string `json:"project_id"`
		FlavorID  *string `json:"flavor_id"`
	}
	if err := dec.Decode(&body); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "invalid json"})
		return
	}
	var newAbs *string
	if body.Path != nil {
		abs, msg, stCode := validateIndexerRootDir(*body.Path)
		if stCode != 0 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(stCode)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": msg})
			return
		}
		newAbs = &abs
	}
	if err := st.UpdatePath(r.Context(), operatorIndexerTenantID(), pathID, newAbs, body.ProjectID, body.FlavorID); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
		return
	}
	roots, _, _ := workspacesAPIPayload(r.Context(), st, operatorIndexerTenantID())
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(operatorapi.IndexerRootsResponse{OK: true, Roots: roots})
}

func handleIndexerWorkspacePathDELETE(h *handler.Handler, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	pathID, err := strconv.ParseInt(r.PathValue("pathid"), 10, 64)
	if err != nil || pathID < 1 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "invalid path id"})
		return
	}
	st := h.RT.OperatorStore()
	if st == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "operator store unavailable"})
		return
	}
	if err := st.DeletePath(r.Context(), operatorIndexerTenantID(), pathID); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
		return
	}
	if h.Log != nil {
		h.Log.Info("operator workspace path deleted",
			"msg", "gateway.operator.workspace.path_deleted",
			"type", "gateway.operator.workspace.path_deleted",
			"path_id", pathID,
		)
	}
	roots, _, _ := workspacesAPIPayload(r.Context(), st, operatorIndexerTenantID())
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(operatorapi.IndexerRootsResponse{OK: true, Roots: roots})
}
