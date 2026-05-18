package supervise

import (
	"os"
	"strings"

	"github.com/lynn/porcelain/internal/naming"
)

// ChildEnv is applied to wrapper and indexer children started by the supervisor.
func ChildEnv(controlBaseURL string) map[string]string {
	out := map[string]string{
		"CHIMERA_LOG_JSON":   "1",
		"CHIMERA_SUPERVISED": "1",
	}
	if u := strings.TrimSpace(controlBaseURL); u != "" {
		out[naming.EnvSupervisorControlURLTarget] = u
	}
	return out
}

// WrapperArgs are appended to chimera-gateway, chimera-broker, and chimera-vectorstore.
func WrapperArgs(base []string) []string {
	out := append([]string(nil), base...)
	out = append(out, "-debug-forward-upstream")
	return out
}

func mergeEnv(overrides map[string]string) []string {
	m := make(map[string]string)
	for _, e := range os.Environ() {
		i := strings.IndexByte(e, '=')
		if i <= 0 {
			continue
		}
		m[e[:i]] = e[i+1:]
	}
	for k, v := range overrides {
		m[k] = v
	}
	out := make([]string, 0, len(m))
	for k, v := range m {
		out = append(out, k+"="+v)
	}
	return out
}
