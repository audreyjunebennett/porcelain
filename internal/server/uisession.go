package server

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"

	"github.com/lynn/claudia-gateway/internal/servicelogs"
)

const defaultUICookieName = "claudia_ui_session"
const defaultSessionTTL = 24 * time.Hour

// uiSessionStore holds short-lived admin UI sessions after gateway token login.
type uiSessionStore struct {
	mu   sync.Mutex
	ttl  time.Duration
	byID map[string]time.Time
}

func newUISessionStore(ttl time.Duration) *uiSessionStore {
	if ttl <= 0 {
		ttl = defaultSessionTTL
	}
	return &uiSessionStore{
		ttl:  ttl,
		byID: make(map[string]time.Time),
	}
}

// issue creates a session id after the caller has validated the gateway token.
func (s *uiSessionStore) issue() (id string, err error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	id = hex.EncodeToString(b[:])
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneLocked()
	s.byID[id] = time.Now().Add(s.ttl)
	return id, nil
}

func (s *uiSessionStore) valid(id string) bool {
	if id == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneLocked()
	exp, ok := s.byID[id]
	if !ok || time.Now().After(exp) {
		return false
	}
	return true
}

func (s *uiSessionStore) revoke(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.byID, id)
}

func (s *uiSessionStore) pruneLocked() {
	now := time.Now()
	for k, exp := range s.byID {
		if now.After(exp) {
			delete(s.byID, k)
		}
	}
}

// UIOptions configures operator UI routes (session cookie + /ui + /api/ui). Nil disables UI.
type UIOptions struct {
	Sessions *uiSessionStore
	// CookieName defaults to claudia_ui_session.
	CookieName string
	// LogStore enables /api/ui/logs and /api/ui/logs/stream (session auth). Nil omits those routes.
	LogStore *servicelogs.Store
}

func (o *UIOptions) cookieName() string {
	if o == nil {
		return defaultUICookieName
	}
	if n := o.CookieName; n != "" {
		return n
	}
	return defaultUICookieName
}

// NewUIOptions returns UIOptions with an in-memory session store (production: same process as gateway).
func NewUIOptions() *UIOptions {
	return &UIOptions{
		Sessions: newUISessionStore(defaultSessionTTL),
	}
}
