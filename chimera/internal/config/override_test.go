package config

import "testing"

func TestPatchResolvedUpstream(t *testing.T) {
	r := &Resolved{
		UpstreamBaseURL:   "http://chimera-broker:8080",
		HealthUpstreamURL: "http://chimera-broker:8080/health",
	}
	PatchResolvedUpstream(r, "http://127.0.0.1:9090")
	if r.UpstreamBaseURL != "http://127.0.0.1:9090" || r.HealthUpstreamURL != "http://127.0.0.1:9090/health" {
		t.Fatalf("%+v", r)
	}
}

func TestCloneResolved_Slice(t *testing.T) {
	a := &Resolved{
		FallbackChain: []string{"x", "y"},
		RouterModels:  []string{"groq/a"},
	}
	b := CloneResolved(a)
	b.FallbackChain[0] = "z"
	b.RouterModels[0] = "gemini/b"
	if a.FallbackChain[0] != "x" {
		t.Fatal("aliased slice")
	}
	if a.RouterModels[0] != "groq/a" {
		t.Fatal("aliased router_models slice")
	}
}
