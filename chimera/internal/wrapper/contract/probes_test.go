package contract

import "testing"

func TestInitialBinaryReadinessProbes_locked(t *testing.T) {
	t.Parallel()
	if len(InitialBinaryReadinessProbes) != 3 {
		t.Fatalf("expected 3 probes, got %d", len(InitialBinaryReadinessProbes))
	}
	var foundGateway, foundBroker, foundVector bool
	for _, p := range InitialBinaryReadinessProbes {
		if err := p.Validate(); err != nil {
			t.Fatalf("probe validate failed: %+v err=%v", p, err)
		}
		if p.Component == ComponentGateway {
			foundGateway = true
			if p.Path != "/health" || p.WantStatus != 200 || p.Method != "GET" {
				t.Fatalf("gateway probe mismatch: %+v", p)
			}
		}
		if p.Component == ComponentBroker {
			foundBroker = true
			if p.Path != "/models" || p.WantStatus != 200 || p.Method != "GET" {
				t.Fatalf("broker probe mismatch: %+v", p)
			}
		}
		if p.Component == ComponentVectorstore {
			foundVector = true
			if p.Path != "/collections" || p.WantStatus != 200 || p.Method != "GET" {
				t.Fatalf("vectorstore probe mismatch: %+v", p)
			}
		}
	}
	if !foundGateway || !foundBroker || !foundVector {
		t.Fatalf("missing probes gateway=%v broker=%v vector=%v", foundGateway, foundBroker, foundVector)
	}
}
