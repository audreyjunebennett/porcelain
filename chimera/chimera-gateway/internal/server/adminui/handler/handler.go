package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/adminui/session"
	gruntime "github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/runtime"
	"github.com/lynn/porcelain/internal/naming"
)

// Handler carries shared dependencies for operator UI HTTP handlers.
type Handler struct {
	RT   *gruntime.Runtime
	Log  *slog.Logger
	Opts *session.UIOptions
}

// New constructs a Handler. Returns nil when ui is nil or has no session store.
func New(rt *gruntime.Runtime, log *slog.Logger, ui *session.UIOptions) *Handler {
	if ui == nil || ui.Sessions == nil {
		return nil
	}
	return &Handler{RT: rt, Log: log, Opts: ui}
}

func (h *Handler) cookieName() string { return h.Opts.SessionCookieName() }

// CookieName returns the session cookie name for this handler.
func (h *Handler) CookieName() string { return h.cookieName() }

// SessionOK reports whether the request has a valid UI session cookie.
func (h *Handler) SessionOK(r *http.Request) bool {
	c, err := r.Cookie(h.cookieName())
	if err != nil || c.Value == "" {
		return false
	}
	return h.Opts.Sessions.Valid(c.Value)
}

// SessionTenantID returns the api-keys.yaml tenant_id bound at UI login, or "" when absent.
func (h *Handler) SessionTenantID(r *http.Request) string {
	if h == nil || h.Opts == nil || h.Opts.Sessions == nil {
		return ""
	}
	c, err := r.Cookie(h.cookieName())
	if err != nil || c.Value == "" {
		return ""
	}
	return h.Opts.Sessions.TenantID(c.Value)
}

// RequireAuthJSON wraps JSON API handlers with session auth.
func (h *Handler) RequireAuthJSON(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !h.SessionOK(r) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": "unauthorized"})
			return
		}
		next(w, r)
	}
}

// RequireAuthPage wraps HTML handlers with session auth (redirect to login).
func (h *Handler) RequireAuthPage(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !h.SessionOK(r) {
			q := url.Values{}
			q.Set("next", r.URL.Path)
			http.Redirect(w, r, "/ui/login?"+q.Encode(), http.StatusFound)
			return
		}
		next(w, r)
	}
}

// SetSessionCookie validates a gateway token and sets the admin UI session cookie.
// Returns ok false with serverErr false when the token is missing or invalid; serverErr true when session storage failed.
func (h *Handler) SetSessionCookie(w http.ResponseWriter, token string) (ok bool, serverErr bool) {
	token = strings.TrimSpace(token)
	if token == "" {
		return false, false
	}
	h.RT.Sync()
	_, tokStore, _ := h.RT.Snapshot()
	rec := tokStore.Validate(token)
	if tokStore == nil || rec == nil {
		return false, false
	}
	sid, err := h.Opts.Sessions.Issue(rec.TenantID)
	if err != nil {
		if h.Log != nil {
			h.Log.Debug("ui session issue", "msg", "ui.session.error", "err", err)
		}
		return false, true
	}
	http.SetCookie(w, &http.Cookie{
		Name:     h.cookieName(),
		Value:    sid,
		Path:     "/",
		MaxAge:   int((24 * time.Hour).Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	return true, false
}

// EnvLoginToken returns the optional auto-login token from the environment.
func EnvLoginToken() string {
	return strings.TrimSpace(os.Getenv(naming.EnvGatewayTokenTarget))
}

// SanitizeLoginNext mirrors embedui/login.html: only same-origin /ui/* paths are allowed after sign-in.
func SanitizeLoginNext(next string) string {
	next = strings.TrimSpace(next)
	if !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") || !strings.HasPrefix(next, "/ui/") {
		return "/ui"
	}
	return next
}
