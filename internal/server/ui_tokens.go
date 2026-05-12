package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/lynn/claudia-gateway/internal/tokens"
)

func (a *adminUI) handleTokensList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if serverBootstrapLocked(a.rt) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "bootstrap mode — use /ui/setup"})
		return
	}
	a.rt.Sync()
	_, tokStore, _ := a.rt.Snapshot()
	if tokStore == nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	meta, err := tokens.ListTokenMeta(tokStore.Path())
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "list tokens", "detail": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"tokens": meta})
}

func (a *adminUI) handleTokensCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if serverBootstrapLocked(a.rt) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "bootstrap mode — use /ui/setup"})
		return
	}
	var body struct {
		Label string `json:"label"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	if err := dec.Decode(&body); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "invalid json"})
		return
	}
	label := strings.TrimSpace(body.Label)
	if label == "" {
		label = "token"
	}
	_, tokStore, _ := a.rt.Snapshot()
	if tokStore == nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	plain, tenant, err := tokens.AppendToken(tokStore.Path(), label)
	if err != nil {
		if a.log != nil {
			a.log.Error("append token", "msg", "gateway.auth.append_failed", "surface", "ui", "err", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "could not save token", "detail": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":        true,
		"token":     plain,
		"tenant_id": tenant,
		"label":     label,
		"message":   "Copy this token now; it will not be shown again.",
	})
}

func (a *adminUI) handleTokensDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if serverBootstrapLocked(a.rt) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "bootstrap mode"})
		return
	}
	var body struct {
		Index int `json:"index"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	if err := dec.Decode(&body); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "invalid json"})
		return
	}
	_, tokStore, _ := a.rt.Snapshot()
	if tokStore == nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if err := tokens.RemoveTokenAt(tokStore.Path(), body.Index); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

// serverBootstrapLocked is true when no valid gateway tokens exist (admin token APIs unavailable).
func serverBootstrapLocked(rt *Runtime) bool {
	return BootstrapMode(rt)
}
