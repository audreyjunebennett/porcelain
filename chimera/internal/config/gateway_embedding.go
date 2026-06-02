package config

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// PatchGatewayYAMLBytesWithEmbeddingModel sets rag.embedding.model and, when dim > 0,
// rag.embedding.dim in gateway.yaml bytes.
func PatchGatewayYAMLBytesWithEmbeddingModel(raw []byte, model string, dim int) ([]byte, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		return nil, fmt.Errorf("embedding model required")
	}
	return patchGatewayYAMLApplyRAG(func(embNode *yaml.Node) {
		setOrReplaceMappingString(embNode, "model", model)
		if dim > 0 {
			setOrReplaceMappingInt(embNode, "dim", dim)
		}
	}, raw)
}

// WriteGatewayEmbeddingModel updates rag.embedding.model (and dim when known) in gateway.yaml.
func WriteGatewayEmbeddingModel(gatewayPath, model string, dim int) error {
	raw, err := os.ReadFile(gatewayPath)
	if err != nil {
		return fmt.Errorf("read gateway yaml: %w", err)
	}
	out, err := PatchGatewayYAMLBytesWithEmbeddingModel(raw, model, dim)
	if err != nil {
		return err
	}
	dir := filepath.Dir(gatewayPath)
	tmp, err := os.CreateTemp(dir, "chimera-gw-embed-*.yaml")
	if err != nil {
		return fmt.Errorf("temp file: %w", err)
	}
	tmpPath := tmp.Name()
	_ = tmp.Close()
	defer func() { _ = os.Remove(tmpPath) }()
	if err := os.WriteFile(tmpPath, out, 0o600); err != nil {
		return err
	}
	if _, err := LoadGatewayYAML(tmpPath, nil); err != nil {
		return fmt.Errorf("gateway yaml after patch failed to load: %w", err)
	}
	mode := fs.FileMode(0o644)
	if st, err := os.Stat(gatewayPath); err == nil {
		mode = st.Mode() & fs.ModePerm
	}
	return ReplaceFile(gatewayPath, out, mode)
}

func patchGatewayYAMLApplyRAG(fn func(*yaml.Node), raw []byte) ([]byte, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(raw, &root); err != nil {
		return nil, fmt.Errorf("parse gateway yaml: %w", err)
	}
	if root.Kind != yaml.DocumentNode || len(root.Content) == 0 {
		return nil, fmt.Errorf("gateway yaml: expected document root")
	}
	docMap := root.Content[0]
	if docMap.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("gateway yaml: expected mapping at document root")
	}
	ragNode := mappingGetOrCreateChildMapping(docMap, "rag")
	embNode := mappingGetOrCreateChildMapping(ragNode, "embedding")
	fn(embNode)
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&root); err != nil {
		_ = enc.Close()
		return nil, fmt.Errorf("encode gateway yaml: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("encode gateway yaml: %w", err)
	}
	return buf.Bytes(), nil
}

func setOrReplaceMappingInt(m *yaml.Node, key string, v int) {
	if m.Kind != yaml.MappingNode {
		return
	}
	scalar := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: strconv.Itoa(v)}
	idx := mappingIndex(m, key)
	if idx >= 0 {
		m.Content[idx+1] = scalar
		return
	}
	kn := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}
	m.Content = append(m.Content, kn, scalar)
}
