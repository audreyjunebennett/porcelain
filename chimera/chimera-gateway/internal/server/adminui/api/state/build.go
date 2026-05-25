package state

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/brokeradmin"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/adminui/apirut"
	gruntime "github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/runtime"
	"github.com/lynn/porcelain/chimera/internal/brokerclient"
	"github.com/lynn/porcelain/chimera/internal/config"
	"github.com/lynn/porcelain/internal/operatorapi"
)

// BuildResponse assembles GET /api/ui/state from runtime config and live probes.
func BuildResponse(ctx context.Context, r *http.Request, res *config.Resolved, rt *gruntime.Runtime, log *slog.Logger) operatorapi.StateResponse {
	client := apirut.BrokerAdminClient(rt)
	configured, listOK := brokeradmin.ListConfiguredProviders(ctx, client)
	probeNames := apirut.ConfiguredProviderIDsResolved(ctx, client, configured, listOK)
	provOut := make(map[string]operatorapi.StateProviderEntry, len(probeNames))
	for _, name := range probeNames {
		provOut[name] = probeStateProvider(ctx, client, name)
	}

	routeBase := ""
	routingPolicyYAML := ""
	if p := strings.TrimSpace(res.RoutingPolicyPath); p != "" {
		routeBase = filepath.Base(p)
		if b, err := os.ReadFile(p); err == nil {
			routingPolicyYAML = string(b)
		}
	}
	rm, trAt, trErr := rt.ToolRouterLast()
	chimeraBrokerURL := strings.TrimSuffix(res.UpstreamBaseURL, "/")
	chimeraBrokerOK, _, chimeraBrokerDetail := brokerclient.ProbeHealth(ctx, res.HealthUpstreamURL, rt.UpstreamAPIKey(), gruntime.HealthTimeout(res), log)
	chimeraBrokerState := "down"
	if chimeraBrokerOK {
		chimeraBrokerState = "up"
	}
	vectorstoreURL := strings.TrimSuffix(res.RAG.QdrantURL, "/")
	vectorstoreState := "disabled"
	if res.RAG.Enabled {
		if rt.RAG() == nil {
			vectorstoreState = "unavailable"
		} else if err := rt.RAG().StoreHealth(ctx); err != nil {
			vectorstoreState = "down"
		} else {
			vectorstoreState = "up"
		}
	}
	idxScope := res.IndexerSupervisedEnabled && (res.RAG.Enabled || res.IndexerSupervisedStartWhenRAGDisabled)
	indexerWorker := "disabled"
	indexerDeclaredState := ""
	indexerLastHeartbeatAt := ""
	indexerLastLogAt := ""
	indexerDetail := ""
	if res.IndexerSupervisedEnabled {
		if !idxScope {
			indexerWorker = "not_running_out_of_scope"
		} else {
			indexerWorker = "starting"
			idxSt := rt.IndexerSupervisorStatus()
			if strings.TrimSpace(idxSt.WorkerState) != "" {
				indexerWorker = strings.TrimSpace(idxSt.WorkerState)
			}
			indexerDeclaredState = strings.TrimSpace(idxSt.LastState)
			indexerLastHeartbeatAt = apirut.FormatRFC3339OrEmpty(idxSt.LastHeartbeatAt)
			indexerLastLogAt = apirut.FormatRFC3339OrEmpty(idxSt.LastLogAt)
			indexerDetail = strings.TrimSpace(idxSt.LastError)
			if indexerWorker == "" {
				indexerWorker = "unknown"
			}
		}
	}
	overviewState := "ok"
	if chimeraBrokerState != "up" || (res.RAG.Enabled && vectorstoreState != "up") {
		overviewState = "degraded"
	}
	if res.IndexerSupervisedEnabled && idxScope {
		switch indexerWorker {
		case "down", "degraded":
			overviewState = "degraded"
		case "up":
		default:
			if overviewState == "ok" {
				overviewState = "monitor"
			}
		}
	}

	return operatorapi.StateResponse{
		Gateway: operatorapi.GatewayState{
			Semver:                        res.Semver,
			VirtualModelID:                res.VirtualModelID,
			PublicBaseURL:                 apirut.PublicGatewayBase(r),
			TokenHint:                     "Paste the same gateway token you used to sign in.",
			FilterFreeTierModels:          res.FilterFreeTierModels,
			FallbackChain:                 res.FallbackChain,
			RoutingPolicyBasename:         routeBase,
			RouterModels:                  res.RouterModels,
			ToolRouterEnabled:             res.ToolRouterEnabled,
			ToolRouterConfidenceThreshold: res.ToolRouterConfidenceThreshold,
			ToolRouterLastModel:           rm,
			ToolRouterLastError:           trErr,
			ToolRouterLastAt:              apirut.FormatRFC3339OrEmpty(trAt),
			RoutingPolicyYAML:             routingPolicyYAML,
			ServiceOverview: operatorapi.ServiceOverview{
				OverallState: overviewState,
				Gateway:      operatorapi.ServiceState{State: "up"},
				ChimeraBroker: operatorapi.ServiceEndpointState{
					State:  chimeraBrokerState,
					URL:    chimeraBrokerURL,
					Detail: chimeraBrokerDetail,
				},
				ChimeraVectorstore: operatorapi.VectorstoreState{
					Enabled: res.RAG.Enabled,
					State:   vectorstoreState,
					URL:     vectorstoreURL,
				},
				ChimeraIndexer: operatorapi.IndexerOverviewState{
					Enabled:            res.IndexerSupervisedEnabled,
					InScope:            idxScope,
					Worker:             indexerWorker,
					State:              indexerDeclaredState,
					LastHeartbeatAt:    indexerLastHeartbeatAt,
					LastLogAt:          indexerLastLogAt,
					Detail:             indexerDetail,
					SupervisionSignals: "process_liveness + indexer.state heartbeat",
				},
				RefreshedAt: time.Now().UTC().Format(time.RFC3339),
			},
			IndexerSupervisedConfigPath: res.IndexerSupervisedConfigPath,
			IndexerSupervisedEnabled:    res.IndexerSupervisedEnabled,
			OperatorSQLitePath:          res.OperatorSQLitePath,
			OperatorStoreOpen:           rt.OperatorStore() != nil,
		},
		Providers:             provOut,
		ConfiguredProviderIDs: append([]string(nil), probeNames...),
	}
}

func probeStateProvider(ctx context.Context, client *brokeradmin.Client, name string) operatorapi.StateProviderEntry {
	entry := operatorapi.StateProviderEntry{Provider: name}
	b, st, err, _ := brokeradmin.GetProviderForProbe(ctx, client, name)
	if err != nil {
		entry.OK = false
		entry.Error = err.Error()
		return entry
	}
	if brokeradmin.IsProviderMissingGET(st, b) {
		entry.OK = true
		entry.KeyConfigured = false
		entry.KeyHint = ""
		entry.Keys = []operatorapi.ProviderKeyEntry{}
		if name == "ollama" {
			entry.OllamaBaseURL = ""
		}
		return entry
	}
	entry.HTTPStatus = st
	if st < 200 || st >= 300 {
		entry.OK = false
		entry.Error = strings.TrimSpace(string(b))
		if entry.Error == "" {
			entry.Error = http.StatusText(st)
		}
		return entry
	}
	sum, serr := brokeradmin.SummarizeProvider(name, b)
	if serr != nil {
		entry.OK = false
		entry.Error = serr.Error()
		return entry
	}
	keyRows, _ := brokeradmin.SummarizeProviderKeys(name, b)
	entry.OK = true
	entry.KeyHint = sum.KeyHint
	entry.KeyConfigured = sum.KeyConfigured
	entry.Keys = make([]operatorapi.ProviderKeyEntry, len(keyRows))
	for i, k := range keyRows {
		entry.Keys[i] = operatorapi.ProviderKeyEntry{
			Name:          k.Name,
			KeyHint:       k.KeyHint,
			KeyConfigured: k.KeyConfigured,
		}
	}
	if sum.OllamaBaseURL != "" {
		entry.OllamaBaseURL = sum.OllamaBaseURL
	}
	return entry
}
