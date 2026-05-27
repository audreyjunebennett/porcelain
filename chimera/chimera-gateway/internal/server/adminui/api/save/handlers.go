package save

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/brokeradmin"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/adminui/apijson"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/adminui/apirut"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/adminui/handler"
)

func saveAppendProviderKey(h *handler.Handler, provider string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !apirut.IsKeyedCatalogProvider(provider) {
			apijson.WriteError(w, http.StatusNotFound, "unknown provider", "")
			return
		}
		var body struct {
			Value string `json:"value"`
		}
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
		if err := dec.Decode(&body); err != nil {
			apijson.WriteError(w, http.StatusBadRequest, "invalid json", "")
			return
		}
		v := strings.TrimSpace(body.Value)
		if v == "" {
			apijson.WriteError(w, http.StatusBadRequest, "value required", "")
			return
		}
		ctx := r.Context()
		client := apirut.BrokerAdminClient(h.RT)
		cur, st, err := client.GetProvider(ctx, provider)
		if err != nil {
			apijson.WriteError(w, http.StatusBadGateway, "chimera-broker unreachable", apijson.TruncateErrMsg(err.Error()))
			return
		}
		cur, ok := brokeradmin.NormalizeProviderGETForMerge(st, cur)
		if !ok {
			apijson.WriteError(w, http.StatusBadGateway, fmt.Sprintf("chimera-broker GET %d", st), apijson.TruncateErrMsg(string(cur)))
			return
		}
		merged, err := brokeradmin.AppendProviderAPIKey(provider, cur, v)
		if err != nil {
			apijson.WriteError(w, http.StatusInternalServerError, "merge failed", apijson.TruncateErrMsg(err.Error()))
			return
		}
		pst, pbody, err := client.PutProvider(ctx, provider, merged)
		if err != nil {
			apijson.WriteError(w, http.StatusBadGateway, "chimera-broker PUT failed", apijson.TruncateErrMsg(err.Error()))
			return
		}
		if pst < 200 || pst >= 300 {
			apijson.WriteError(w, http.StatusBadGateway, fmt.Sprintf("chimera-broker PUT %d", pst), apijson.TruncateErrMsg(string(pbody)))
			return
		}
		brokeradmin.InvalidateProviderProbeCacheFor(provider)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}
}

func saveRemoveProviderKey(h *handler.Handler, provider string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !apirut.IsKeyedCatalogProvider(provider) {
			apijson.WriteError(w, http.StatusNotFound, "unknown provider", "")
			return
		}
		var body struct {
			Name string `json:"name"`
		}
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
		if err := dec.Decode(&body); err != nil {
			apijson.WriteError(w, http.StatusBadRequest, "invalid json", "")
			return
		}
		name := strings.TrimSpace(body.Name)
		if name == "" {
			apijson.WriteError(w, http.StatusBadRequest, "name required", "")
			return
		}
		ctx := r.Context()
		client := apirut.BrokerAdminClient(h.RT)
		cur, st, err := client.GetProvider(ctx, provider)
		if err != nil {
			apijson.WriteError(w, http.StatusBadGateway, "chimera-broker unreachable", apijson.TruncateErrMsg(err.Error()))
			return
		}
		cur, ok := brokeradmin.NormalizeProviderGETForMerge(st, cur)
		if !ok {
			apijson.WriteError(w, http.StatusBadGateway, fmt.Sprintf("chimera-broker GET %d", st), apijson.TruncateErrMsg(string(cur)))
			return
		}
		merged, err := brokeradmin.RemoveProviderKeyByName(cur, name)
		if err != nil {
			apijson.WriteError(w, http.StatusBadRequest, apijson.TruncateErrMsg(err.Error()), "")
			return
		}
		pst, pbody, err := client.PutProvider(ctx, provider, merged)
		if err != nil {
			apijson.WriteError(w, http.StatusBadGateway, "chimera-broker PUT failed", apijson.TruncateErrMsg(err.Error()))
			return
		}
		if pst < 200 || pst >= 300 {
			apijson.WriteError(w, http.StatusBadGateway, fmt.Sprintf("chimera-broker PUT %d", pst), apijson.TruncateErrMsg(string(pbody)))
			return
		}
		brokeradmin.InvalidateProviderProbeCacheFor(provider)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}
}

func saveOllamaBaseURL(h *handler.Handler, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		BaseURL string `json:"base_url"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	if err := dec.Decode(&body); err != nil {
		apijson.WriteError(w, http.StatusBadRequest, "invalid json", "")
		return
	}
	u := strings.TrimSpace(body.BaseURL)
	if u == "" {
		apijson.WriteError(w, http.StatusBadRequest, "base_url required", "")
		return
	}
	parsed, err := url.Parse(u)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		apijson.WriteError(w, http.StatusBadRequest, "invalid base_url", "")
		return
	}
	ctx := r.Context()
	client := apirut.BrokerAdminClient(h.RT)
	cur, st, err := client.GetProvider(ctx, "ollama")
	if err != nil {
		apijson.WriteError(w, http.StatusBadGateway, "chimera-broker unreachable", apijson.TruncateErrMsg(err.Error()))
		return
	}
	cur, ok := brokeradmin.NormalizeProviderGETForMerge(st, cur)
	if !ok {
		apijson.WriteError(w, http.StatusBadGateway, fmt.Sprintf("chimera-broker GET %d", st), apijson.TruncateErrMsg(string(cur)))
		return
	}
	merged, err := brokeradmin.MergeOllamaBaseURL(cur, u)
	if err != nil {
		apijson.WriteError(w, http.StatusInternalServerError, "merge failed", apijson.TruncateErrMsg(err.Error()))
		return
	}
	pst, pbody, err := client.PutProvider(ctx, "ollama", merged)
	if err != nil {
		apijson.WriteError(w, http.StatusBadGateway, "chimera-broker PUT failed", apijson.TruncateErrMsg(err.Error()))
		return
	}
	if pst < 200 || pst >= 300 {
		apijson.WriteError(w, http.StatusBadGateway, fmt.Sprintf("chimera-broker PUT %d", pst), apijson.TruncateErrMsg(string(pbody)))
		return
	}
	brokeradmin.InvalidateProviderProbeCacheFor("ollama")
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

func handleLogoutPOST(h *handler.Handler, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if c, err := r.Cookie(h.CookieName()); err == nil && c.Value != "" {
		h.Opts.Sessions.Revoke(c.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name:   h.CookieName(),
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})
	w.WriteHeader(http.StatusNoContent)
}
