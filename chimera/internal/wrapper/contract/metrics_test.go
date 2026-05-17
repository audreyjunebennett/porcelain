package contract

import "testing"

func TestRequiredMetrics_locked(t *testing.T) {
	t.Parallel()
	want := map[string]bool{
		"chimera_wrapper_up":               false,
		"chimera_backend_up":               false,
		"chimera_backend_restarts_total":   false,
		"chimera_requests_total":           false,
		"chimera_request_duration_seconds": false,
	}
	for _, m := range RequiredMetrics {
		if _, ok := want[m]; ok {
			want[m] = true
		}
	}
	for k, seen := range want {
		if !seen {
			t.Fatalf("missing required metric: %s", k)
		}
	}
	if UpstreamMetricsPrefix != "upstream_" {
		t.Fatalf("unexpected upstream metrics prefix: %s", UpstreamMetricsPrefix)
	}
}
