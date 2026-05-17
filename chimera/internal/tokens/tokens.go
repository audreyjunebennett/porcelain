package tokens

import (
	"log/slog"
	"os"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// Record matches one client credential row from api-keys.yaml.
type Record struct {
	Token    string
	TenantID string
	Label    string
}

type yamlDoc struct {
	APIKeys []struct {
		Secret   string `yaml:"secret"`
		TenantID string `yaml:"tenant_id"`
		Label    string `yaml:"label"`
	} `yaml:"api_keys"`
}

// Store reloads the credential YAML when mtime changes (same semantics as src/tokens.ts).
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

// Path returns the resolved credential YAML path.
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
	for _, row := range doc.APIKeys {
		if strings.TrimSpace(row.Secret) == "" || strings.TrimSpace(row.TenantID) == "" {
			continue
		}
		next[row.Secret] = Record{
			Token:    strings.TrimSpace(row.Secret),
			TenantID: strings.TrimSpace(row.TenantID),
			Label:    strings.TrimSpace(row.Label),
		}
	}
	s.byToken = next
	if s.log != nil {
		s.log.Info("gateway client credentials reloaded", "msg", "gateway.auth.reloaded", "path", s.path, "count", len(s.byToken))
	}
}

func normalizeBearerSecret(raw string) string {
	return strings.TrimSpace(raw)
}

func (s *Store) Validate(bearer string) *Record {
	s.ReloadIfStale()
	s.mu.Lock()
	defer s.mu.Unlock()
	if r, ok := s.byToken[normalizeBearerSecret(bearer)]; ok {
		return &r
	}
	return nil
}

// Count returns the number of configured gateway client credentials.
func (s *Store) Count() int {
	if s == nil {
		return 0
	}
	s.ReloadIfStale()
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.byToken)
}

func defaultCredentialDoc() yamlDoc {
	return yamlDoc{
		APIKeys: []struct {
			Secret   string `yaml:"secret"`
			TenantID string `yaml:"tenant_id"`
			Label    string `yaml:"label"`
		}{},
	}
}

func appendCredentialRow(doc *yamlDoc, secret, tenantID, label string) {
	if doc == nil {
		return
	}
	doc.APIKeys = append(doc.APIKeys, struct {
		Secret   string `yaml:"secret"`
		TenantID string `yaml:"tenant_id"`
		Label    string `yaml:"label"`
	}{
		Secret:   secret,
		TenantID: tenantID,
		Label:    label,
	})
}

func rowIsValidSecret(secret, tenantID string) bool {
	return strings.TrimSpace(secret) != "" && strings.TrimSpace(tenantID) != ""
}

func canonicalCredentialPath(path string) string {
	return strings.TrimSpace(path)
}

