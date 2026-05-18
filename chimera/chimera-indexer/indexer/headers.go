package indexer

import (
	"strings"

	"github.com/lynn/porcelain/internal/naming"
)

// ScopeHTTPHeaders builds optional Chimera scope headers for gateway indexer APIs.
// Returns nil when both project and flavor are empty.
func ScopeHTTPHeaders(project, flavor string) map[string]string {
	p := strings.TrimSpace(project)
	f := strings.TrimSpace(flavor)
	if p == "" && f == "" {
		return nil
	}
	m := map[string]string{}
	if p != "" {
		m[naming.HeaderProjectTarget] = p
	}
	if f != "" {
		m[naming.HeaderFlavorTarget] = f
	}
	return m
}
