package config

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

const upstreamAPIKeyYAML = "api_key"

// GenerateUpstreamAPIKey returns a random hex string suitable for Bearer auth (32 random bytes).
func GenerateUpstreamAPIKey() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// EnsureGeneratedUpstreamAPIKey writes upstream.api_key to gateway.yaml when the env var is unset
// and the loaded config has no upstream API key. Otherwise returns res unchanged.
func EnsureGeneratedUpstreamAPIKey(gatewayPath string, res *Resolved, log *slog.Logger) (*Resolved, error) {
	if res == nil {
		return nil, fmt.Errorf("nil resolved config")
	}
	if strings.TrimSpace(os.Getenv(res.UpstreamAPIKeyEnv)) != "" {
		return res, nil
	}
	if strings.TrimSpace(res.UpstreamAPIKey) != "" {
		return res, nil
	}
	key, err := GenerateUpstreamAPIKey()
	if err != nil {
		return nil, fmt.Errorf("generate upstream api key: %w", err)
	}
	if err := writeUpstreamAPIKeyYAML(gatewayPath, key); err != nil {
		return nil, err
	}
	if log != nil {
		log.Info("wrote auto-generated upstream.api_key to gateway.yaml", "msg", "gateway.auth.upstream_api_key.autogen", "path", gatewayPath)
	}
	out := CloneResolved(res)
	out.UpstreamAPIKey = key
	return out, nil
}

func writeUpstreamAPIKeyYAML(gatewayPath, apiKey string) error {
	raw, err := os.ReadFile(gatewayPath)
	if err != nil {
		return fmt.Errorf("read gateway yaml: %w", err)
	}
	var root yaml.Node
	if err := yaml.Unmarshal(raw, &root); err != nil {
		return fmt.Errorf("parse gateway yaml: %w", err)
	}
	if root.Kind != yaml.DocumentNode || len(root.Content) == 0 {
		return fmt.Errorf("gateway yaml: expected document root")
	}
	docMap := root.Content[0]
	if docMap.Kind != yaml.MappingNode {
		return fmt.Errorf("gateway yaml: expected mapping at document root")
	}
	upNode := mappingGetOrCreateChildMapping(docMap, "upstream")
	if upNode == nil {
		return fmt.Errorf("gateway yaml: upstream block")
	}
	setOrReplaceMappingScalar(upNode, upstreamAPIKeyYAML, apiKey)
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&root); err != nil {
		_ = enc.Close()
		return fmt.Errorf("encode gateway yaml: %w", err)
	}
	if err := enc.Close(); err != nil {
		return fmt.Errorf("encode gateway yaml: %w", err)
	}
	var mode fs.FileMode = 0o644
	if st, err := os.Stat(gatewayPath); err == nil {
		mode = st.Mode() & fs.ModePerm
	}
	if err := os.WriteFile(gatewayPath, buf.Bytes(), mode); err != nil {
		return fmt.Errorf("write gateway yaml: %w", err)
	}
	return nil
}

func mappingGetOrCreateChildMapping(docMap *yaml.Node, key string) *yaml.Node {
	idx := mappingIndex(docMap, key)
	if idx < 0 {
		kn := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}
		vn := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		docMap.Content = append(docMap.Content, kn, vn)
		return vn
	}
	v := docMap.Content[idx+1]
	if v.Kind != yaml.MappingNode {
		vn := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		docMap.Content[idx+1] = vn
		return vn
	}
	return v
}

func mappingIndex(m *yaml.Node, wantKey string) int {
	if m.Kind != yaml.MappingNode {
		return -1
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		k := m.Content[i]
		if k.Value == wantKey {
			return i
		}
	}
	return -1
}

func setOrReplaceMappingScalar(m *yaml.Node, key, value string) {
	if m.Kind != yaml.MappingNode {
		return
	}
	idx := mappingIndex(m, key)
	valNode := &yaml.Node{
		Kind:  yaml.ScalarNode,
		Tag:   "!!str",
		Value: value,
		Style: yaml.DoubleQuotedStyle,
	}
	if idx >= 0 {
		m.Content[idx+1] = valNode
		return
	}
	kn := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}
	m.Content = append(m.Content, kn, valNode)
}
