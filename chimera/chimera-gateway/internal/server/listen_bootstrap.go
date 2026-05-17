package server

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/lynn/porcelain/chimera/internal/config"
	"github.com/lynn/porcelain/chimera/internal/netaddr"
)

// BootstrapListenPort returns the TCP port used for bootstrap loopback listeners.
// It respects -listen when it specifies a port (e.g. :4000 or host:4000).
func BootstrapListenPort(res *config.Resolved, listenFlag string) int {
	if res == nil {
		return 3000
	}
	addr := netaddr.ListenAddrOverride(res, listenFlag)
	_, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return res.ListenPort
	}
	p, err := strconv.Atoi(portStr)
	if err != nil || p <= 0 {
		return res.ListenPort
	}
	return p
}

// BootstrapTCPAddrs returns loopback addresses for dual-stack bootstrap (IPv4 + IPv6).
func BootstrapTCPAddrs(res *config.Resolved, listenFlag string) []string {
	p := BootstrapListenPort(res, listenFlag)
	return []string{
		fmt.Sprintf("127.0.0.1:%d", p),
		fmt.Sprintf("[::1]:%d", p),
	}
}

// IsIPv6LoopbackAddr reports whether addr is the [::1]:port form.
func IsIPv6LoopbackAddr(addr string) bool {
	return strings.HasPrefix(addr, "[::1]:")
}
