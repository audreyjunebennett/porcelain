package config

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPatchGatewayYAMLBytesWithFilterFreeTierModels(t *testing.T) {
	raw := []byte(`gateway:
  semver: "0.1.0"
upstream:
  base_url: "http://127.0.0.1:8080"
paths:
  tokens: "./tokens.yaml"
  routing_policy: "./routing-policy.yaml"
routing:
  filter_free_tier_models: false
  fallback_chain:
    - "x"
`)
	out, err := PatchGatewayYAMLBytesWithFilterFreeTierModels(raw, true)
	if err != nil {
		t.Fatal(err)
	}
	res, err := LoadGatewayYAML(writeTempGateway(t, out), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !res.FilterFreeTierModels {
		t.Fatal("expected true")
	}
	out2, err := PatchGatewayYAMLBytesWithFilterFreeTierModels(out, false)
	if err != nil {
		t.Fatal(err)
	}
	res2, err := LoadGatewayYAML(writeTempGateway(t, out2), nil)
	if err != nil {
		t.Fatal(err)
	}
	if res2.FilterFreeTierModels {
		t.Fatal("expected false")
	}
}

func writeTempGateway(t *testing.T, raw []byte) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "gateway.yaml")
	if err := os.WriteFile(p, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestPatchGatewayYAMLBytesWithRouterModels_only(t *testing.T) {
	raw := []byte(`gateway:
  semver: "0.1.0"
routing:
  fallback_chain:
    - "x"
  tool_router:
    enabled: false
    confidence_threshold: 0.3
`)
	out, err := PatchGatewayYAMLBytesWithRouterModels(raw, []string{"groq/a"})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(out, []byte(`"groq/a"`)) {
		t.Fatalf("%s", out)
	}
	// tool_router block should still be present (not stripped by encoder)
	p := writeTempGateway(t, out)
	res, err := LoadGatewayYAML(p, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.RouterModels) != 1 || res.RouterModels[0] != "groq/a" {
		t.Fatalf("%#v", res.RouterModels)
	}
	if res.ToolRouterEnabled {
		t.Fatal("router_models alone should not force tool_router on when list non-empty and yaml said enabled false")
	}
}

func TestPatchGatewayYAMLBytesWithRouterTooling(t *testing.T) {
	raw := []byte(`gateway:
  semver: "0.1.0"
routing:
  fallback_chain:
    - "x"
`)
	out, err := PatchGatewayYAMLBytesWithRouterTooling(raw, []string{"groq/a", "gemini/b"}, true, 0.42)
	if err != nil {
		t.Fatal(err)
	}
	p := writeTempGateway(t, out)
	res, err := LoadGatewayYAML(p, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.RouterModels) != 2 || res.RouterModels[0] != "groq/a" {
		t.Fatalf("%#v", res.RouterModels)
	}
	if !res.ToolRouterEnabled {
		t.Fatal("expected tool router on")
	}
	if res.ToolRouterConfidenceThreshold < 0.41 || res.ToolRouterConfidenceThreshold > 0.43 {
		t.Fatalf("threshold %v", res.ToolRouterConfidenceThreshold)
	}
}

func TestPatchGatewayYAMLBytesWithFallbackChain(t *testing.T) {
	raw := []byte(`gateway:
  semver: "0.1.0"
routing:
  fallback_chain:
    - "old"
`)
	out, err := PatchGatewayYAMLBytesWithFallbackChain(raw, []string{"a", "b"})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(out, []byte(`"a"`)) || !bytes.Contains(out, []byte(`"b"`)) || bytes.Contains(out, []byte(`"old"`)) {
		t.Fatalf("%s", out)
	}
}

func TestPatchGatewayYAMLBytesWithEnsembleEnabled(t *testing.T) {
	raw := []byte(`gateway:
  semver: "0.1.0"
routing:
  fallback_chain:
    - "x"
`)
	out, err := PatchGatewayYAMLBytesWithEnsembleEnabled(raw, true)
	if err != nil {
		t.Fatal(err)
	}
	res, err := LoadGatewayYAML(writeTempGateway(t, out), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !res.EnsembleEnabled {
		t.Fatal("expected ensemble enabled")
	}
}

func TestLoadGatewayYAML_filterFreeTierDefaultTrueWhenOmitted(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "gateway.yaml")
	raw := `gateway:
  semver: "0.1.0"
upstream:
  base_url: "http://127.0.0.1:8080"
paths:
  tokens: "./tokens.yaml"
  routing_policy: "./routing-policy.yaml"
routing:
  fallback_chain:
    - "groq/x"
`
	if err := os.WriteFile(p, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := LoadGatewayYAML(p, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !res.FilterFreeTierModels {
		t.Fatal("expected default true when filter_free_tier_models omitted")
	}
	rawFalse := strings.Replace(raw, `routing:
  fallback_chain:`, `routing:
  filter_free_tier_models: false
  fallback_chain:`, 1)
	if err := os.WriteFile(p, []byte(rawFalse), 0o644); err != nil {
		t.Fatal(err)
	}
	res2, err := LoadGatewayYAML(p, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res2.FilterFreeTierModels {
		t.Fatal("expected explicit false")
	}
}

func TestWriteGatewayFallbackChain_roundTrip(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "gateway.yaml")
	raw := `gateway:
  semver: "0.1.0"
  listen_port: 3000
  listen_host: "127.0.0.1"
upstream:
  base_url: "http://127.0.0.1:8080"
  api_key_env: "CLAUDIA_UPSTREAM_API_KEY"
paths:
  tokens: "./tokens.yaml"
  routing_policy: "./routing-policy.yaml"
routing:
  fallback_chain:
    - "old"
`
	if err := os.WriteFile(p, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	chain := []string{"groq/a", "gemini/b"}
	if err := WriteGatewayFallbackChain(p, chain); err != nil {
		t.Fatal(err)
	}
	res, err := LoadGatewayYAML(p, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.FallbackChain) != 2 || res.FallbackChain[0] != "groq/a" {
		t.Fatalf("%#v", res.FallbackChain)
	}
}

func TestWriteGatewayEnsembleEnabled_roundTrip(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "gateway.yaml")
	raw := `gateway:
  semver: "0.1.0"
  listen_port: 3000
  listen_host: "127.0.0.1"
upstream:
  base_url: "http://127.0.0.1:8080"
  api_key_env: "CLAUDIA_UPSTREAM_API_KEY"
paths:
  tokens: "./tokens.yaml"
  routing_policy: "./routing-policy.yaml"
routing:
  fallback_chain:
    - "old"
`
	if err := os.WriteFile(p, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WriteGatewayEnsembleEnabled(p, true); err != nil {
		t.Fatal(err)
	}
	res, err := LoadGatewayYAML(p, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !res.EnsembleEnabled {
		t.Fatal("expected ensemble enabled")
	}
}
