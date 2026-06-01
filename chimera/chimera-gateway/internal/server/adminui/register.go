package adminui

import (
	"log/slog"
	"net/http"

	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/adminui/api/auth"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/adminui/api/conversations"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/adminui/api/indexer"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/adminui/api/logs"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/adminui/api/metrics"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/adminui/api/providers"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/adminui/api/rag"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/adminui/api/save"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/adminui/api/state"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/adminui/api/tokens"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/adminui/api/virtualmodels"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/adminui/embed"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/adminui/handler"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/adminui/session"
	gruntime "github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/runtime"
)

// Register wires operator UI routes (/ui, /api/ui/*) on mux.
func Register(mux *http.ServeMux, rt *gruntime.Runtime, log *slog.Logger, ui *session.UIOptions) {
	h := handler.New(rt, log, ui)
	if h == nil {
		return
	}
	auth.Register(mux, h)
	embed.Register(mux, h)
	state.Register(mux, h)
	metrics.Register(mux, h)
	providers.Register(mux, h)
	save.Register(mux, h)
	tokens.Register(mux, h)
	indexer.Register(mux, h)
	rag.Register(mux, h)
	virtualmodels.Register(mux, h)
	conversations.Register(mux, h)
	logs.Register(mux, h)
}
