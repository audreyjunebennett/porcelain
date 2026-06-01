package rag

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/rag"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/adminui/handler"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/indexerapi"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/scope"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/vectorstore"
	"github.com/lynn/porcelain/internal/operatorapi"
)

const maxSearchBodyBytes = 1 << 20

func handleSearch(h *handler.Handler, w http.ResponseWriter, r *http.Request, tenantID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	h.RT.Sync()
	res, _, _ := h.RT.Snapshot()
	if res == nil || !res.RAG.Enabled || h.RT.RAG() == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "RAG is not enabled"})
		return
	}

	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxSearchBodyBytes))
	var body operatorapi.RAGSearchRequest
	if err := dec.Decode(&body); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "invalid json"})
		return
	}
	projectID := strings.TrimSpace(body.ProjectID)
	if projectID == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "project_id required"})
		return
	}
	flavorID := strings.TrimSpace(body.FlavorID)

	coords := vectorstore.Coords{
		TenantID:  tenantID,
		ProjectID: scope.ResolveProject(projectID, res.RAG.DefaultProject),
		FlavorID:  scope.ResolveFlavor(flavorID, res.RAG.DefaultFlavor),
	}
	collection := vectorstore.CollectionName(coords)
	ragSvc := h.RT.RAG()

	topK := ragSvc.TopK()
	if body.TopK != nil && *body.TopK > 0 {
		topK = *body.TopK
	}
	scoreFloor := ragSvc.ScoreThreshold()
	if body.ScoreThreshold != nil && *body.ScoreThreshold > 0 {
		scoreFloor = float32(*body.ScoreThreshold)
	}

	query := strings.TrimSpace(body.Query)
	resp := operatorapi.RAGSearchResponse{
		Hits:           nil,
		Collection:     collection,
		TopK:           topK,
		ScoreThreshold: scoreFloor,
		IndexerHint:    buildIndexerHint(r, h, tenantID, coords, res.RAG.QdrantURL),
	}
	if query == "" {
		writeSearchResponse(w, resp)
		return
	}

	hits, err := ragSvc.Retrieve(r.Context(), rag.RetrieveRequest{
		Coords:         coords,
		Query:          query,
		TopK:           topK,
		ScoreThreshold: scoreFloor,
	})
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
		return
	}
	resp.Hits = searchHitsFromVector(hits)
	writeSearchResponse(w, resp)
}

func writeSearchResponse(w http.ResponseWriter, resp operatorapi.RAGSearchResponse) {
	if resp.Hits == nil {
		resp.Hits = []operatorapi.RAGSearchHit{}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func searchHitsFromVector(hits []vectorstore.Hit) []operatorapi.RAGSearchHit {
	if len(hits) == 0 {
		return nil
	}
	summaries := rag.SummarizeHits(hits)
	out := make([]operatorapi.RAGSearchHit, 0, len(hits))
	for i, h := range hits {
		src := strings.TrimSpace(h.Payload.Source)
		if src == "" {
			src = "unknown"
		}
		excerpt := ""
		if i < len(summaries) {
			excerpt = summaries[i].Text
		}
		out = append(out, operatorapi.RAGSearchHit{
			Source:      src,
			Score:       h.Score,
			TextExcerpt: excerpt,
			PointID:     h.ID,
		})
	}
	return out
}

func buildIndexerHint(r *http.Request, h *handler.Handler, tenantID string, coords vectorstore.Coords, qdrantURL string) string {
	ragSvc := h.RT.RAG()
	if ragSvc == nil {
		return ""
	}
	stats, err := ragSvc.StoreStats(r.Context(), coords)
	if err != nil {
		lower := strings.ToLower(err.Error())
		if strings.Contains(lower, "404") || strings.Contains(lower, "not found") {
			return "no_search_index"
		}
		return ""
	}
	if stats.Points == 0 {
		return "empty_collection"
	}
	health := indexerapi.BuildStorageHealthResponse(r.Context(), h.RT, h.Log, tenantID, qdrantURL)
	if ok, _ := health["ok"].(bool); !ok {
		if checks, _ := health["checks"].(map[string]any); checks != nil {
			if embed, _ := checks["embedding"].(map[string]any); embed != nil {
				if embedOK, _ := embed["ok"].(bool); !embedOK {
					if code, _ := embed["reason_code"].(string); code != "" {
						return code
					}
					return "embed_unavailable"
				}
			}
			if vs, _ := checks["vectorstore"].(map[string]any); vs != nil {
				if vsOK, _ := vs["ok"].(bool); !vsOK {
					if code, _ := vs["reason_code"].(string); code != "" {
						return code
					}
					return indexerapi.ReasonVectorstoreUnreachable
				}
			}
		}
	}
	return ""
}
