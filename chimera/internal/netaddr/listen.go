package netaddr

import (
	"strings"

	"github.com/lynn/porcelain/chimera/internal/config"
)

// ListenAddrOverride applies -listen flag semantics: "host:port" or ":port".
func ListenAddrOverride(res *config.Resolved, listenFlag string) string {
	if strings.TrimSpace(listenFlag) == "" {
		return res.ListenAddr()
	}
	if strings.HasPrefix(listenFlag, ":") {
		return res.ListenHost + listenFlag
	}
	return listenFlag
}
