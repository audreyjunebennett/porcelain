package indexerapi

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/brokeradmin"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/rag/embedprobe"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/adminui/api/providers"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/adminui/apirut"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/catalog"
	gruntime "github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/runtime"
	gwconfig "github.com/lynn/porcelain/chimera/internal/config"
	"github.com/lynn/porcelain/internal/naming"
)

// Stable reason codes for indexer branching and operator copy (see docs/indexer.md).
const (
	ReasonVectorstoreUnreachable  = "vectorstore_unreachable"
	ReasonEmbedModelNotInCatalog  = "embed_model_not_in_catalog"
	ReasonEmbedProviderDown       = "embed_provider_down"
	ReasonEmbedProviderKeyMissing = "embed_provider_key_missing"
	ReasonEmbedCatalogStale       = "embed_catalog_stale"
)

// BuildStorageHealthResponse assembles the JSON body for GET /v1/indexer/storage/health.
//
// Example (embedding unavailable, vector store OK):
//
//	{
//	  "object": "indexer.storage.health",
//	  "ok": false,
//	  "status": "degraded",
//	  "tenant_id": "lynn",
//	  "backend": "qdrant",
//	  "url": "http://127.0.0.1:6333",
//	  "checks": {
//	    "vectorstore": {"ok": true, "status": "ok", "detail": ""},
//	    "embedding": {
//	      "ok": false,
//	      "status": "unavailable",
//	      "model": "ollama/nomic-embed-text:latest",
//	      "model_in_catalog": false,
//	      "provider": "ollama",
//	      "provider_state": "down",
//	      "reason_code": "embed_model_not_in_catalog",
//	      "detail": "no models available in live catalog"
//	    }
//	  }
//	}
//
// HTTP status is 200 for degraded dependency checks so the indexer can poll without treating
// the body as a transport error; auth and RAG-disabled errors remain 503 (see HandleHealth).
func BuildStorageHealthResponse(ctx context.Context, rt *gruntime.Runtime, log *slog.Logger, tenantID, qdrantURL string) map[string]any {
	vsCheck, vsOK := buildVectorstoreCheck(ctx, rt)
	embedCheck, embedOK := buildEmbeddingCheck(ctx, rt, log, rt.RAG().EmbeddingModel())

	allOK := vsOK && embedOK
	status := "ok"
	if !allOK {
		status = "degraded"
	}

	return map[string]any{
		"object":    "indexer.storage.health",
		"ok":        allOK,
		"status":    status,
		"tenant_id": tenantID,
		"backend":   naming.ProductVectorstoreName,
		"url":       qdrantURL,
		"checks": map[string]any{
			"vectorstore": vsCheck,
			"embedding":   embedCheck,
		},
	}
}

func buildVectorstoreCheck(ctx context.Context, rt *gruntime.Runtime) (map[string]any, bool) {
	err := rt.RAG().StoreHealth(ctx)
	out := map[string]any{
		"ok":     err == nil,
		"status": checkStatusLabel(err == nil, false),
	}
	if err != nil {
		out["detail"] = err.Error()
		out["reason_code"] = ReasonVectorstoreUnreachable
	}
	return out, err == nil
}

func buildEmbeddingCheck(ctx context.Context, rt *gruntime.Runtime, log *slog.Logger, modelID string) (map[string]any, bool) {
	modelID = strings.TrimSpace(modelID)
	out := map[string]any{
		"ok":     true,
		"status": "ok",
		"model":  modelID,
	}
	if modelID == "" {
		out["ok"] = false
		out["status"] = "unavailable"
		out["reason_code"] = ReasonEmbedModelNotInCatalog
		out["detail"] = "embedding model not configured"
		out["model_in_catalog"] = false
		return out, false
	}

	res, _, _ := rt.Snapshot()
	if res != nil && gwconfig.UsesInternalProvider(modelID, res.InternalEmbedding) {
		return buildInternalEmbeddingCheck(ctx, rt, modelID, res, out)
	}

	snap := catalogSnapshotForIndexerHealth(ctx, rt, log)
	now := time.Now()
	fresh := snap != nil && snap.IsFresh(now, catalog.CatalogSnapshotFreshness)
	if !fresh {
		out["ok"] = false
		out["status"] = "degraded"
		out["reason_code"] = ReasonEmbedCatalogStale
		out["model_in_catalog"] = false
		out["detail"] = catalogStaleDetail(snap)
		return out, false
	}

	inCatalog := snap.HasModel(modelID)
	out["model_in_catalog"] = inCatalog
	if !inCatalog {
		out["ok"] = false
		out["status"] = "unavailable"
		out["reason_code"] = ReasonEmbedModelNotInCatalog
		out["detail"] = "configured embedding model absent from live catalog"
		provider := providerFromModelID(modelID)
		if provider != "" {
			out["provider"] = provider
			out["provider_state"] = providerStateFromSnapshot(snap, provider)
		}
		return out, false
	}

	provider := providerFromModelID(modelID)
	if provider == "" {
		return out, true
	}
	out["provider"] = provider

	client := apirut.BrokerAdminClient(rt)
	configured, listOK := brokeradmin.ListConfiguredProviders(ctx, client)
	body, status, transportErr, _ := brokeradmin.GetProviderForProbeWithList(ctx, client, provider, configured, listOK)
	entry := providers.ClassifyBrokerProviderResult(provider, body, status, transportErr, snap)
	out["provider_state"] = entry.State
	if entry.Error != "" {
		out["detail"] = entry.Error
	}

	switch entry.State {
	case "key_missing":
		out["ok"] = false
		out["status"] = "unavailable"
		out["reason_code"] = ReasonEmbedProviderKeyMissing
		return out, false
	case "down":
		out["ok"] = false
		out["status"] = "unavailable"
		out["reason_code"] = ReasonEmbedProviderDown
		if d, _ := out["detail"].(string); d == "" {
			out["detail"] = "embedding provider unavailable"
		}
		return out, false
	case "not_configured":
		out["ok"] = false
		out["status"] = "unavailable"
		out["reason_code"] = ReasonEmbedProviderDown
		if d, _ := out["detail"].(string); d == "" {
			out["detail"] = "embedding provider not configured"
		}
		return out, false
	default:
		return out, true
	}
}

func buildInternalEmbeddingCheck(ctx context.Context, rt *gruntime.Runtime, modelID string, res *gwconfig.Resolved, out map[string]any) (map[string]any, bool) {
	provider := res.InternalEmbedding.Provider
	out["provider"] = provider
	out["provider_state"] = "up"
	out["model_in_catalog"] = true

	embURL := res.RAG.EmbeddingURL(res.UpstreamBaseURL)
	if err := embedprobe.Probe(ctx, embURL, modelID, res.RAG.EmbeddingDim); err != nil {
		out["ok"] = false
		out["status"] = "unavailable"
		out["reason_code"] = ReasonEmbedProviderDown
		out["provider_state"] = "down"
		out["detail"] = err.Error()
		return out, false
	}
	return out, true
}

func catalogSnapshotForIndexerHealth(ctx context.Context, rt *gruntime.Runtime, log *slog.Logger) *catalog.CatalogSnapshot {
	if rt == nil {
		return nil
	}
	snap := rt.CatalogSnapshot()
	if snap != nil && snap.IsFresh(time.Now(), catalog.CatalogSnapshotFreshness) {
		return snap
	}
	return gruntime.RefreshAvailableModels(ctx, rt, log)
}

func catalogStaleDetail(snap *catalog.CatalogSnapshot) string {
	if snap == nil {
		return "model catalog not yet available"
	}
	if !snap.OK {
		errText := strings.TrimSpace(snap.FetchErr)
		if errText != "" {
			return errText
		}
		return "model catalog unavailable"
	}
	return "model catalog snapshot is stale"
}

func providerFromModelID(modelID string) string {
	modelID = strings.TrimSpace(modelID)
	if slash := strings.Index(modelID, "/"); slash > 0 {
		return strings.ToLower(modelID[:slash])
	}
	return ""
}

func providerStateFromSnapshot(snap *catalog.CatalogSnapshot, provider string) string {
	if snap == nil || !snap.OK {
		return "down"
	}
	if snap.HasProvider(provider) {
		return "up"
	}
	return "down"
}

func checkStatusLabel(ok, degraded bool) string {
	if ok {
		return "ok"
	}
	if degraded {
		return "degraded"
	}
	return "unavailable"
}
