package indexer

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/operatorstore"
	gruntime "github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/runtime"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/vectorstore"
)

func workspaceCorpusCoords(ingestTenantID string, ws *operatorstore.Workspace) vectorstore.Coords {
	if ws == nil {
		return vectorstore.Coords{}
	}
	tenantID := strings.TrimSpace(ingestTenantID)
	if tenantID == "" {
		tenantID = strings.TrimSpace(ws.TenantID)
	}
	return vectorstore.Coords{
		TenantID:  tenantID,
		ProjectID: ws.ProjectID,
		FlavorID:  ws.FlavorID,
	}
}

func purgeWorkspaceCorpusIfEnabled(ctx context.Context, rt *gruntime.Runtime, ws *operatorstore.Workspace, ingestTenantID string, log *slog.Logger) error {
	if rt == nil || ws == nil {
		return nil
	}
	res, _ := rt.Snapshot()
	if res == nil || !res.RAG.Enabled {
		return nil
	}
	ragSvc := rt.RAG()
	if ragSvc == nil {
		return fmt.Errorf("vector store unavailable")
	}
	coords := workspaceCorpusCoords(ingestTenantID, ws)
	if _, err := ragSvc.PurgeWorkspaceCorpus(ctx, coords); err != nil {
		if log != nil {
			log.Warn("operator workspace corpus purge failed",
				"msg", "gateway.operator.workspace.purge_failed",
				"type", "gateway.operator.workspace.purge_failed",
				"workspace_id", ws.ID,
				"project_id", ws.ProjectID,
				"flavor_id", ws.FlavorID,
				"collection", vectorstore.CollectionName(coords),
				"err", err,
			)
		}
		return fmt.Errorf("vector purge failed: %w", err)
	}
	return nil
}
