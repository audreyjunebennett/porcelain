package catalogreview

import (
	"reflect"
	"testing"
	"time"

	"github.com/lynn/porcelain/chimera/internal/freecatalog"
	"github.com/lynn/porcelain/chimera/internal/providerfreetier"
	"github.com/lynn/porcelain/chimera/internal/providerlimits"
)

func TestBuildReportSeparatesAvailabilityCapabilityAndRemoval(t *testing.T) {
	now := time.Date(2026, 8, 29, 0, 0, 0, 0, time.UTC)
	live := []CatalogModel{
		{ID: "ollama/qwen3-vl:8b", ContextLength: 262144, Capabilities: []string{"completion", "tools"}},
		{ID: "ollama/nomic-embed-text:latest", ContextLength: 8192, Capabilities: []string{"embedding"}},
	}
	public := []freecatalog.Entry{
		{Provider: "groq", SourceID: "openai/gpt-oss-120b", BiFrostID: "groq/openai/gpt-oss-120b"},
	}
	active := &providerfreetier.Spec{FormatVersion: 1, Models: []string{"groq/retired-chat", "groq/openai/gpt-oss-120b"}, Patterns: []string{"ollama/*"}}
	report := BuildReport(now, "catalog", "groq", "gemini", "ready", live, public, active, nil, nil, nil)
	if report.Summary.GeneralCandidates != 1 {
		t.Fatalf("general candidates=%d", report.Summary.GeneralCandidates)
	}
	if !reflect.DeepEqual(report.Summary.RetiredOrRemoved, []string{"groq/retired-chat"}) {
		t.Fatalf("retired=%v", report.Summary.RetiredOrRemoved)
	}
	if !contains(report.Summary.SpecialistsExcluded, "ollama/nomic-embed-text:latest") {
		t.Fatalf("specialists=%v", report.Summary.SpecialistsExcluded)
	}
	if !contains(report.Summary.RecommendedForApproval, "verify the persisted Groq credential and provider health before treating absent Groq models as retired") {
		t.Fatalf("recommendations=%v", report.Summary.RecommendedForApproval)
	}
	for _, model := range report.Models {
		if model.ModelID == "groq/openai/gpt-oss-120b" && model.Availability != "not_in_live_account_catalog" {
			t.Fatalf("cloud availability conflated: %+v", model)
		}
	}
}

func TestApplyPublishedGroqLimitsOnlyExistingModels(t *testing.T) {
	cfg, err := providerlimits.Parse([]byte(`schema_version: 2
defaults:
  usage_day_timezone: UTC
providers:
  groq:
    models:
      groq/openai/gpt-oss-120b:
        rpm: 1
`))
	if err != nil {
		t.Fatal(err)
	}
	limits := freecatalog.GroqLimits{RPM: "30", RPD: "1K", TPM: "8K", TPD: "200K"}
	changes := ApplyPublishedGroqLimits(cfg, []freecatalog.Entry{
		{Provider: "groq", SourceID: "openai/gpt-oss-120b", BiFrostID: "groq/openai/gpt-oss-120b", Groq: &limits},
		{Provider: "groq", SourceID: "qwen/new", BiFrostID: "groq/qwen/new", Groq: &limits},
	})
	if len(changes) != 4 {
		t.Fatalf("changes=%+v", changes)
	}
	eff := cfg.Resolve("groq/openai/gpt-oss-120b")
	if eff.RPM == nil || *eff.RPM != 30 || eff.TPM == nil || *eff.TPM != 8000 {
		t.Fatalf("effective=%+v", eff)
	}
	if _, exists := cfg.Providers["groq"].Models["groq/qwen/new"]; exists {
		t.Fatal("public evidence must not create an unavailable account model")
	}
}

func TestParsePublishedCount(t *testing.T) {
	for raw, want := range map[string]int64{"14.4K": 14400, "2M": 2000000, "250": 250} {
		got, ok := ParsePublishedCount(raw)
		if !ok || got != want {
			t.Fatalf("%s: got %d ok=%v", raw, got, ok)
		}
	}
	if _, ok := ParsePublishedCount("-"); ok {
		t.Fatal("dash must be unset")
	}
}

func TestBuildCodingProposalUsesOnlyCuratedAvailableModels(t *testing.T) {
	now := time.Date(2026, 8, 29, 0, 0, 0, 0, time.UTC)
	live := []CatalogModel{
		{ID: "ollama/qwen3-vl:8b", ContextLength: 262144, Capabilities: []string{"completion", "tools"}},
		{ID: "gemini/antigravity-preview-05-2026", ContextLength: 196608},
		{ID: "gemini/aqa", ContextLength: 8192},
		{ID: "gemini/gemini-2.5-pro", ContextLength: 1114112},
		{ID: "gemini/gemini-2.5-flash", ContextLength: 1114112},
	}
	proposal := BuildCodingProposal(now, live, nil, nil)
	want := []string{"ollama/qwen3-vl:8b", "gemini/gemini-2.5-pro"}
	if !reflect.DeepEqual(proposal.EffectiveFallbackChain, want) {
		t.Fatalf("chain=%v want=%v", proposal.EffectiveFallbackChain, want)
	}
	for _, candidate := range proposal.Candidates {
		if candidate.ModelID == "gemini/antigravity-preview-05-2026" || candidate.ModelID == "gemini/aqa" {
			t.Fatalf("specialist leaked into coding candidates: %+v", candidate)
		}
	}
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
