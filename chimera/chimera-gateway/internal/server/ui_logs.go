package server

import (
	"net/http"
	"net/url"

	"github.com/lynn/porcelain/chimera/internal/servicelogs"
)

func (a *adminUI) handleLogsPoll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	store := a.opts.LogStore
	if store == nil {
		http.Error(w, "logs unavailable", http.StatusNotFound)
		return
	}
	servicelogs.HandleLogsPoll(store, w, r)
}

func (a *adminUI) handleLogsStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	store := a.opts.LogStore
	if store == nil {
		http.Error(w, "logs unavailable", http.StatusNotFound)
		return
	}
	// Operator UI replays a short tail on connect; full buffer is available via poll/backfill.
	q := r.URL.Query()
	if q.Get("replay") == "" {
		q.Set("replay", string(servicelogs.ReplayTail))
	}
	if q.Get("tail") == "" {
		q.Set("tail", "200")
	}
	r2 := r.Clone(r.Context())
	r2.URL = &url.URL{
		Path:     r.URL.Path,
		RawQuery: q.Encode(),
	}
	servicelogs.HandleLogsStream(store, w, r2)
}

func registerUILogs(mux *http.ServeMux, a *adminUI) {
	if a.opts.LogStore == nil {
		return
	}
	mux.HandleFunc("GET /api/ui/logs", a.requireAuthJSON(a.handleLogsPoll))
	mux.HandleFunc("GET /api/ui/logs/stream", a.requireAuthJSON(a.handleLogsStream))
}
