package tokens

import (
	"log/slog"
	"os"
	"sync"

	"gopkg.in/yaml.v3"
)

// Record matches one tokens.yaml entry.
type Record struct {
	Token    string
	TenantID string
	Label    string
}

type yamlDoc struct {
	Tokens []struct {
		Token    string `yaml:"token"`
		TenantID string `yaml:"tenant_id"`
		Label    string `yaml:"label"`
	} `yaml:"tokens"`
}

// Store reloads tokens.yaml when mtime changes (same semantics as src/tokens.ts).
type Store struct {
	path    string
	log     *slog.Logger
	mu      sync.Mutex
	mtimeNs int64
	byToken map[string]Record
}

func NewStore(path string, log *slog.Logger) *Store {
	return &Store{
		path:    path,
		log:     log,
		byToken: make(map[string]Record),
	}
}

// Path returns the resolved tokens.yaml path.
func (s *Store) Path() string {
	if s == nil {
		return ""
	}
	return s.path
}

func (s *Store) ReloadIfStale() {
	s.mu.Lock()
	defer s.mu.Unlock()

	st, err := os.Stat(s.path)
	if err != nil {
		if s.log != nil {
			s.log.Error("tokens file missing", "msg", "gateway.auth.file_missing", "path", s.path, "err", err)
		}
		s.byToken = make(map[string]Record)
		s.mtimeNs = 0
		return
	}
	mt := st.ModTime().UnixNano()
	if mt == s.mtimeNs {
		return
	}
	s.mtimeNs = mt

	raw, err := os.ReadFile(s.path)
	if err != nil {
		if s.log != nil {
			s.log.Error("read tokens yaml", "msg", "gateway.auth.read_failed", "path", s.path, "err", err)
		}
		return
	}
	var doc yamlDoc
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		if s.log != nil {
			s.log.Error("failed to parse tokens yaml", "msg", "gateway.auth.parse_failed", "path", s.path, "err", err)
		}
		s.byToken = make(map[string]Record)
		s.mtimeNs = mt
		return
	}
	next := make(map[string]Record)
	for _, row := range doc.Tokens {
		if row.Token == "" || row.TenantID == "" {
			continue
		}
		next[row.Token] = Record{
			Token:    row.Token,
			TenantID: row.TenantID,
			Label:    row.Label,
		}
	}
	s.byToken = next
	if s.log != nil {
		s.log.Info("gateway client credentials reloaded", "msg", "gateway.auth.reloaded", "path", s.path, "count", len(s.byToken))
	}
}

func (s *Store) Validate(bearer string) *Record {
	s.ReloadIfStale()
	s.mu.Lock()
	defer s.mu.Unlock()
	if r, ok := s.byToken[bearer]; ok {
		return &r
	}
	return nil
}

// Count returns the number of configured gateway tokens (non-empty token + tenant_id rows).
func (s *Store) Count() int {
	if s == nil {
		return 0
	}
	s.ReloadIfStale()
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.byToken)
}
