package server

import (
	"context"
	"embed"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	chimeraBrokeradmin "github.com/lynn/porcelain/chimera/chimera-gateway/internal/bifrostadmin"
	"github.com/lynn/porcelain/chimera/internal/naming"
	"github.com/lynn/porcelain/chimera/internal/upstream"
)

func envLoginToken() string {
	return strings.TrimSpace(os.Getenv(naming.EnvGatewayTokenTarget))
}

// sanitizeLoginNext mirrors embedui/login.html: only same-origin /ui/* paths are allowed after sign-in.
func sanitizeLoginNext(next string) string {
	next = strings.TrimSpace(next)
	if !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") || !strings.HasPrefix(next, "/ui/") {
		return "/ui/logs"
	}
	return next
}

//go:embed embedui/login.html embedui/panel.html embedui/logs.html embedui/logs.css embedui/theme-tokens.css embedui/logs.js embedui/logs_bootstrap.js embedui/logs/* embedui/logs/*/* embedui/metrics.html embedui/shell.html embedui/pwa.html embedui/reload.svg embedui/setup.html
var adminEmbedUI embed.FS

func chimeraBrokerAdminClient(rt *Runtime) *chimeraBrokeradmin.Client {
	rt.Sync()
	res, _, _ := rt.Snapshot()
	if res == nil {
		return &chimeraBrokeradmin.Client{}
	}
	tok := ""
	if res.UpstreamAPIKeyEnv != "" {
		tok = strings.TrimSpace(os.Getenv(res.UpstreamAPIKeyEnv))
	}
	return &chimeraBrokeradmin.Client{
		BaseURL:     res.UpstreamBaseURL,
		BearerToken: tok,
		HTTPClient:  &http.Client{Timeout: 8 * time.Second},
	}
}

func formatRFC3339OrEmpty(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func publicGatewayBase(r *http.Request) string {
	host := strings.TrimSpace(r.Host)
	if host == "" {
		return "http://127.0.0.1:3000"
	}
	return "http://" + host
}

type adminUI struct {
	rt   *Runtime
	log  *slog.Logger
	opts *UIOptions
}

func (a *adminUI) cookieName() string { return a.opts.cookieName() }

func (a *adminUI) sessionOK(r *http.Request) bool {
	c, err := r.Cookie(a.cookieName())
	if err != nil || c.Value == "" {
		return false
	}
	return a.opts.Sessions.valid(c.Value)
}

func (a *adminUI) requireAuthJSON(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !a.sessionOK(r) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": "unauthorized"})
			return
		}
		next(w, r)
	}
}

func (a *adminUI) requireAuthPage(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !a.sessionOK(r) {
			q := url.Values{}
			q.Set("next", r.URL.Path)
			http.Redirect(w, r, "/ui/login?"+q.Encode(), http.StatusFound)
			return
		}
		next(w, r)
	}
}

func (a *adminUI) serveEmbed(name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		b, err := adminEmbedUI.ReadFile(name)
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(b)
	}
}

func (a *adminUI) serveEmbedAsset(name string, contentType string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		b, err := adminEmbedUI.ReadFile(name)
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		// WebView2 can aggressively cache local app assets across runs; these UI assets
		// are versioned with the executable, so prefer correctness over caching.
		w.Header().Set("Cache-Control", "no-store")
		if contentType != "" {
			w.Header().Set("Content-Type", contentType)
		}
		_, _ = w.Write(b)
	}
}

func (a *adminUI) serveLogsModuleAsset() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// /ui/assets/logs/<path>
		p := strings.TrimPrefix(r.URL.Path, "/ui/assets/logs/")
		p = strings.TrimSpace(p)
		if p == "" || strings.Contains(p, "..") || strings.HasPrefix(p, "/") || strings.ContainsAny(p, "\\") {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		name := "embedui/logs/" + p
		b, err := adminEmbedUI.ReadFile(name)
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		// All current module assets are JS. If/when CSS/images are added here, make this smarter.
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		// Same rationale as serveEmbedAsset: avoid stale JS in desktop WebView.
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(b)
	}
}

// setUISessionCookie validates a gateway token and sets the admin UI session cookie.
// Returns ok false with serverErr false when the token is missing or invalid; serverErr true when session storage failed.
func (a *adminUI) setUISessionCookie(w http.ResponseWriter, token string) (ok bool, serverErr bool) {
	token = strings.TrimSpace(token)
	if token == "" {
		return false, false
	}
	a.rt.Sync()
	_, tokStore, _ := a.rt.Snapshot()
	if tokStore == nil || tokStore.Validate(token) == nil {
		return false, false
	}
	sid, err := a.opts.Sessions.issue()
	if err != nil {
		if a.log != nil {
			// In-memory store: failures here are not persistence I/O; keep default stream quiet (taxonomy P3).
			a.log.Debug("ui session issue", "msg", "ui.session.error", "err", err)
		}
		return false, true
	}
	http.SetCookie(w, &http.Cookie{
		Name:     a.cookieName(),
		Value:    sid,
		Path:     "/",
		MaxAge:   int((24 * time.Hour).Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	return true, false
}

func (a *adminUI) handleLoginGET(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if a.sessionOK(r) {
		http.Redirect(w, r, sanitizeLoginNext(r.URL.Query().Get("next")), http.StatusFound)
		return
	}
	if tok := envLoginToken(); tok != "" {
		ok, srvErr := a.setUISessionCookie(w, tok)
		if srvErr {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		if ok {
			http.Redirect(w, r, sanitizeLoginNext(r.URL.Query().Get("next")), http.StatusFound)
			return
		}
	}
	a.serveEmbed("embedui/login.html")(w, r)
}

func (a *adminUI) handleLoginPOST(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Token string `json:"token"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	if err := dec.Decode(&body); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "invalid json"})
		return
	}
	token := strings.TrimSpace(body.Token)
	ok, srvErr := a.setUISessionCookie(w, token)
	if srvErr {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "invalid token"})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

func (a *adminUI) handleState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	a.rt.Sync()
	res, _, _ := a.rt.Snapshot()
	if res == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "gateway not configured"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()
	client := chimeraBrokerAdminClient(a.rt)
	provOut := make(map[string]any, len(chimeraBrokerUIProviderNames))
	for _, name := range chimeraBrokerUIProviderNames {
		b, st, err := client.GetProvider(ctx, name)
		entry := map[string]any{"provider": name}
		if err != nil {
			entry["ok"] = false
			entry["error"] = err.Error()
			provOut[name] = entry
			continue
		}
		if chimeraBrokeradmin.IsProviderMissingGET(st, b) {
			entry["ok"] = true
			entry["key_configured"] = false
			entry["key_hint"] = ""
			entry["keys"] = []chimeraBrokeradmin.KeyEntrySummary{}
			if name == "ollama" {
				entry["ollama_base_url"] = ""
			}
			provOut[name] = entry
			continue
		}
		entry["http_status"] = st
		if st < 200 || st >= 300 {
			entry["ok"] = false
			entry["error"] = strings.TrimSpace(string(b))
			if entry["error"] == "" {
				entry["error"] = http.StatusText(st)
			}
			provOut[name] = entry
			continue
		}
		sum, serr := chimeraBrokeradmin.SummarizeProvider(name, b)
		if serr != nil {
			entry["ok"] = false
			entry["error"] = serr.Error()
			provOut[name] = entry
			continue
		}
		keyRows, _ := chimeraBrokeradmin.SummarizeProviderKeys(name, b)
		entry["ok"] = true
		entry["key_hint"] = sum.KeyHint
		entry["key_configured"] = sum.KeyConfigured
		entry["keys"] = keyRows
		if sum.OllamaBaseURL != "" {
			entry["ollama_base_url"] = sum.OllamaBaseURL
		}
		provOut[name] = entry
	}
	routeBase := ""
	routingPolicyYAML := ""
	if p := strings.TrimSpace(res.RoutingPolicyPath); p != "" {
		routeBase = filepath.Base(p)
		if b, err := os.ReadFile(p); err == nil {
			routingPolicyYAML = string(b)
		}
	}
	rm, trAt, trErr := a.rt.ToolRouterLast()
	chimeraBrokerURL := strings.TrimSuffix(res.UpstreamBaseURL, "/")
	chimeraBrokerOK, _, chimeraBrokerDetail := upstream.ProbeHealth(ctx, res.HealthUpstreamURL, a.rt.UpstreamAPIKey(), healthTimeout(res), a.log)
	chimeraBrokerState := "down"
	if chimeraBrokerOK {
		chimeraBrokerState = "up"
	}
	qdrantURL := strings.TrimSuffix(res.RAG.QdrantURL, "/")
	qdrantState := "disabled"
	if res.RAG.Enabled {
		if a.rt.RAG() == nil {
			qdrantState = "unavailable"
		} else if err := a.rt.RAG().StoreHealth(ctx); err != nil {
			qdrantState = "down"
		} else {
			qdrantState = "up"
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
			idxSt := a.rt.IndexerSupervisorStatus()
			if strings.TrimSpace(idxSt.WorkerState) != "" {
				indexerWorker = strings.TrimSpace(idxSt.WorkerState)
			}
			indexerDeclaredState = strings.TrimSpace(idxSt.LastState)
			indexerLastHeartbeatAt = formatRFC3339OrEmpty(idxSt.LastHeartbeatAt)
			indexerLastLogAt = formatRFC3339OrEmpty(idxSt.LastLogAt)
			indexerDetail = strings.TrimSpace(idxSt.LastError)
			if indexerWorker == "" {
				indexerWorker = "unknown"
			}
		}
	}
	overviewState := "ok"
	if chimeraBrokerState != "up" || (res.RAG.Enabled && qdrantState != "up") {
		overviewState = "degraded"
	}
	if res.IndexerSupervisedEnabled && idxScope {
		switch indexerWorker {
		case "down", "degraded":
			overviewState = "degraded"
		case "up":
			// no-op
		default:
			if overviewState == "ok" {
				overviewState = "monitor"
			}
		}
	}
	gwOut := map[string]any{
		"semver":                           res.Semver,
		"virtual_model_id":                 res.VirtualModelID,
		"public_base_url":                  publicGatewayBase(r),
		"token_hint":                       "Paste the same gateway token you used to sign in.",
		"filter_free_tier_models":          res.FilterFreeTierModels,
		"fallback_chain":                   res.FallbackChain,
		"routing_policy_basename":          routeBase,
		"router_models":                    res.RouterModels,
		"tool_router_enabled":              res.ToolRouterEnabled,
		"tool_router_confidence_threshold": res.ToolRouterConfidenceThreshold,
		"tool_router_last_model":           rm,
		"tool_router_last_error":           trErr,
		"tool_router_last_at":              formatRFC3339OrEmpty(trAt),
		"routing_policy_yaml":              routingPolicyYAML,
		"service_overview": map[string]any{
			"overall_state": overviewState,
			"gateway": map[string]any{
				"state": "up",
			},
			"chimera-broker": map[string]any{
				"state":  chimeraBrokerState,
				"url":    chimeraBrokerURL,
				"detail": chimeraBrokerDetail,
			},
			"chimera-vectorstore": map[string]any{
				"enabled": res.RAG.Enabled,
				"state":   qdrantState,
				"url":     qdrantURL,
			},
			"chimera-indexer": map[string]any{
				"enabled":             res.IndexerSupervisedEnabled,
				"in_scope":            idxScope,
				"worker":              indexerWorker,
				"state":               indexerDeclaredState,
				"last_heartbeat_at":   indexerLastHeartbeatAt,
				"last_log_at":         indexerLastLogAt,
				"detail":              indexerDetail,
				"supervision_signals": "process_liveness + indexer.state heartbeat",
			},
			"refreshed_at": time.Now().UTC().Format(time.RFC3339),
		},
	}
	gwOut["indexer_supervised_config_path"] = res.IndexerSupervisedConfigPath
	gwOut["indexer_supervised_enabled"] = res.IndexerSupervisedEnabled
	gwOut["operator_sqlite_path"] = res.OperatorSQLitePath
	gwOut["operator_store_open"] = a.rt.OperatorStore() != nil
	out := map[string]any{
		"gateway":   gwOut,
		"providers": provOut,
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

func registerAdminUI(mux *http.ServeMux, rt *Runtime, log *slog.Logger, ui *UIOptions) {
	if ui == nil || ui.Sessions == nil {
		return
	}
	a := &adminUI{rt: rt, log: log, opts: ui}

	mux.HandleFunc("GET /ui", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if a.sessionOK(r) {
			http.Redirect(w, r, "/ui/desktop", http.StatusFound)
			return
		}
		http.Redirect(w, r, "/ui/login", http.StatusFound)
	})

	mux.HandleFunc("GET /ui/login", a.handleLoginGET)
	mux.HandleFunc("GET /ui/panel", a.requireAuthPage(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/ui/logs?focus=admin", http.StatusFound)
	}))
	mux.HandleFunc("GET /ui/metrics", a.requireAuthPage(a.serveEmbed("embedui/metrics.html")))
	mux.HandleFunc("GET /ui/desktop", a.requireAuthPage(a.serveEmbed("embedui/shell.html")))
	mux.HandleFunc("GET /ui/pwa", a.requireAuthPage(a.serveEmbed("embedui/pwa.html")))
	mux.HandleFunc("GET /ui/indexer", a.requireAuthPage(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/ui/logs", http.StatusFound)
	}))
	if a.opts.LogStore != nil {
		mux.HandleFunc("GET /ui/assets/reload.svg", a.requireAuthPage(a.serveEmbedAsset("embedui/reload.svg", "image/svg+xml; charset=utf-8")))
		mux.HandleFunc("GET /ui/assets/logs.css", a.requireAuthPage(a.serveEmbedAsset("embedui/logs.css", "text/css; charset=utf-8")))
		mux.HandleFunc("GET /ui/assets/theme-tokens.css", a.requireAuthPage(a.serveEmbedAsset("embedui/theme-tokens.css", "text/css; charset=utf-8")))
		mux.HandleFunc("GET /ui/assets/logs.js", a.requireAuthPage(a.serveEmbedAsset("embedui/logs_bootstrap.js", "application/javascript; charset=utf-8")))
		mux.HandleFunc("GET /ui/assets/logs/main.js", a.requireAuthPage(a.serveEmbedAsset("embedui/logs.js", "application/javascript; charset=utf-8")))
		mux.HandleFunc("GET /ui/assets/logs/", a.requireAuthPage(a.serveLogsModuleAsset()))
		mux.HandleFunc("GET /ui/logs", a.requireAuthPage(a.serveEmbed("embedui/logs.html")))
	}

	mux.HandleFunc("POST /api/ui/login", a.handleLoginPOST)
	mux.HandleFunc("POST /api/ui/logout", a.handleLogoutPOST)
	mux.HandleFunc("GET /api/ui/state", a.requireAuthJSON(a.handleState))
	mux.HandleFunc("GET /api/ui/metrics", a.requireAuthJSON(a.handleMetricsGET))
	mux.HandleFunc("GET /api/ui/chimera-broker/providers", a.requireAuthJSON(a.handleChimeraBrokerProviderHealth))

	for _, p := range []string{"groq", "gemini"} {
		prov := p
		mux.HandleFunc("POST /api/ui/provider/"+prov+"/keys", a.requireAuthJSON(a.saveAppendProviderKey(prov)))
		mux.HandleFunc("POST /api/ui/provider/"+prov+"/keys/delete", a.requireAuthJSON(a.saveRemoveProviderKey(prov)))
	}
	mux.HandleFunc("POST /api/ui/provider/ollama/base_url", a.requireAuthJSON(a.saveOllamaBaseURL))

	mux.HandleFunc("GET /api/ui/tokens", a.requireAuthJSON(a.handleTokensList))
	mux.HandleFunc("POST /api/ui/tokens", a.requireAuthJSON(a.handleTokensCreate))
	mux.HandleFunc("POST /api/ui/tokens/delete", a.requireAuthJSON(a.handleTokensDelete))
	mux.HandleFunc("POST /api/ui/routing/preview", a.requireAuthJSON(a.handleRoutingPreviewPOST))
	mux.HandleFunc("POST /api/ui/routing/policy", a.requireAuthJSON(a.handleRoutingPolicySavePOST))
	mux.HandleFunc("POST /api/ui/routing/fallback_chain", a.requireAuthJSON(a.handleRoutingFallbackChainSavePOST))
	mux.HandleFunc("POST /api/ui/routing/generate", a.requireAuthJSON(a.handleRoutingGeneratePOST))
	mux.HandleFunc("POST /api/ui/routing/evaluate", a.requireAuthJSON(a.handleRoutingEvaluatePOST))
	mux.HandleFunc("POST /api/ui/routing/filter_free_tier_models", a.requireAuthJSON(a.handleRoutingFilterFreeTierPOST))
	mux.HandleFunc("POST /api/ui/routing/router_tooling", a.requireAuthJSON(a.handleRoutingRouterToolingPOST))

	mux.HandleFunc("GET /api/ui/indexer/config", a.requireAuthJSON(a.handleIndexerConfigGET))
	mux.HandleFunc("PUT /api/ui/indexer/config", a.requireAuthJSON(a.handleIndexerConfigPUT))
	mux.HandleFunc("GET /api/ui/indexer/workspaces", a.requireAuthJSON(a.handleIndexerWorkspacesGET))
	mux.HandleFunc("POST /api/ui/indexer/workspaces", a.requireAuthJSON(a.handleIndexerWorkspacesPOST))
	mux.HandleFunc("PUT /api/ui/indexer/workspaces/{id}", a.requireAuthJSON(a.handleIndexerWorkspacePUT))
	mux.HandleFunc("DELETE /api/ui/indexer/workspaces/{id}", a.requireAuthJSON(a.handleIndexerWorkspaceDELETE))
	mux.HandleFunc("POST /api/ui/indexer/workspaces/{id}/paths", a.requireAuthJSON(a.handleIndexerWorkspacePathPOST))
	mux.HandleFunc("PUT /api/ui/indexer/workspace-paths/{pathid}", a.requireAuthJSON(a.handleIndexerWorkspacePathPUT))
	mux.HandleFunc("DELETE /api/ui/indexer/workspace-paths/{pathid}", a.requireAuthJSON(a.handleIndexerWorkspacePathDELETE))

	registerUILogs(mux, a)
}
