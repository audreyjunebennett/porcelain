package server

import "testing"

func TestTimelineKindForGatewayHTTPPath(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{"/v1/ingest", "indexer"},
		{"/v1/ingest/session", "indexer"},
		{"/v1/ingest/session/x/chunk", "indexer"},
		{"/v1/indexer/config", "indexer"},
		{"/v1/chat/completions", "upstream"},
		{"/v1/models", "upstream"},
		{"/ui/models", "upstream"},
		{"/health", "web"},
		{"/api/ui/logs", "web"},
		{"/collections/foo/points/search", "qdrant"},
		{"", "web"},
	}
	for _, tc := range cases {
		if got := timelineKindForGatewayHTTPPath(tc.path); got != tc.want {
			t.Fatalf("path=%q: got %q want %q", tc.path, got, tc.want)
		}
	}
}
