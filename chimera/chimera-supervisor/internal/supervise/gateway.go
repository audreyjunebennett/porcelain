package supervise

import (
	"fmt"
	"strings"

	gwconfig "github.com/lynn/porcelain/chimera/internal/config"
)

func gatewayPublicURLFromResolved(res *gwconfig.Resolved) string {
	if res == nil {
		return "http://127.0.0.1:7710"
	}
	host := strings.TrimSpace(res.ListenHost)
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	if strings.Contains(host, ":") && !strings.HasPrefix(host, "[") {
		return fmt.Sprintf("http://[%s]:%d", host, res.ListenPort)
	}
	return fmt.Sprintf("http://%s:%d", host, res.ListenPort)
}
