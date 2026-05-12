package tokens

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

// GenerateGatewayToken returns a random bearer string suitable for gateway auth.
func GenerateGatewayToken() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// TenantIDFromLabel derives a stable tenant slug from a human label.
func TenantIDFromLabel(label string) string {
	s := strings.ToLower(strings.TrimSpace(label))
	s = nonSlug.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		return "default"
	}
	if len(s) > 64 {
		s = s[:64]
	}
	return s
}

// AppendToken adds one token row to path (creates the file if absent). Writes atomically.
func AppendToken(path, label string) (plainToken, tenantID string, err error) {
	plainToken, err = GenerateGatewayToken()
	if err != nil {
		return "", "", err
	}
	tenantID = TenantIDFromLabel(label)
	if d := filepath.Dir(path); d != "" && d != "." {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return "", "", fmt.Errorf("tokens dir: %w", err)
		}
	}

	var doc yamlDoc
	if raw, rerr := os.ReadFile(path); rerr == nil && len(strings.TrimSpace(string(raw))) > 0 {
		if uerr := yaml.Unmarshal(raw, &doc); uerr != nil {
			return "", "", fmt.Errorf("parse existing tokens yaml: %w", uerr)
		}
	}
	doc.Tokens = append(doc.Tokens, struct {
		Token    string `yaml:"token"`
		TenantID string `yaml:"tenant_id"`
		Label    string `yaml:"label"`
	}{
		Token:    plainToken,
		TenantID: tenantID,
		Label:    strings.TrimSpace(label),
	})
	out, err := yaml.Marshal(&doc)
	if err != nil {
		return "", "", err
	}
	if err := atomicWriteFile(path, out, 0o600); err != nil {
		return "", "", err
	}
	return plainToken, tenantID, nil
}

// TokenMeta is one row for admin listing.
type TokenMeta struct {
	Index    int    `json:"index"`
	Label    string `json:"label"`
	TenantID string `json:"tenant_id"`
	Token    string `json:"token"`
}

// ListTokenMeta returns valid rows in file order with YAML slice indices for removal.
func ListTokenMeta(path string) ([]TokenMeta, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc yamlDoc
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	var out []TokenMeta
	for i, row := range doc.Tokens {
		if strings.TrimSpace(row.Token) == "" || strings.TrimSpace(row.TenantID) == "" {
			continue
		}
		out = append(out, TokenMeta{
			Index:    i,
			Label:    row.Label,
			TenantID: row.TenantID,
			Token:    row.Token,
		})
	}
	return out, nil
}

// RemoveTokenAt removes the row at doc.Tokens[index] and rewrites the file atomically.
func RemoveTokenAt(path string, index int) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var doc yamlDoc
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return err
	}
	if index < 0 || index >= len(doc.Tokens) {
		return fmt.Errorf("token index out of range")
	}
	doc.Tokens = append(doc.Tokens[:index], doc.Tokens[index+1:]...)
	out, err := yaml.Marshal(&doc)
	if err != nil {
		return err
	}
	return atomicWriteFile(path, out, 0o600)
}

func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".tokens-*.yaml.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}
