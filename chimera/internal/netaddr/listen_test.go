package netaddr

import (
	"testing"

	"github.com/lynn/porcelain/chimera/internal/config"
)

func TestListenAddrOverride(t *testing.T) {
	r := &config.Resolved{ListenHost: "127.0.0.1", ListenPort: 3000}
	if ListenAddrOverride(r, "") != "127.0.0.1:3000" {
		t.Fatal(ListenAddrOverride(r, ""))
	}
	if ListenAddrOverride(r, ":4000") != "127.0.0.1:4000" {
		t.Fatal()
	}
	if ListenAddrOverride(r, "0.0.0.0:9") != "0.0.0.0:9" {
		t.Fatal()
	}
}
