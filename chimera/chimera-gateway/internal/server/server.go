package server

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/assets"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/chat"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/conversationmerge"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/conversationwitness"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/gwhttp"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/rag"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/adminui"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/catalog"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/indexerapi"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/ingest"
	gruntime "github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/runtime"
	"github.com/lynn/porcelain/chimera/internal/brokerclient"
	"github.com/lynn/porcelain/chimera/internal/config"
	"github.com/lynn/porcelain/chimera/internal/platform/requestid"
	"github.com/lynn/porcelain/internal/naming"

	"github.com/google/uuid"
)

const maxBodyBytes = 25 * 1024 * 1024

// publicGatewayURL is a browser-friendly base URL for this gateway (loopback when listening on all interfaces).
func publicGatewayURL(res *config.Resolved, overlay *StatusOverlay) string {
	if res == nil {
		return "http://127.0.0.1:3000"
	}
	listen := res.ListenAddr()
	if overlay != nil && overlay.EffectiveListen != "" {
		listen = overlay.EffectiveListen
	}
	host, port, err := net.SplitHostPort(listen)
	if err != nil {
		host := strings.TrimSpace(res.ListenHost)
		if host == "" || host == "0.0.0.0" || host == "::" {
			host = "127.0.0.1"
		}
		if strings.Contains(host, ":") && !strings.HasPrefix(host, "[") {
			return fmt.Sprintf("http://[%s]:%d", host, res.ListenPort)
		}
		return fmt.Sprintf("http://%s:%d", host, res.ListenPort)
	}
	if host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	if strings.Contains(host, ":") && !strings.HasPrefix(host, "[") {
		return fmt.Sprintf("http://[%s]:%s", host, port)
	}
	return fmt.Sprintf("http://%s:%s", host, port)
}

// mergedUpstreamModelStats returns merged model count (virtual + filtered upstream) and distinct provider
// prefixes from upstream model ids, when the upstream /v1/models call succeeds.
//
// Thin wrapper over [buildCatalogSnapshot] that preserves the historical (count, providers, ok)
// shape used by the gateway home page and `chat.chimera-broker.available_models` log emission. Side
// effect: emits the slog line, matching the prior behavior. Prefer [RefreshAvailableModels]
// when you also want the snapshot cached on the runtime.
func mergedUpstreamModelStats(ctx context.Context, res *config.Resolved, apiKey string, timeout time.Duration, log *slog.Logger) (count int, providers []string, ok bool) {
	snap := catalog.BuildSnapshot(ctx, res, apiKey, timeout, log)
	catalog.EmitAvailableModelsLog(snap, log)
	if snap == nil || !snap.OK {
		return 0, nil, false
	}
	return snap.CatalogModelCount, snap.Providers, true
}

var gatewayIndexTmpl = template.Must(template.New("gatewayIndex").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8"/>
  <meta name="viewport" content="width=device-width, initial-scale=1"/>
  <title>Chimera Gateway — Status</title>
  <style>
    body {
      font-family: system-ui, sans-serif; max-width: 48rem; margin: 1.5rem auto 2.5rem; padding: 0 1rem;
      line-height: 1.55; color: #1a1a1a;
      position: relative;
      min-height: 100vh;
    }
    /* Large faint brand mark — behind content, fixed to the right */
    body::before {
      content: "";
      position: fixed;
      right: -4%;
      top: 50%;
      transform: translateY(-50%);
      width: min(52vw, 24rem);
      height: min(52vw, 24rem);
      max-height: 85vh;
      background: url("/assets/icon.png") no-repeat center right;
      background-size: contain;
      opacity: 0.07;
      pointer-events: none;
      z-index: 0;
    }
    body > * { position: relative; z-index: 1; }
    h1 { font-size: 1.45rem; margin-bottom: 0.25rem; }
    h2 { font-size: 1.05rem; margin-top: 1.75rem; margin-bottom: 0.65rem; color: #222; }
    .subtitle { color: #555; margin-top: 0; font-size: 0.95rem; }
    .ok { color: #0d6832; font-weight: 600; }
    .err { color: #a40000; font-weight: 600; }
    .muted { color: #666; font-weight: 500; }
    dl { margin: 0; display: grid; grid-template-columns: 11rem 1fr; gap: 0.35rem 1rem; align-items: baseline; }
    dt { color: #444; font-size: 0.9rem; }
    dd { margin: 0; }
    a { color: #0b57d0; word-break: break-all; }
    code { background: #f4f4f4; padding: 0.12em 0.35em; border-radius: 4px; font-size: 0.88em; }
    .block { margin-top: 0.5rem; }
  </style>
</head>
<body>
  <h1>Chimera Gateway</h1>
  <p class="subtitle">Site status</p>

  <h2>Version</h2>
  <dl>
    <dt>Gateway version</dt><dd><code>{{.Semver}}</code></dd>
    <dt>Virtual model</dt><dd><code>{{.VirtualModel}}</code></dd>
  </dl>

  <h2>Services</h2>
  <dl>
    <dt>Chimera (this gateway)</dt>
    <dd><span class="ok">up</span> · <a href="{{.GatewayURL}}">{{.GatewayURL}}</a></dd>
    <dt>Broker</dt>
    <dd><span class="{{.BrokerClass}}">{{if .BrokerOK}}up{{else}}down{{end}}</span> · <a href="{{.BrokerURL}}">{{.BrokerURL}}</a></dd>
    <dt>Vector Store</dt>
    <dd><span class="{{.VectorstoreClass}}">{{.VectorstoreState}}</span> · <a href="{{.VectorstoreURL}}">{{.VectorstoreURL}}</a></dd>
    <dt>Indexer (supervised)</dt>
    <dd><span class="{{.IndexerWorkerClass}}">{{.IndexerWorker}}</span> · config: <span class="muted">{{.IndexerConfig}}</span></dd>
  </dl>

  <h2>Configuration</h2>
  <dl>
    <dt>Gateway tokens</dt><dd>{{.TokensCount}} configured</dd>
    <dt>Metrics</dt><dd>{{if .MetricsEnabled}}enabled{{else}}disabled{{end}}</dd>
    <dt>Conversation merge</dt><dd>{{if .ConversationMerge}}enabled{{else}}disabled{{end}}</dd>
    <dt>Broker model providers</dt><dd>{{.Providers}}</dd>
    <dt>Models available</dt><dd>{{.ModelCount}} <span class="muted">(merged list: virtual + upstream)</span></dd>
  </dl>
</body>
</html>`))

// NewMux builds the v0.1 HTTP surface (src/server.ts parity). overlay configures GET /status;
// pass nil in tests; production passes listen address and optional supervisor info.
// ui enables operator /ui and /api/ui routes; pass nil to disable (tests).
func NewMux(rt *Runtime, log *slog.Logger, overlay *StatusOverlay, ui *UIOptions) http.Handler {
	configureAdminUIListenForEmbed(rt, overlay)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /assets/icon.png", func(w http.ResponseWriter, r *http.Request) {
		if len(assets.IconPNG) == 0 {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "public, max-age=86400")
		_, _ = w.Write(assets.IconPNG)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if ui != nil {
			http.Redirect(w, r, "/ui", http.StatusFound)
			return
		}
		rt.Sync()
		res, tokStore, _ := rt.Snapshot()
		ctx := r.Context()
		apiKey := rt.UpstreamAPIKey()

		gwURL := publicGatewayURL(res, overlay)
		chimeraBrokerURL := strings.TrimSuffix(res.UpstreamBaseURL, "/")
		chimeraBrokerOK, _, _ := brokerclient.ProbeHealth(ctx, res.HealthUpstreamURL, apiKey, healthTimeout(res), log)
		chimeraBrokerClass := "err"
		if chimeraBrokerOK {
			chimeraBrokerClass = "ok"
		}

		vectorstoreURL := strings.TrimSuffix(res.RAG.QdrantURL, "/")
		vectorstoreState := "disabled (RAG off)"
		vsClass := "muted"
		if res.RAG.Enabled {
			if rt.RAG() == nil {
				vectorstoreState = "unavailable"
				vsClass = "err"
			} else if err := rt.RAG().StoreHealth(ctx); err != nil {
				vectorstoreState = "down"
				vsClass = "err"
			} else {
				vectorstoreState = "up"
				vsClass = "ok"
			}
		}

		idxScope := res.IndexerSupervisedEnabled && (res.RAG.Enabled || res.IndexerSupervisedStartWhenRAGDisabled)
		idxConfig := "disabled"
		idxWorker := "—"
		if res.IndexerSupervisedEnabled {
			idxConfig = "enabled"
			if !idxScope {
				idxWorker = "not running (out of scope)"
			} else if overlay != nil && overlay.Supervisor != nil {
				if overlay.Supervisor.IndexerSupervised {
					idxWorker = "up"
				} else {
					idxWorker = "down"
				}
			} else {
				idxWorker = "unknown"
			}
		}
		idxWorkerClass := "muted"
		switch idxWorker {
		case "up":
			idxWorkerClass = "ok"
		case "down":
			idxWorkerClass = "err"
		}

		modelCount := "unavailable"
		providers := "—"
		if n, provs, ok := mergedUpstreamModelStats(ctx, res, apiKey, healthTimeout(res), log); ok {
			modelCount = strconv.Itoa(n)
			if len(provs) > 0 {
				providers = strings.Join(provs, ", ")
			} else {
				providers = "(none)"
			}
		} else if apiKey == "" {
			providers = "set chimera-broker API key to query catalog"
		}

		data := struct {
			Semver, VirtualModel string
			GatewayURL           string
			BrokerURL            string
			BrokerOK             bool
			BrokerClass          string
			VectorstoreURL       string
			VectorstoreState     string
			VectorstoreClass     string
			IndexerConfig        string
			IndexerWorker        string
			IndexerWorkerClass   string
			TokensCount          int
			MetricsEnabled       bool
			ConversationMerge    bool
			Providers            string
			ModelCount           string
		}{
			Semver:             res.Semver,
			VirtualModel:       res.VirtualModelID,
			GatewayURL:         gwURL,
			BrokerURL:          chimeraBrokerURL,
			BrokerOK:           chimeraBrokerOK,
			BrokerClass:        chimeraBrokerClass,
			VectorstoreURL:     vectorstoreURL,
			VectorstoreState:   vectorstoreState,
			VectorstoreClass:   vsClass,
			IndexerConfig:      idxConfig,
			IndexerWorker:      idxWorker,
			IndexerWorkerClass: idxWorkerClass,
			TokensCount:        tokStore.Count(),
			MetricsEnabled:     res.MetricsEnabled,
			ConversationMerge:  res.ConversationMerge.Enabled,
			Providers:          providers,
			ModelCount:         modelCount,
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = gatewayIndexTmpl.Execute(w, data)
	})

	// /healthz is process liveness only (HTTP up). The chimera-gateway wrapper uses it for
	// startup readiness; /health remains the dependency check (broker + vectorstore).
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
	})

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		rt.Sync()
		res, _, _ := rt.Snapshot()
		apiKey := rt.UpstreamAPIKey()
		ctx := r.Context()
		ok, st, detail := brokerclient.ProbeHealth(ctx, res.HealthUpstreamURL, apiKey, healthTimeout(res), log)
		brokerCheck := map[string]any{
			"ok":     ok,
			"status": st,
		}
		if detail != "" {
			brokerCheck["detail"] = detail
		}
		checks := map[string]any{
			"broker": brokerCheck,
		}
		degraded := !ok
		if res.RAG.Enabled && rt.RAG() != nil {
			qErr := rt.RAG().StoreHealth(ctx)
			qCheck := map[string]any{"ok": qErr == nil}
			if qErr != nil {
				qCheck["detail"] = qErr.Error()
				degraded = true
			}
			checks["vectorstore"] = qCheck
		}
		if degraded {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"degraded": true,
				"status":   "degraded",
				"checks":   checks,
			})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "ok",
			"checks": checks,
		})
	})

	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		handleStatus(w, r, rt, log, overlay)
	})

	mux.HandleFunc("/ui/models", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		rt.Sync()
		res, _, _ := rt.Snapshot()
		writeMergedModelsResponse(w, r.Context(), rt, res, "", rt.UpstreamAPIKey(), healthTimeout(res), log)
	})

	mux.HandleFunc("/v1/models", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		handleV1Models(w, r, rt, log)
	})

	mux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		handleV1Chat(w, r, rt, log)
	})

	mux.HandleFunc("/v1/ingest", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		ingest.HandleV1(w, r, rt, log)
	})
	mux.HandleFunc("/v1/ingest/session/", func(w http.ResponseWriter, r *http.Request) {
		gruntime.HandleIngestSessionTail(w, r, rt, log)
	})
	mux.HandleFunc("/v1/ingest/session", func(w http.ResponseWriter, r *http.Request) {
		gruntime.HandleIngestSessionStart(w, r, rt, log)
	})

	mux.HandleFunc("/v1/indexer/config", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		indexerapi.HandleConfig(w, r, rt, log)
	})
	mux.HandleFunc("/v1/indexer/workspaces", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		indexerapi.HandleWorkspaces(w, r, rt, log)
	})
	mux.HandleFunc("/v1/indexer/storage/health", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		indexerapi.HandleHealth(w, r, rt, log)
	})
	mux.HandleFunc("/v1/indexer/storage/stats", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		indexerapi.HandleStats(w, r, rt, log)
	})
	mux.HandleFunc("/v1/indexer/corpus/inventory", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		indexerapi.HandleCorpusInventory(w, r, rt, log)
	})

	adminui.Register(mux, rt, log, ui)

	return requestid.Middleware(loggingMiddleware(log, mux))
}

func handleV1Models(w http.ResponseWriter, r *http.Request, rt *Runtime, log *slog.Logger) {
	rt.Sync()
	res, tokStore, _ := rt.Snapshot()
	token := gwhttp.BearerToken(r.Header.Get("Authorization"))
	sess := tokStore.Validate(token)
	if token == "" || sess == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{"message": "Unauthorized", "type": "invalid_api_key"},
		})
		return
	}
	writeMergedModelsResponse(w, r.Context(), rt, res, sess.TenantID, rt.UpstreamAPIKey(), healthTimeout(res), log)
}

// ensureOpenAIModelListItems sets object/created on each upstream model. BiFrost often omits
// OpenAI's required "object":"model" and "created" on many entries; strict clients (e.g. VS Code Continue)
// may drop or fail to display them without these fields.
func ensureOpenAIModelListItems(data []any) {
	for _, raw := range data {
		m, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if o, ok := m["object"].(string); !ok || strings.TrimSpace(o) == "" {
			m["object"] = "model"
		}
		if _, has := m["created"]; !has {
			m["created"] = int64(0)
		}
	}
}

// writeMergedModelsResponse lists upstream GET /v1/models, prepends virtual models, and writes OpenAI-style JSON.
func writeMergedModelsResponse(w http.ResponseWriter, ctx context.Context, rt *Runtime, res *config.Resolved, principalID, apiKey string, timeout time.Duration, log *slog.Logger) {
	w.Header().Set("Content-Type", "application/json")
	if apiKey == "" {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"message": "Missing chimera-broker API key (set " + res.UpstreamAPIKeyEnv + " or broker.api_key in " + naming.GatewayConfigFileTarget + ")",
				"type":    "gateway_config",
			},
		})
		return
	}
	st, body, ok := brokerclient.FetchOpenAIModels(ctx, res.UpstreamBaseURL, apiKey, timeout, log)
	if !ok {
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"message": "Failed to list models from upstream",
				"type":    "gateway_upstream",
				"status":  st,
			},
		})
		return
	}
	var list map[string]any
	if err := json.Unmarshal(body, &list); err != nil {
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"message": "Invalid models response from upstream",
				"type":    "gateway_upstream",
			},
		})
		return
	}
	data, _ := list["data"].([]any)
	if data == nil {
		data = []any{}
	}
	ensureOpenAIModelListItems(data)
	data = catalog.FilterOpenAIModelDataByFreeTier(data, res)
	out := prependVirtualModelsToCatalog(data, rt, principalID, res)
	_ = json.NewEncoder(w).Encode(map[string]any{"object": "list", "data": out})
}

func emitConversationRequestWitness(routeLog *slog.Logger, res *config.Resolved, raw map[string]json.RawMessage) {
	if routeLog == nil {
		return
	}
	conversationwitness.LogRequestWitness(routeLog, raw)
	if res == nil || !res.ShouldEmitPayloadSample() {
		return
	}
	b, err := json.Marshal(raw)
	if err != nil || len(b) == 0 {
		return
	}
	conversationwitness.LogPayloadSample(routeLog, true, res.WitnessSampleMaxRunes(), "request", b)
}

func chatRouteLogger(log *slog.Logger, rid, cid, tenant string, turnIndex int) *slog.Logger {
	if log == nil {
		return nil
	}
	return log.With(
		"request_id", rid,
		"conversation_id", cid,
		"service", "gateway",
		"principal_id", tenant,
		"turn_index", turnIndex,
	)
}

func lifecycleErrorType(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "invalid_request"
	case http.StatusUnauthorized:
		return "invalid_api_key"
	case http.StatusTooManyRequests:
		return "gateway_provider_limits"
	case http.StatusServiceUnavailable:
		return "gateway_config"
	default:
		return "gateway_upstream"
	}
}

func attachConversationDelivery(routeLog *slog.Logger, opts **chat.ProxyOpts) {
	dfn := func(st int, stream bool, nb int64, elapsedMs int64) {
		if routeLog == nil {
			return
		}
		if st >= 200 && st < 300 {
			routeLog.Info("conversation delivered", "msg", naming.MsgConversationDelivered,
				"statusCode", st, "stream", stream, "bytes", nb, "total_ms", elapsedMs,
				"timeline_kind", naming.TimelineKindBroker)
			return
		}
		routeLog.Warn("conversation errored", "msg", naming.MsgConversationErrored,
			"statusCode", st, "errorType", lifecycleErrorType(st), "timeline_kind", naming.TimelineKindBroker)
	}
	if *opts == nil {
		*opts = &chat.ProxyOpts{OnChatDelivery: dfn}
		return
	}
	(*opts).OnChatDelivery = dfn
}

func handleV1Chat(w http.ResponseWriter, r *http.Request, rt *Runtime, log *slog.Logger) {
	rt.Sync()
	res, tokStore, pol := rt.Snapshot()
	token := gwhttp.BearerToken(r.Header.Get("Authorization"))
	sess := tokStore.Validate(token)
	if token == "" || sess == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{"message": "Unauthorized", "type": "invalid_api_key"},
		})
		return
	}
	apiKey := rt.UpstreamAPIKey()
	if apiKey == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"message": "Missing chimera-broker API key (set " + res.UpstreamAPIKeyEnv + " or broker.api_key in " + naming.GatewayConfigFileTarget + ")",
				"type":    "gateway_config",
			},
		})
		return
	}

	rid := requestid.FromContext(r.Context())

	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	var raw map[string]json.RawMessage
	if err := dec.Decode(&raw); err != nil || raw == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{"message": "Expected JSON body", "type": "invalid_request"},
		})
		return
	}
	flowStart := time.Now()

	var stream bool
	if s, ok := raw["stream"]; ok {
		_ = json.Unmarshal(s, &stream)
	}

	var clientModel string
	if m, ok := raw["model"]; ok {
		_ = json.Unmarshal(m, &clientModel)
	}

	ctx := r.Context()
	skipToolRouter := strings.EqualFold(strings.TrimSpace(r.Header.Get(naming.HeaderToolRouterTarget)), "skip")
	th := res.ToolRouterConfidenceThreshold
	if h := strings.TrimSpace(r.Header.Get(naming.HeaderToolConfidenceThresholdTarget)); h != "" {
		if v, err := strconv.ParseFloat(h, 64); err == nil {
			th = v
		}
	}
	rtDur := time.Duration(res.ChatTimeoutMs) * time.Millisecond
	if rtDur > 60*time.Second {
		rtDur = 60 * time.Second
	}
	if rtDur < 5*time.Second {
		rtDur = 5 * time.Second
	}
	headerCID := ingest.OptionalConversationIDFromHeader(r)
	proj := ingest.ResolveProject(r.Header.Get(ingest.HeaderProject), res.RAG.DefaultProject)
	flav := ingest.ResolveFlavor(r.Header.Get(ingest.HeaderFlavor), res.RAG.DefaultFlavor)
	lastUser := rag.LastUserText(raw["messages"])

	var mergeSvc *conversationmerge.Service
	if os := rt.OperatorStore(); os != nil {
		mergeSvc = conversationmerge.NewService(res.ConversationMerge, os.DB(), res.UpstreamBaseURL, apiKey, res.RAG, log)
	}

	incomingFP := strings.TrimSpace(r.Header.Get(headerRequestFingerprint))

	var cid string
	var cidSource string
	var mergeTurn int

	switch {
	case headerCID != "":
		cid = headerCID
		cidSource = "header"
	case mergeSvc != nil:
		out, err := mergeSvc.Resolve(ctx, conversationmerge.ResolveInput{
			TenantID:             sess.TenantID,
			ProjectID:            proj,
			FlavorID:             flav,
			LastUserText:         lastUser,
			IncomingFingerprint:  incomingFP,
			ClientConversationID: "",
			RequestID:            rid,
			NextTurnIndex:        rt.NextChatTurnIndex,
		})
		if err != nil && log != nil {
			log.With("request_id", rid, "service", "gateway", "principal_id", sess.TenantID).
				Debug("conversation merge resolve failed", "msg", naming.MsgConversationMergeResolveFailed, "err", err)
		}
		if len(out.DedupJSON) > 0 {
			cid = out.ConversationID
			turnIdx := out.TurnIndex
			if turnIdx <= 0 {
				turnIdx = rt.NextChatTurnIndex(cid)
			}
			dedupLog := chatRouteLogger(log, rid, cid, sess.TenantID, turnIdx)
			w.Header().Set(headerConversationID, cid)
			if dedupLog != nil {
				dedupRecv := []any{
					"msg", naming.MsgConversationReceived,
					"clientModel", clientModel, "stream", stream, "tenant", sess.TenantID,
					"project", proj, "flavor", flav, "cid_source", "merge", "timeline_kind", naming.TimelineKindBroker,
				}
				if mc := conversationwitness.RequestMessageCount(raw); mc > 0 {
					dedupRecv = append(dedupRecv, "message_count", mc)
				}
				dedupLog.Info("conversation received", dedupRecv...)
				emitConversationRequestWitness(dedupLog, res, raw)
			}
			if fp := mergeSvc.RollingFingerprint(ctx, cid); fp != "" {
				w.Header().Set(headerRollingFingerprint, fp)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			n, _ := w.Write(out.DedupJSON)
			if dedupLog != nil {
				conversationwitness.LogResponseWitness(dedupLog, false, out.DedupJSON)
				if res.ShouldEmitPayloadSample() {
					conversationwitness.LogPayloadSample(dedupLog, true, res.WitnessSampleMaxRunes(), "response", out.DedupJSON)
				}
				dedupLog.Info("conversation delivered", "msg", naming.MsgConversationDelivered,
					"statusCode", http.StatusOK, "stream", false, "bytes", int64(n),
					"total_ms", time.Since(flowStart).Milliseconds(), "timeline_kind", naming.TimelineKindBroker)
			}
			return
		}
		cid = out.ConversationID
		mergeTurn = out.TurnIndex
		cidSource = "merge"
	default:
		cid = uuid.NewString()
		cidSource = "generated"
	}

	turnIdx := mergeTurn
	if turnIdx <= 0 {
		turnIdx = rt.NextChatTurnIndex(cid)
	}

	routeLog := chatRouteLogger(log, rid, cid, sess.TenantID, turnIdx)
	w.Header().Set(headerConversationID, cid)

	if routeLog != nil {
		msgCount := conversationwitness.RequestMessageCount(raw)
		recvArgs := []any{
			"msg", naming.MsgConversationReceived,
			"clientModel", clientModel, "stream", stream, "tenant", sess.TenantID,
			"project", proj, "flavor", flav, "cid_source", cidSource, "timeline_kind", naming.TimelineKindBroker,
		}
		if msgCount > 0 {
			recvArgs = append(recvArgs, "message_count", msgCount)
		}
		routeLog.Info("conversation received", recvArgs...)
		routeLog.Info("chat completion request", "msg", "chat.request", "clientModel", clientModel, "stream", stream, "tenant", sess.TenantID, "timeline_kind", naming.TimelineKindBroker)
	}
	LogConversationIncomingToolMessages(routeLog, raw["messages"])

	var chatOpts *chat.ProxyOpts
	if mergeSvc != nil && !stream {
		ms := mergeSvc
		ccid := cid
		ccTenant := sess.TenantID
		lu := lastUser
		chatOpts = &chat.ProxyOpts{
			OnUpstreamJSONSuccess: func(status int, upstreamModel string, jsonBody []byte) {
				if status < 200 || status >= 300 {
					return
				}
				fp := ms.RecordTurn(ctx, ccTenant, proj, flav, ccid, lu, jsonBody, time.Now().UTC(), rid)
				if fp != "" {
					w.Header().Set(headerRollingFingerprint, fp)
				}
			},
		}
	}
	attachConversationDelivery(routeLog, &chatOpts)
	chatOpts.UpstreamRequestID = rid
	chatOpts.WitnessEmitPayloadSample = res.ShouldEmitPayloadSample()
	chatOpts.WitnessPayloadSampleMaxRunes = res.WitnessSampleMaxRunes()

	vmCtx, vmStatus, vmErrBody := resolveVirtualModelChat(rt, clientModel, sess.TenantID, res)
	if vmErrBody != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(vmStatus)
		_ = json.NewEncoder(w).Encode(vmErrBody)
		return
	}
	if vmCtx != nil {
		if handleVirtualModelChat(ctx, w, rt, res, pol, vmCtx, raw, stream, skipToolRouter, th, routeLog,
			cid, turnIdx, rid, sess.TenantID, proj, flav, apiKey, rtDur, chatOpts) {
			return
		}
	}

	emitConversationRequestWitness(routeLog, res, raw)

	if clientModel == "" {
		if routeLog != nil {
			routeLog.Warn("conversation errored", "msg", naming.MsgConversationErrored,
				"statusCode", http.StatusBadRequest, "errorType", "invalid_request", "timeline_kind", naming.TimelineKindBroker)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{"message": "Missing model", "type": "invalid_request"},
		})
		return
	}

	pr := chat.ProxyChatCompletion(ctx, w, res.UpstreamBaseURL, apiKey, clientModel, stream, raw, chatTimeout(res), routeLog, rt.Metrics(), rt.LimitsGuard(), chatOpts)
	if pr.Stream {
		return
	}
	if pr.ErrMessage != "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(pr.Status)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{"message": pr.ErrMessage, "type": "gateway_upstream"},
		})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(pr.Status)
	_, _ = w.Write(pr.JSONBody)
}

type wrapResponse struct {
	http.ResponseWriter
	status int
}

func (w *wrapResponse) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *wrapResponse) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(b)
}

func healthTimeout(res *config.Resolved) time.Duration {
	return time.Duration(res.HealthTimeoutMs) * time.Millisecond
}

func chatTimeout(res *config.Resolved) time.Duration {
	return time.Duration(res.ChatTimeoutMs) * time.Millisecond
}

// httpAccessLogLevel picks the slog level for access-style "http response" lines.
// Successful probe, UI polling, and static asset routes are DEBUG so default INFO logs stay readable.
func httpAccessLogLevel(path string, status int) slog.Level {
	if status < 200 || status >= 300 {
		return slog.LevelInfo
	}
	if strings.HasPrefix(path, "/ui/assets/") || strings.HasPrefix(path, "/assets/") {
		return slog.LevelDebug
	}
	if path == "/v1/indexer/workspaces" {
		return slog.LevelDebug
	}
	switch path {
	case "/health", "/healthz", "/readyz", "/status", "/api/ui/logs", "/api/ui/logs/stream",
		"/ui/settings", "/api/ui/metrics",
		"/api/ui/tokens", "/api/ui/state", "/api/ui/chimera-broker/providers",
		"/api/ui/providers/catalog",
		"/api/ui/indexer/config", "/api/ui/indexer/workspaces",
		"/v1/indexer/storage/stats":
		return slog.LevelDebug
	case "/v1/ingest":
		return slog.LevelDebug
	default:
		return slog.LevelInfo
	}
}

func loggingMiddleware(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		wr := &wrapResponse{ResponseWriter: w}
		next.ServeHTTP(wr, r)
		if log != nil {
			st := wr.status
			if st == 0 {
				st = 200
			}
			rid := requestid.FromContext(r.Context())
			args := []any{
				"msg", "gateway.http.access",
				"method", r.Method,
				"path", r.URL.Path,
				"statusCode", st,
				"responseTimeMs", time.Since(start).Milliseconds(),
				"authorization", gwhttp.RedactBearerAuth(r.Header.Get("Authorization")),
				"service", "gateway",
				"timeline_kind", timelineKindForGatewayHTTPPath(r.URL.Path),
			}
			if rid != "" {
				args = append(args, "request_id", rid)
			}
			log.Log(r.Context(), httpAccessLogLevel(r.URL.Path, st), "http response", args...)
		}
	})
}

// headerConversationID is an optional client-provided id for log correlation; must match requestid.Valid charset.
const headerConversationID = naming.HeaderConversationIDTarget

// headerRequestFingerprint optional client echo of legacy rolling fingerprint header for duplicate detection.
const headerRequestFingerprint = naming.HeaderRequestFingerprintTarget

// headerRollingFingerprint is the gateway-computed rolling hash after each completed JSON completion.
const headerRollingFingerprint = naming.HeaderRollingFingerprintTarget
