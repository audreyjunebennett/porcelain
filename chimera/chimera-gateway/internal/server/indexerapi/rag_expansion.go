package indexerapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/gwhttp"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/rag"
	gruntime "github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/runtime"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/scope"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/vectorstore"
)

func expansionService(rt *gruntime.Runtime) *rag.ExpansionService {
	res, _ := rt.Snapshot()
	return rag.NewExpansionService(rag.ExpansionOptions{
		RAG:           rt.RAG(),
		OperatorStore: rt.OperatorStore(),
		StaleStore:    rt.CorpusStaleStore(),
		Resolved:      res,
	})
}

func ragCoordsFromRequest(r *http.Request, rt *gruntime.Runtime) vectorstore.Coords {
	res, tokStore := rt.Snapshot()
	tenant := ""
	if sess := tokStore.Validate(gwhttp.BearerToken(r.Header.Get("Authorization"))); sess != nil {
		tenant = sess.TenantID
	}
	return vectorstore.Coords{
		TenantID:  tenant,
		ProjectID: scope.ResolveProject(r.Header.Get(scope.HeaderProject), res.RAG.DefaultProject),
		FlavorID:  scope.ResolveFlavor(r.Header.Get(scope.HeaderFlavor), res.RAG.DefaultFlavor),
	}
}

// HandleRAGSegmentsGET implements GET /v1/rag/segments.
func HandleRAGSegmentsGET(w http.ResponseWriter, r *http.Request, rt *gruntime.Runtime) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	rt.Sync()
	res, tokStore := rt.Snapshot()
	token := gwhttp.BearerToken(r.Header.Get("Authorization"))
	if tokStore.Validate(token) == nil {
		gwhttp.WriteJSONError(w, http.StatusUnauthorized, "Unauthorized", "invalid_api_key")
		return
	}
	if !res.RAG.Enabled || rt.RAG() == nil {
		gwhttp.WriteJSONError(w, http.StatusServiceUnavailable, "RAG is not enabled", "gateway_config")
		return
	}
	source := strings.TrimSpace(r.URL.Query().Get("source"))
	hash := strings.TrimSpace(r.URL.Query().Get("content_sha256"))
	if source == "" || hash == "" {
		gwhttp.WriteJSONError(w, http.StatusBadRequest, "source and content_sha256 required", "invalid_request")
		return
	}
	exp := expansionService(rt)
	rows, err := exp.ListSegments(r.Context(), ragCoordsFromRequest(r, rt), source, hash)
	if err != nil {
		writeExpansionError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"object":   "rag.segments",
		"source":   source,
		"segments": rows,
	})
}

// HandleRAGContextGET implements GET /v1/rag/context.
func HandleRAGContextGET(w http.ResponseWriter, r *http.Request, rt *gruntime.Runtime) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	rt.Sync()
	res, tokStore := rt.Snapshot()
	token := gwhttp.BearerToken(r.Header.Get("Authorization"))
	if tokStore.Validate(token) == nil {
		gwhttp.WriteJSONError(w, http.StatusUnauthorized, "Unauthorized", "invalid_api_key")
		return
	}
	if !res.RAG.Enabled || rt.RAG() == nil {
		gwhttp.WriteJSONError(w, http.StatusServiceUnavailable, "RAG is not enabled", "gateway_config")
		return
	}
	q := r.URL.Query()
	source := strings.TrimSpace(q.Get("source"))
	hash := strings.TrimSpace(q.Get("content_sha256"))
	line, _ := strconv.Atoi(q.Get("line"))
	before, _ := strconv.Atoi(q.Get("before"))
	after, _ := strconv.Atoi(q.Get("after"))
	if source == "" {
		gwhttp.WriteJSONError(w, http.StatusBadRequest, "source required", "invalid_request")
		return
	}
	if hash == "" {
		gwhttp.WriteJSONError(w, http.StatusBadRequest, "content_sha256 required", "invalid_request")
		return
	}
	if startS, endS := strings.TrimSpace(q.Get("start_line")), strings.TrimSpace(q.Get("end_line")); startS != "" && endS != "" {
		start, _ := strconv.Atoi(startS)
		end, _ := strconv.Atoi(endS)
		if start > 0 && end >= start {
			line = start
			before = 0
			after = end - start
		}
	}
	exp := expansionService(rt)
	text, err := exp.ContextAround(r.Context(), ragCoordsFromRequest(r, rt), source, hash, line, before, after)
	if err != nil {
		writeExpansionError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"object": "rag.context",
		"source": source,
		"line":   line,
		"text":   text,
	})
}

// HandleRAGAdjacentGET implements GET /v1/rag/adjacent.
func HandleRAGAdjacentGET(w http.ResponseWriter, r *http.Request, rt *gruntime.Runtime) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	rt.Sync()
	res, tokStore := rt.Snapshot()
	token := gwhttp.BearerToken(r.Header.Get("Authorization"))
	if tokStore.Validate(token) == nil {
		gwhttp.WriteJSONError(w, http.StatusUnauthorized, "Unauthorized", "invalid_api_key")
		return
	}
	if !res.RAG.Enabled || rt.RAG() == nil {
		gwhttp.WriteJSONError(w, http.StatusServiceUnavailable, "RAG is not enabled", "gateway_config")
		return
	}
	pointID := strings.TrimSpace(r.URL.Query().Get("point_id"))
	radius, _ := strconv.Atoi(r.URL.Query().Get("radius"))
	if pointID == "" {
		gwhttp.WriteJSONError(w, http.StatusBadRequest, "point_id required", "invalid_request")
		return
	}
	if radius <= 0 {
		radius = 1
	}
	exp := expansionService(rt)
	hits, err := exp.AdjacentChunks(r.Context(), ragCoordsFromRequest(r, rt), pointID, radius)
	if err != nil {
		writeExpansionError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"object": "rag.adjacent",
		"hits":   hits,
	})
}

// HandleRAGToolsGET lists workspace expansion tool definitions.
func HandleRAGToolsGET(w http.ResponseWriter, r *http.Request, rt *gruntime.Runtime) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	rt.Sync()
	res, tokStore := rt.Snapshot()
	token := gwhttp.BearerToken(r.Header.Get("Authorization"))
	if tokStore.Validate(token) == nil {
		gwhttp.WriteJSONError(w, http.StatusUnauthorized, "Unauthorized", "invalid_api_key")
		return
	}
	if !res.RAG.Enabled {
		gwhttp.WriteJSONError(w, http.StatusServiceUnavailable, "RAG is not enabled", "gateway_config")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"object": "rag.tools",
		"tools":  rag.WorkspaceToolDefinitions(),
	})
}

func writeExpansionError(w http.ResponseWriter, err error) {
	msg := err.Error()
	if strings.Contains(msg, "stale") {
		gwhttp.WriteJSONError(w, http.StatusConflict, msg, "corpus_stale")
		return
	}
	if strings.Contains(msg, "disabled") {
		gwhttp.WriteJSONError(w, http.StatusServiceUnavailable, msg, "gateway_config")
		return
	}
	gwhttp.WriteJSONError(w, http.StatusBadRequest, msg, "invalid_request")
}
