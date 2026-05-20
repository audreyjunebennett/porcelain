package server

import (
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/adminui/embed"
)

// configureAdminUIListenForEmbed passes the effective gateway listen address to the embed
// package so CHIMERA_ADMINUI_ROOT is only honored on loopback binds.
func configureAdminUIListenForEmbed(rt *Runtime, overlay *StatusOverlay) {
	listen := ""
	if overlay != nil && overlay.EffectiveListen != "" {
		listen = overlay.EffectiveListen
	} else if res, _, _ := rt.Snapshot(); res != nil {
		listen = res.ListenAddr()
	}
	if listen != "" {
		embed.SetGatewayListenAddr(listen)
	}
}
