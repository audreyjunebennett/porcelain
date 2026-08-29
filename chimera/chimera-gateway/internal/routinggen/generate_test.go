package routinggen

import (
	"testing"

	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/routing"
	"github.com/lynn/porcelain/chimera/internal/providerlimits"
)

func TestExtractCatalogModelIDs(t *testing.T) {
	raw := []byte(`{"data":[{"id":"Chimera-0.1.0"},{"id":"groq/a"},{"id":"groq/b"},{"id":"groq/a"}]}`)
	ids, err := ExtractCatalogModelIDs(raw, "Chimera-0.1.0")
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 2 || ids[0] != "groq/a" || ids[1] != "groq/b" {
		t.Fatalf("%#v", ids)
	}
}

func TestOrderFallbackChain_localFirstThenLexical(t *testing.T) {
	in := []string{"ollama/small", "groq/llama-3.3-70b-versatile", "groq/llama-3.1-8b-instant"}
	out := OrderFallbackChain(in)
	if out[0] != "ollama/small" {
		t.Fatalf("want local model first, got %v", out)
	}
	if out[1] != "groq/llama-3.1-8b-instant" || out[2] != "groq/llama-3.3-70b-versatile" {
		t.Fatalf("want lexical cloud remainder, got %v", out)
	}
}

func TestOrderRouterModels_prefersSmallFastOverLarge(t *testing.T) {
	in := []string{"groq/llama-3.3-70b-versatile", "groq/llama-3.1-8b-instant"}
	out := OrderRouterModels(in, nil)
	if len(out) != 2 || out[0] != "groq/llama-3.1-8b-instant" {
		t.Fatalf("want 8b instant first for router, got %v", out)
	}
}

func TestOrderRouterModels_rpmFromLimits(t *testing.T) {
	raw := `schema_version: 1
providers:
  groq:
    usage_day_timezone: UTC
    rpm: 10
    tpm: 1000
    models:
      groq/small:
        rpm: 500
        tpm: 2000
      groq/tiny:
        rpm: 50
        tpm: 2000
`
	cfg, err := providerlimits.Parse([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	in := []string{"groq/small", "groq/tiny"}
	out := OrderRouterModels(in, cfg)
	if len(out) != 2 || out[0] != "groq/small" {
		t.Fatalf("want higher RPM model first, got %v", out)
	}
}

func TestBuildRoutingPolicyYAML_validates(t *testing.T) {
	b, err := BuildRoutingPolicyYAML([]string{"gemini/x", "groq/y"})
	if err != nil {
		t.Fatal(err)
	}
	if err := routing.ValidatePolicyYAML(b); err != nil {
		t.Fatal(err)
	}
}

func TestPickLongTurnModelPreservesOperatorPreference(t *testing.T) {
	got := PickLongTurnModel([]string{"ollama/qwen:8b", "groq/openai/gpt-oss-120b"})
	if got != "ollama/qwen:8b" {
		t.Fatalf("got %q", got)
	}
}
