package runtime

import (
	"github.com/lynn/porcelain/chimera/internal/tokens"
)

// BootstrapMode reports whether the gateway should serve the limited bootstrap surface
// (no valid rows in api-keys.yaml).
func BootstrapMode(rt *Runtime) bool {
	if rt == nil {
		return true
	}
	_, tokStore := rt.Snapshot()
	if tokStore == nil {
		return true
	}
	return tokens.IsBootstrapMode(tokStore.Path())
}
