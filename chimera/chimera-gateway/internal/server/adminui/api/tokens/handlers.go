package tokens

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/adminui/apirut"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/adminui/handler"
	"github.com/lynn/porcelain/chimera/internal/tokens"
	"github.com/lynn/porcelain/internal/operatorapi"
)

func handleTokensList(h *handler.Handler, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if apirut.BootstrapLocked(h.RT) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(operatorapi.ErrorBody{Error: "bootstrap mode — use /ui/setup"})
		return
	}
	h.RT.Sync()
	_, tokStore := h.RT.Snapshot()
	if tokStore == nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	meta, err := tokens.ListTokenMeta(tokStore.Path())
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(operatorapi.ErrorBody{Error: "list tokens", Detail: err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(operatorapi.TokensListResponse{Tokens: toAPITokens(meta)})
}

func handleTokensCreate(h *handler.Handler, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if apirut.BootstrapLocked(h.RT) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(operatorapi.ErrorBody{Error: "bootstrap mode — use /ui/setup"})
		return
	}
	var body operatorapi.TokenCreateRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	if err := dec.Decode(&body); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(operatorapi.ErrorBody{Error: "invalid json"})
		return
	}
	label := strings.TrimSpace(body.Label)
	if label == "" {
		label = "token"
	}
	_, tokStore := h.RT.Snapshot()
	if tokStore == nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	plain, tenant, err := tokens.AppendToken(tokStore.Path(), label)
	if err != nil {
		if h.Log != nil {
			h.Log.Error("append token", "msg", "gateway.auth.append_failed", "surface", "ui", "err", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(operatorapi.ErrorBody{Error: "could not save token", Detail: err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(operatorapi.TokenCreateResponse{
		OK:       true,
		Token:    plain,
		TenantID: tenant,
		Label:    label,
		Message:  "Copy this token now; it will not be shown again.",
	})
}

func handleTokensDelete(h *handler.Handler, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if apirut.BootstrapLocked(h.RT) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(operatorapi.ErrorBody{Error: "bootstrap mode"})
		return
	}
	var body operatorapi.TokenDeleteRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	if err := dec.Decode(&body); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(operatorapi.ErrorBody{Error: "invalid json"})
		return
	}
	_, tokStore := h.RT.Snapshot()
	if tokStore == nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if err := tokens.RemoveTokenAt(tokStore.Path(), body.Index); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(operatorapi.ErrorBody{Error: err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(operatorapi.OKResponse{OK: true})
}

func toAPITokens(in []tokens.TokenMeta) []operatorapi.TokenMeta {
	out := make([]operatorapi.TokenMeta, len(in))
	for i, m := range in {
		out[i] = operatorapi.TokenMeta{
			Index:    m.Index,
			Label:    m.Label,
			TenantID: m.TenantID,
			Token:    m.Token,
		}
	}
	return out
}
