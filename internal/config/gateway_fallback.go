package config

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"

	"gopkg.in/yaml.v3"
)

func patchGatewayYAMLApply(raw []byte, fn func(*yaml.Node)) ([]byte, error) {
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
	rtNode := mappingGetOrCreateChildMapping(docMap, "routing")
	fn(rtNode)
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

func patchGatewayYAMLApplyRoot(raw []byte, fn func(*yaml.Node)) ([]byte, error) {
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
	fn(docMap)
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

// PatchGatewayYAMLBytesWithFallbackChain returns a copy of raw gateway YAML with routing.fallback_chain replaced.
func PatchGatewayYAMLBytesWithFallbackChain(raw []byte, chain []string) ([]byte, error) {
	return patchGatewayYAMLApply(raw, func(rtNode *yaml.Node) {
		setOrReplaceMappingSequence(rtNode, "fallback_chain", chain)
	})
}

// PatchGatewayYAMLBytesWithFilterFreeTierModels sets routing.filter_free_tier_models.
func PatchGatewayYAMLBytesWithFilterFreeTierModels(raw []byte, enabled bool) ([]byte, error) {
	return patchGatewayYAMLApply(raw, func(rtNode *yaml.Node) {
		setOrReplaceMappingBool(rtNode, "filter_free_tier_models", enabled)
	})
}

// PatchGatewayYAMLBytesWithRouterModels sets routing.router_models only (does not change tool_router).
func PatchGatewayYAMLBytesWithRouterModels(raw []byte, routerModels []string) ([]byte, error) {
	return patchGatewayYAMLApply(raw, func(rtNode *yaml.Node) {
		setOrReplaceMappingSequence(rtNode, "router_models", routerModels)
	})
}

// PatchGatewayYAMLBytesWithRouterTooling sets routing.router_models and routing.tool_router.
func PatchGatewayYAMLBytesWithRouterTooling(raw []byte, routerModels []string, toolRouterEnabled bool, confidenceThreshold float64) ([]byte, error) {
	return patchGatewayYAMLApply(raw, func(rtNode *yaml.Node) {
		setOrReplaceMappingSequence(rtNode, "router_models", routerModels)
		tr := mappingGetOrCreateChildMapping(rtNode, "tool_router")
		setOrReplaceMappingBool(tr, "enabled", toolRouterEnabled)
		setOrReplaceMappingFloat(tr, "confidence_threshold", confidenceThreshold)
	})
}

// PatchGatewayYAMLBytesWithEnsembleEnabled sets top-level ensemble.enabled.
func PatchGatewayYAMLBytesWithEnsembleEnabled(raw []byte, enabled bool) ([]byte, error) {
	return patchGatewayYAMLApplyRoot(raw, func(docMap *yaml.Node) {
		en := mappingGetOrCreateChildMapping(docMap, "ensemble")
		setOrReplaceMappingBool(en, "enabled", enabled)
	})
}

// WriteGatewayFallbackChain updates routing.fallback_chain in gateway.yaml using a yaml.v3
// document round-trip (comments and key order outside routing.fallback_chain may shift).
func WriteGatewayFallbackChain(gatewayPath string, chain []string) error {
	raw, err := os.ReadFile(gatewayPath)
	if err != nil {
		return fmt.Errorf("read gateway yaml: %w", err)
	}
	out, err := PatchGatewayYAMLBytesWithFallbackChain(raw, chain)
	if err != nil {
		return err
	}
	var mode fs.FileMode = 0o644
	if st, err := os.Stat(gatewayPath); err == nil {
		mode = st.Mode() & fs.ModePerm
	}
	if err := os.WriteFile(gatewayPath, out, mode); err != nil {
		return fmt.Errorf("write gateway yaml: %w", err)
	}
	return nil
}

// WriteGatewayFilterFreeTierModels sets routing.filter_free_tier_models and validates load from a temp file beside gatewayPath.
func WriteGatewayFilterFreeTierModels(gatewayPath string, enabled bool) error {
	raw, err := os.ReadFile(gatewayPath)
	if err != nil {
		return fmt.Errorf("read gateway yaml: %w", err)
	}
	out, err := PatchGatewayYAMLBytesWithFilterFreeTierModels(raw, enabled)
	if err != nil {
		return err
	}
	dir := filepath.Dir(gatewayPath)
	tmp, err := os.CreateTemp(dir, "claudia-gw-ft-*.yaml")
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

// WriteGatewayRouterTooling updates router_models and tool_router in gateway.yaml and validates load.
func WriteGatewayRouterTooling(gatewayPath string, routerModels []string, toolRouterEnabled bool, confidenceThreshold float64) error {
	raw, err := os.ReadFile(gatewayPath)
	if err != nil {
		return fmt.Errorf("read gateway yaml: %w", err)
	}
	out, err := PatchGatewayYAMLBytesWithRouterTooling(raw, routerModels, toolRouterEnabled, confidenceThreshold)
	if err != nil {
		return err
	}
	dir := filepath.Dir(gatewayPath)
	tmp, err := os.CreateTemp(dir, "claudia-gw-router-*.yaml")
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

// WriteGatewayEnsembleEnabled updates ensemble.enabled in gateway.yaml and validates load.
func WriteGatewayEnsembleEnabled(gatewayPath string, enabled bool) error {
	raw, err := os.ReadFile(gatewayPath)
	if err != nil {
		return fmt.Errorf("read gateway yaml: %w", err)
	}
	out, err := PatchGatewayYAMLBytesWithEnsembleEnabled(raw, enabled)
	if err != nil {
		return err
	}
	dir := filepath.Dir(gatewayPath)
	tmp, err := os.CreateTemp(dir, "claudia-gw-ensemble-*.yaml")
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

func setOrReplaceMappingSequence(m *yaml.Node, key string, values []string) {
	if m.Kind != yaml.MappingNode {
		return
	}
	seq := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	for _, v := range values {
		seq.Content = append(seq.Content, &yaml.Node{
			Kind:  yaml.ScalarNode,
			Tag:   "!!str",
			Value: v,
			Style: yaml.DoubleQuotedStyle,
		})
	}
	idx := mappingIndex(m, key)
	if idx >= 0 {
		m.Content[idx+1] = seq
		return
	}
	kn := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}
	m.Content = append(m.Content, kn, seq)
}

func setOrReplaceMappingBool(m *yaml.Node, key string, v bool) {
	if m.Kind != yaml.MappingNode {
		return
	}
	val := "false"
	if v {
		val = "true"
	}
	scalar := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: val}
	idx := mappingIndex(m, key)
	if idx >= 0 {
		m.Content[idx+1] = scalar
		return
	}
	kn := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}
	m.Content = append(m.Content, kn, scalar)
}

func setOrReplaceMappingFloat(m *yaml.Node, key string, v float64) {
	if m.Kind != yaml.MappingNode {
		return
	}
	s := strconv.FormatFloat(v, 'f', -1, 64)
	scalar := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!float", Value: s}
	idx := mappingIndex(m, key)
	if idx >= 0 {
		m.Content[idx+1] = scalar
		return
	}
	kn := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}
	m.Content = append(m.Content, kn, scalar)
}
