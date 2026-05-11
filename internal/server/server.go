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

	"github.com/lynn/claudia-gateway/assets"
	"github.com/lynn/claudia-gateway/internal/chat"
	"github.com/lynn/claudia-gateway/internal/config"
	"github.com/lynn/claudia-gateway/internal/conversationmerge"
	"github.com/lynn/claudia-gateway/internal/conversationwitness"
	"github.com/lynn/claudia-gateway/internal/platform"
	"github.com/lynn/claudia-gateway/internal/platform/requestid"
	"github.com/lynn/claudia-gateway/internal/rag"
	"github.com/lynn/claudia-gateway/internal/transform"
	"github.com/lynn/claudia-gateway/internal/upstream"
	"github.com/lynn/claudia-gateway/internal/vectorstore"

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
// shape used by the gateway home page and `chat.bifrost.available_models` log emission. Side
// effect: emits the slog line, matching the prior behavior. Prefer [RefreshAvailableModels]
// when you also want the snapshot cached on the runtime.
func mergedUpstreamModelStats(ctx context.Context, res *config.Resolved, apiKey string, timeout time.Duration, log *slog.Logger) (count int, providers []string, ok bool) {
	snap := buildCatalogSnapshot(ctx, res, apiKey, timeout, log)
	emitAvailableModelsLog(snap, log)
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
  <title>Claudia Gateway — Status</title>
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
  <h1>Claudia Gateway</h1>
  <p class="subtitle">Site status</p>

  <h2>Version</h2>
  <dl>
    <dt>Gateway version</dt><dd><code>{{.Semver}}</code></dd>
    <dt>Virtual model</dt><dd><code>{{.VirtualModel}}</code></dd>
  </dl>

  <h2>Services</h2>
  <dl>
    <dt>Claudia (this gateway)</dt>
    <dd><span class="ok">up</span> · <a href="{{.GatewayURL}}">{{.GatewayURL}}</a></dd>
    <dt>BiFrost (upstream)</dt>
    <dd><span class="{{.BifrostClass}}">{{if .BifrostOK}}up{{else}}down{{end}}</span> · <a href="{{.BifrostURL}}">{{.BifrostURL}}</a></dd>
    <dt>Qdrant (vector store)</dt>
    <dd><span class="{{.QdrantClass}}">{{.QdrantState}}</span> · <a href="{{.QdrantURL}}">{{.QdrantURL}}</a></dd>
    <dt>Indexer (supervised)</dt>
    <dd><span class="{{.IndexerWorkerClass}}">{{.IndexerWorker}}</span> · config: <span class="muted">{{.IndexerConfig}}</span></dd>
  </dl>

  <h2>Configuration</h2>
  <dl>
    <dt>Gateway tokens</dt><dd>{{.TokensCount}} configured</dd>
    <dt>Metrics</dt><dd>{{if .MetricsEnabled}}enabled{{else}}disabled{{end}}</dd>
    <dt>Conversation merge</dt><dd>{{if .ConversationMerge}}enabled{{else}}disabled{{end}}</dd>
    <dt>Upstream model providers</dt><dd>{{.Providers}}</dd>
    <dt>Models available</dt><dd>{{.ModelCount}} <span class="muted">(merged list: virtual + upstream)</span></dd>
  </dl>
</body>
</html>`))

// NewMux builds the v0.1 HTTP surface (src/server.ts parity). overlay configures GET /status;
// pass nil in tests; production passes listen address and optional supervisor info.
// ui enables operator /ui and /api/ui routes; pass nil to disable (tests).
func NewMux(rt *Runtime, log *slog.Logger, overlay *StatusOverlay, ui *UIOptions) http.Handler {
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
		rt.Sync()
		res, tokStore, _ := rt.Snapshot()
		ctx := r.Context()
		apiKey := rt.UpstreamAPIKey()

		gwURL := publicGatewayURL(res, overlay)
		bifrostURL := strings.TrimSuffix(res.UpstreamBaseURL, "/")
		bifrostOK, _, _ := upstream.ProbeHealth(ctx, res.HealthUpstreamURL, apiKey, healthTimeout(res), log)
		bifrostClass := "err"
		if bifrostOK {
			bifrostClass = "ok"
		}

		qdrantURL := strings.TrimSuffix(res.RAG.QdrantURL, "/")
		qdrantState := "disabled (RAG off)"
		qClass := "muted"
		if res.RAG.Enabled {
			if rt.RAG() == nil {
				qdrantState = "unavailable"
				qClass = "err"
			} else if err := rt.RAG().StoreHealth(ctx); err != nil {
				qdrantState = "down"
				qClass = "err"
			} else {
				qdrantState = "up"
				qClass = "ok"
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
			providers = "set upstream API key to query upstream"
		}

		data := struct {
			Semver, VirtualModel string
			GatewayURL           string
			BifrostURL           string
			BifrostOK            bool
			BifrostClass         string
			QdrantURL            string
			QdrantState          string
			QdrantClass          string
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
			BifrostURL:         bifrostURL,
			BifrostOK:          bifrostOK,
			BifrostClass:       bifrostClass,
			QdrantURL:          qdrantURL,
			QdrantState:        qdrantState,
			QdrantClass:        qClass,
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

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		rt.Sync()
		res, _, _ := rt.Snapshot()
		apiKey := rt.UpstreamAPIKey()
		ctx := r.Context()
		ok, st, detail := upstream.ProbeHealth(ctx, res.HealthUpstreamURL, apiKey, healthTimeout(res), log)
		upstreamCheck := map[string]any{
			"ok":     ok,
			"status": st,
		}
		if detail != "" {
			upstreamCheck["detail"] = detail
		}
		checks := map[string]any{
			"upstream": upstreamCheck,
		}
		degraded := !ok
		if res.RAG.Enabled && rt.RAG() != nil {
			qErr := rt.RAG().StoreHealth(ctx)
			qCheck := map[string]any{"ok": qErr == nil}
			if qErr != nil {
				qCheck["detail"] = qErr.Error()
				degraded = true
			}
			checks["qdrant"] = qCheck
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
		writeMergedModelsResponse(w, r.Context(), res, rt.UpstreamAPIKey(), healthTimeout(res), log)
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
		handleV1Ingest(w, r, rt, log)
	})
	mux.HandleFunc("/v1/ingest/session/", func(w http.ResponseWriter, r *http.Request) {
		handleV1IngestSessionTail(w, r, rt, log)
	})
	mux.HandleFunc("/v1/ingest/session", func(w http.ResponseWriter, r *http.Request) {
		handleV1IngestSessionStart(w, r, rt, log)
	})

	mux.HandleFunc("/v1/indexer/config", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		handleIndexerConfig(w, r, rt, log)
	})
	mux.HandleFunc("/v1/indexer/workspaces", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		handleIndexerWorkspaces(w, r, rt, log)
	})
	mux.HandleFunc("/v1/indexer/storage/health", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		handleIndexerHealth(w, r, rt, log)
	})
	mux.HandleFunc("/v1/indexer/storage/stats", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		handleIndexerStats(w, r, rt, log)
	})
	mux.HandleFunc("/v1/indexer/corpus/inventory", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		handleIndexerCorpusInventory(w, r, rt, log)
	})

	registerAdminUI(mux, rt, log, ui)

	return requestid.Middleware(loggingMiddleware(log, mux))
}

func handleV1Models(w http.ResponseWriter, r *http.Request, rt *Runtime, log *slog.Logger) {
	rt.Sync()
	res, tokStore, _ := rt.Snapshot()
	token := bearerToken(r.Header.Get("Authorization"))
	sess := tokStore.Validate(token)
	if token == "" || sess == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{"message": "Unauthorized", "type": "invalid_api_key"},
		})
		return
	}
	writeMergedModelsResponse(w, r.Context(), res, rt.UpstreamAPIKey(), healthTimeout(res), log)
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

// writeMergedModelsResponse lists upstream GET /v1/models, prepends the virtual Claudia model, and writes OpenAI-style JSON.
func writeMergedModelsResponse(w http.ResponseWriter, ctx context.Context, res *config.Resolved, apiKey string, timeout time.Duration, log *slog.Logger) {
	w.Header().Set("Content-Type", "application/json")
	if apiKey == "" {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"message": "Missing upstream API key (set " + res.UpstreamAPIKeyEnv + " or upstream.api_key in gateway.yaml)",
				"type":    "gateway_config",
			},
		})
		return
	}
	st, body, ok := upstream.FetchOpenAIModels(ctx, res.UpstreamBaseURL, apiKey, timeout, log)
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
	data = filterOpenAIModelDataByFreeTier(data, res)
	virtual := map[string]any{
		"id":       res.VirtualModelID,
		"object":   "model",
		"created":  time.Now().Unix(),
		"owned_by": "claudia",
	}
	out := append([]any{virtual}, data...)
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
			routeLog.Info("conversation delivered", "msg", "conversation.delivered",
				"statusCode", st, "stream", stream, "bytes", nb, "total_ms", elapsedMs,
				"timeline_kind", "upstream")
			return
		}
		routeLog.Warn("conversation errored", "msg", "conversation.errored",
			"statusCode", st, "errorType", lifecycleErrorType(st), "timeline_kind", "upstream")
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
	token := bearerToken(r.Header.Get("Authorization"))
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
				"message": "Missing upstream API key (set " + res.UpstreamAPIKeyEnv + " or upstream.api_key in gateway.yaml)",
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
	skipToolRouter := strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Claudia-Tool-Router")), "skip")
	th := res.ToolRouterConfidenceThreshold
	if h := strings.TrimSpace(r.Header.Get("X-Claudia-Tool-Confidence-Threshold")); h != "" {
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
	headerCID := optionalConversationIDFromHeader(r)
	proj := resolveProject(r.Header.Get(headerProject), res.RAG.DefaultProject)
	flav := resolveFlavor(r.Header.Get(headerFlavor), res.RAG.DefaultFlavor)
	lastUser := rag.LastUserText(raw["messages"])

	var mergeSvc *conversationmerge.Service
	if ms := rt.MetricsStore(); ms != nil {
		mergeSvc = conversationmerge.NewService(res.ConversationMerge, ms.DB(), res.UpstreamBaseURL, apiKey, res.RAG, log)
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
				Debug("conversation merge resolve failed", "msg", "conversation.merge.resolve_failed", "err", err)
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
				dedupLog.Info("conversation received", "msg", "conversation.received",
					"clientModel", clientModel, "stream", stream, "tenant", sess.TenantID,
					"project", proj, "flavor", flav, "cid_source", "merge", "timeline_kind", "upstream")
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
				dedupLog.Info("conversation delivered", "msg", "conversation.delivered",
					"statusCode", http.StatusOK, "stream", false, "bytes", int64(n),
					"total_ms", time.Since(flowStart).Milliseconds(), "timeline_kind", "upstream")
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

	raw, trSum := transform.ApplyToolRouter(ctx, raw, transform.Config{
		Enabled:      res.ToolRouterEnabled && !skipToolRouter,
		RouterModels: res.RouterModels,
		Threshold:    th,
		BaseURL:      res.UpstreamBaseURL,
		APIKey:       apiKey,
		HTTPTimeout:  rtDur,
		Log:          routeLog,
		OnAttempt: func(model string, err error) {
			rt.NoteToolRouterAttempt(model, err)
		},
	})

	if routeLog != nil {
		routeLog.Info("conversation received", "msg", "conversation.received",
			"clientModel", clientModel, "stream", stream, "tenant", sess.TenantID,
			"project", proj, "flavor", flav, "cid_source", cidSource, "timeline_kind", "upstream")
		routeLog.Info("chat completion request", "msg", "chat.request", "clientModel", clientModel, "stream", stream, "tenant", sess.TenantID, "timeline_kind", "upstream")
	}
	if routeLog != nil && trSum.Ran {
		errStr := ""
		if trSum.Err != nil {
			errStr = trSum.Err.Error()
			if len(errStr) > 300 {
				errStr = errStr[:300] + "…"
			}
		}
		routeLog.Debug("conversation tool router", "msg", "conversation.tool.router",
			"tools_before", trSum.ToolsBefore, "tools_after", trSum.ToolsAfter,
			"router_model", trSum.RouterModel,
			"err", errStr,
			"timeline_kind", "upstream")
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

	if clientModel == res.VirtualModelID {
		coords := vectorstore.Coords{
			TenantID:  sess.TenantID,
			ProjectID: resolveProject(r.Header.Get(headerProject), res.RAG.DefaultProject),
			FlavorID:  resolveFlavor(r.Header.Get(headerFlavor), res.RAG.DefaultFlavor),
		}
		collection := vectorstore.CollectionName(coords)
		if !res.RAG.Enabled || rt.RAG() == nil {
			if routeLog != nil {
				routeLog.Debug("conversation RAG skipped", "msg", "conversation.rag.skipped",
					"reason", "disabled", "timeline_kind", "qdrant")
			}
		} else if q := rag.LastUserText(raw["messages"]); strings.TrimSpace(q) == "" {
			if routeLog != nil {
				routeLog.Debug("conversation RAG skipped", "msg", "conversation.rag.skipped",
					"reason", "empty_query", "timeline_kind", "qdrant")
			}
		} else {
			hits, rerr := rt.RAG().Retrieve(ctx, rag.RetrieveRequest{
				Coords:         coords,
				Query:          q,
				RequestID:      rid,
				ConversationID: cid,
				TurnIndex:      turnIdx,
				LifecycleLog:   routeLog,
			})
			if rerr != nil {
				if routeLog != nil {
					routeLog.Warn("rag retrieve failed; proceeding without context", "msg", "rag.retrieve.error", "err", rerr,
						"tenant", coords.TenantID, "project", coords.ProjectID, "timeline_kind", "qdrant")
				}
			} else if ctxBlock := rag.FormatRetrievedContext(hits); ctxBlock != "" {
				if routeLog != nil {
					bySrc := rag.HitsBySourceCount(hits)
					for _, rel := range rag.SortedSources(bySrc) {
						routeLog.Debug("rag retrieved hits from source",
							"msg", "rag.retrieve.source",
							"rel", rel,
							"source_hits", bySrc[rel],
							"tenant_id", coords.TenantID,
							"project_id", coords.ProjectID,
							"flavor_id", coords.FlavorID,
							"timeline_kind", "qdrant",
						)
					}
				}
				rag.InjectSystemMessage(raw, ctxBlock)
				if routeLog != nil {
					routeLog.Info("conversation RAG attached", "msg", "conversation.rag.attached",
						"tenant", coords.TenantID,
						"project", coords.ProjectID,
						"flavor", coords.FlavorID,
						"hits", len(hits),
						"collection", collection,
						"timeline_kind", "qdrant")
				}
			} else if routeLog != nil {
				routeLog.Debug("conversation RAG skipped", "msg", "conversation.rag.skipped",
					"reason", "no_hits", "timeline_kind", "qdrant")
			}
		}
		emitConversationRequestWitness(routeLog, res, raw)
		initial, _ := pol.PickInitialModel(raw, res.FallbackChain, res.VirtualModelID)
		if initial == "" {
			if routeLog != nil {
				routeLog.Warn("conversation errored", "msg", "conversation.errored",
					"statusCode", http.StatusServiceUnavailable, "errorType", "gateway_config", "timeline_kind", "upstream")
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]any{
					"message": "Could not resolve an initial upstream model for the virtual Claudia model (check routing policy and fallback chain).",
					"type":    "gateway_config",
				},
			})
			return
		}
		chat.WithVirtualModelFallback(ctx, w, initial, res.FallbackChain, res.UpstreamBaseURL, apiKey, stream, raw, chatTimeout(res), routeLog, rt.Metrics(), rt.LimitsGuard(), chatOpts)
		return
	}

	emitConversationRequestWitness(routeLog, res, raw)

	if clientModel == "" {
		if routeLog != nil {
			routeLog.Warn("conversation errored", "msg", "conversation.errored",
				"statusCode", http.StatusBadRequest, "errorType", "invalid_request", "timeline_kind", "upstream")
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

func bearerToken(h string) string {
	h = strings.TrimSpace(h)
	const p = "Bearer "
	if len(h) <= len(p) || !strings.EqualFold(h[:len(p)], p) {
		return ""
	}
	return strings.TrimSpace(h[len(p):])
}

func redactAuth(h string) string {
	h = strings.TrimSpace(h)
	const p = "Bearer "
	if !strings.HasPrefix(strings.ToLower(h), strings.ToLower(p)) {
		return ""
	}
	tok := strings.TrimSpace(h[len(p):])
	if len(tok) <= 8 {
		return "Bearer ***"
	}
	return "Bearer " + tok[:4] + "…"
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
// Successful probe and UI polling routes are DEBUG so default INFO logs stay readable.
func httpAccessLogLevel(path string, status int) slog.Level {
	if status < 200 || status >= 300 {
		return slog.LevelInfo
	}
	switch path {
	case "/health", "/status", "/api/ui/logs", "/api/ui/logs/stream",
		"/ui/logs", "/api/ui/metrics":
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
				"authorization", redactAuth(r.Header.Get("Authorization")),
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
const headerConversationID = "X-Claudia-Conversation-Id"

// headerRequestFingerprint optional client echo of X-Claudia-Rolling-Fingerprint for duplicate detection.
const headerRequestFingerprint = "X-Claudia-Request-Fingerprint"

// headerRollingFingerprint is the gateway-computed rolling hash after each completed JSON completion.
const headerRollingFingerprint = "X-Claudia-Rolling-Fingerprint"

func optionalConversationIDFromHeader(r *http.Request) string {
	if h := strings.TrimSpace(r.Header.Get(headerConversationID)); requestid.Valid(h) {
		return h
	}
	return ""
}

// ParseLogLevel maps gateway.log_level to slog.Level. "trace" is supported as
// a level one step below DEBUG (see platform.LevelTrace) for extra-chatty
// per-operation traces such as RAG ingest results.
func ParseLogLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "trace":
		return platform.LevelTrace
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// ListenAddrOverride applies -listen flag: "host:port" or ":port".
func ListenAddrOverride(res *config.Resolved, listenFlag string) string {
	if strings.TrimSpace(listenFlag) == "" {
		return res.ListenAddr()
	}
	if strings.HasPrefix(listenFlag, ":") {
		return res.ListenHost + listenFlag
	}
	return listenFlag
}
