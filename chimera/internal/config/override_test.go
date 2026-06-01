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

func TestCloneResolved_isolatedCopy(t *testing.T) {
	a := &Resolved{Semver: "0.1.0", WitnessSampleMaxChars: 128}
	b := CloneResolved(a)
	b.WitnessSampleMaxChars = 256
	if a.WitnessSampleMaxChars != 128 {
		t.Fatal("aliased scalar")
	}
}
