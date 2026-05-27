package naming

import "testing"

func TestLogsUIServiceDisplayLabel(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in, want string
	}{
		{"chimera-broker", "broker"},
		{"chimera-gateway", "gateway"},
		{"web", "web"},
		{"routing", "routing"},
		{"", ""},
	}
	for _, tc := range cases {
		if got := LogsUIServiceDisplayLabel(tc.in); got != tc.want {
			t.Errorf("LogsUIServiceDisplayLabel(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
