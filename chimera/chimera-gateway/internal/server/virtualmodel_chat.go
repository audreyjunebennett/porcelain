package server

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/chat"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/operatorstore"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/rag"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/routing"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/transform"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/vectorstore"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/virtualmodel"
	"github.com/lynn/porcelain/chimera/internal/config"
	"github.com/lynn/porcelain/internal/naming"
)

func virtualModelsForCatalog(rt *Runtime, principalID string) []*virtualmodel.Resolved {
	reg := rt.VirtualModels()
	if reg != nil {
		return reg.ListCatalog(principalID)
	}
	return nil
}

func openAIModelEntry(id, description string) map[string]any {
	entry := map[string]any{
		"id":       id,
		"object":   "model",
		"created":  time.Now().Unix(),
		"owned_by": "chimera",
	}
	if strings.TrimSpace(description) != "" {
		entry["description"] = description
	}
	return entry
}

func prependVirtualModelsToCatalog(data []any, rt *Runtime, principalID string, legacy *config.Resolved) []any {
	vms := virtualModelsForCatalog(rt, principalID)
	if len(vms) > 0 {
		out := make([]any, 0, len(vms)+len(data))
		for _, vm := range vms {
			out = append(out, openAIModelEntry(vm.ModelID, vm.Description))
		}
		return append(out, data...)
	}
	if legacy != nil && legacy.VirtualModelID != "" {
		return append([]any{openAIModelEntry(legacy.VirtualModelID, "")}, data...)
	}
	return data
}

type virtualModelChatContext struct {
	vm           *virtualmodel.Resolved
	fallback     []string
	toolEnabled  bool
	routerModels []string
	toolThresh   float64
	useLegacyPol bool
}

func resolveVirtualModelChat(rt *Runtime, clientModel, principalID string, res *config.Resolved) (*virtualModelChatContext, int, map[string]any) {
	reg := rt.VirtualModels()
	if reg != nil {
		vm, err := reg.Resolve(clientModel, principalID)
		if err == nil {
			return &virtualModelChatContext{
				vm:           vm,
				fallback:     vm.FallbackChain,
				toolEnabled:  vm.ToolRouterEnabled,
				routerModels: vm.RouterModels,
				toolThresh:   vm.ToolRouterConfidence,
			}, 0, nil
		}
		if errors.Is(err, virtualmodel.ErrForbidden) {
			return nil, http.StatusForbidden, map[string]any{
				"error": map[string]any{"message": "Virtual model not accessible", "type": "invalid_request"},
			}
		}
		if store := rt.OperatorStore(); store != nil && errors.Is(err, virtualmodel.ErrNotFound) {
			row, dbErr := store.GetVirtualModelByModelID(context.Background(), clientModel)
			if dbErr == nil && row != nil {
				if !row.Enabled {
					return nil, http.StatusNotFound, map[string]any{
						"error": map[string]any{"message": "Virtual model is disabled", "type": "invalid_request"},
					}
				}
				if row.Visibility == operatorstore.VisibilityPrivate &&
					row.CreatedByPrincipalID != "" && row.CreatedByPrincipalID != principalID {
					return nil, http.StatusForbidden, map[string]any{
						"error": map[string]any{"message": "Virtual model not accessible", "type": "invalid_request"},
					}
				}
			}
		}
	}
	if res != nil && clientModel == res.VirtualModelID {
		return &virtualModelChatContext{
			vm: &virtualmodel.Resolved{
				ModelID:              res.VirtualModelID,
				FallbackChain:        res.FallbackChain,
				ToolRouterEnabled:    res.ToolRouterEnabled,
				RouterModels:         res.RouterModels,
				ToolRouterConfidence: res.ToolRouterConfidenceThreshold,
			},
			fallback:     res.FallbackChain,
			toolEnabled:  res.ToolRouterEnabled,
			routerModels: res.RouterModels,
			toolThresh:   res.ToolRouterConfidenceThreshold,
			useLegacyPol: true,
		}, 0, nil
	}
	return nil, 0, nil
}

func routeLogWithVirtualModel(routeLog *slog.Logger, virtualModelID string) *slog.Logger {
	if routeLog == nil || virtualModelID == "" {
		return routeLog
	}
	return routeLog.With("virtual_model_id", virtualModelID)
}

func handleVirtualModelChat(
	ctx context.Context,
	w http.ResponseWriter,
	rt *Runtime,
	res *config.Resolved,
	pol *routing.Policy,
	vmCtx *virtualModelChatContext,
	raw map[string]json.RawMessage,
	stream bool,
	skipToolRouter bool,
	headerThresh float64,
	routeLog *slog.Logger,
	cid string,
	turnIdx int,
	rid string,
	sessTenant string,
	proj string,
	flav string,
	apiKey string,
	rtDur time.Duration,
	chatOpts *chat.ProxyOpts,
) bool {
	vm := vmCtx.vm
	if vm == nil {
		return false
	}
	virtualID := vm.ModelID
	routeLog = routeLogWithVirtualModel(routeLog, virtualID)

	th := vmCtx.toolThresh
	if headerThresh > 0 {
		th = headerThresh
	}
	raw, trSum := transform.ApplyToolRouter(ctx, raw, transform.Config{
		Enabled:      vmCtx.toolEnabled && !skipToolRouter,
		RouterModels: vmCtx.routerModels,
		Threshold:    th,
		BaseURL:      res.UpstreamBaseURL,
		APIKey:       apiKey,
		HTTPTimeout:  rtDur,
		Log:          routeLog,
		OnAttempt: func(model string, err error) {
			rt.NoteToolRouterAttempt(model, err)
		},
	})
	if routeLog != nil && trSum.Ran {
		errStr := ""
		if trSum.Err != nil {
			errStr = trSum.Err.Error()
			if len(errStr) > 300 {
				errStr = errStr[:300] + "…"
			}
		}
		routeLog.Debug("conversation tool router", "msg", naming.MsgConversationToolRouter,
			"tools_before", trSum.ToolsBefore, "tools_after", trSum.ToolsAfter,
			"router_model", trSum.RouterModel, "virtual_model_id", virtualID,
			"err", errStr, "timeline_kind", naming.TimelineKindBroker)
	}

	coords := vectorstore.Coords{TenantID: sessTenant, ProjectID: proj, FlavorID: flav}
	collection := vectorstore.CollectionName(coords)
	if !res.RAG.Enabled || rt.RAG() == nil {
		if routeLog != nil {
			routeLog.Debug("conversation RAG skipped", "msg", naming.MsgConversationRagSkipped,
				"reason", "disabled", "virtual_model_id", virtualID, "timeline_kind", naming.TimelineKindVectorstore)
		}
	} else if q := rag.LastUserText(raw["messages"]); strings.TrimSpace(q) == "" {
		if routeLog != nil {
			routeLog.Debug("conversation RAG skipped", "msg", naming.MsgConversationRagSkipped,
				"reason", "empty_query", "virtual_model_id", virtualID, "timeline_kind", naming.TimelineKindVectorstore)
		}
	} else {
		hits, rerr := rt.RAG().Retrieve(ctx, rag.RetrieveRequest{
			Coords: coords, Query: q, RequestID: rid, ConversationID: cid, TurnIndex: turnIdx, LifecycleLog: routeLog,
		})
		if rerr != nil {
			if routeLog != nil {
				routeLog.Warn("rag retrieve failed; proceeding without context", "msg", "rag.retrieve.error", "err", rerr,
					"virtual_model_id", virtualID, "timeline_kind", naming.TimelineKindVectorstore)
			}
		} else if ctxBlock := rag.FormatRetrievedContext(hits); ctxBlock != "" {
			rag.InjectSystemMessage(raw, ctxBlock)
			if routeLog != nil {
				routeLog.Info("conversation RAG attached", "msg", naming.MsgConversationRagAttached,
					"virtual_model_id", virtualID, "tenant", coords.TenantID, "project", coords.ProjectID,
					"flavor", coords.FlavorID, "hits", len(hits), "collection", collection,
					"timeline_kind", naming.TimelineKindVectorstore)
			}
		}
	}

	emitConversationRequestWitness(routeLog, res, raw)

	var initial string
	if vmCtx.useLegacyPol && pol != nil {
		initial, _ = pol.PickInitialModel(raw, vmCtx.fallback, virtualID)
	} else {
		initial, _ = virtualmodel.PickInitialModel(vm, raw, routeLog)
	}
	if initial == "" {
		if routeLog != nil {
			routeLog.Warn("conversation errored", "msg", naming.MsgConversationErrored,
				"statusCode", http.StatusServiceUnavailable, "errorType", "gateway_config",
				"virtual_model_id", virtualID, "timeline_kind", naming.TimelineKindBroker)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"message": "Could not resolve an initial upstream model for the virtual model (check routing policy and fallback chain).",
				"type":    "gateway_config",
			},
		})
		return true
	}
	if routeLog != nil {
		routeLog.Info("chat routing resolved", "msg", "chat.routing.resolved",
			"virtual_model_id", virtualID, "clientModel", virtualID, "upstreamModel", initial,
			"timeline_kind", naming.TimelineKindBroker)
	}
	chat.WithVirtualModelFallback(ctx, w, initial, vmCtx.fallback, res.UpstreamBaseURL, apiKey, stream, raw,
		chatTimeout(res), routeLog, rt.Metrics(), rt.LimitsGuard(), chatOpts)
	return true
}
