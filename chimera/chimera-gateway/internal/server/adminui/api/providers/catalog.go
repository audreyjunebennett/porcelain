package providers

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/brokeradmin"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/adminui/apirut"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/adminui/handler"
	"github.com/lynn/porcelain/internal/operatorapi"
)

func handleProviderCatalog(h *handler.Handler, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	h.RT.Sync()
	res, _ := h.RT.Snapshot()
	if res == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(operatorapi.ErrorBody{Error: "gateway not configured"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	client := apirut.BrokerAdminClient(h.RT)
	configured, listOK := brokeradmin.ListConfiguredProviders(ctx, client)
	resp := operatorapi.ProviderCatalogResponse{
		Providers:     append([]operatorapi.ProviderCatalogEntry(nil), apirut.ProviderCatalog...),
		ConfiguredIDs: apirut.ConfiguredProviderIDsResolved(ctx, client, configured, listOK),
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
