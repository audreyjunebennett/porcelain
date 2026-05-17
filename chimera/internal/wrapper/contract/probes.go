package contract

import (
	"fmt"
	"strings"
)

type ReadinessProbe struct {
	Component  string
	Backend    string
	Method     string
	Path       string
	WantStatus int
}

func (p ReadinessProbe) Validate() error {
	if _, ok := AllowedComponents[p.Component]; !ok {
		return fmt.Errorf("unknown component %q", p.Component)
	}
	if _, ok := AllowedBackendNames[p.Backend]; !ok {
		return fmt.Errorf("unknown backend %q", p.Backend)
	}
	if strings.TrimSpace(p.Method) == "" {
		return fmt.Errorf("missing method")
	}
	if p.Path == "" || p.Path[0] != '/' {
		return fmt.Errorf("path must be absolute")
	}
	if p.WantStatus < 100 || p.WantStatus > 599 {
		return fmt.Errorf("invalid status code")
	}
	return nil
}

// InitialBinaryReadinessProbes defines Phase 1 probe lock for binary mode:
// readiness succeeds on HTTP 200 from the backend endpoint.
var InitialBinaryReadinessProbes = []ReadinessProbe{
	{
		Component:  ComponentGateway,
		Backend:    "custom",
		Method:     "GET",
		Path:       "/health",
		WantStatus: 200,
	},
	{
		Component:  ComponentBroker,
		Backend:    "bifrost",
		Method:     "GET",
		Path:       "/models",
		WantStatus: 200,
	},
	{
		Component:  ComponentVectorstore,
		Backend:    "qdrant",
		Method:     "GET",
		Path:       "/collections",
		WantStatus: 200,
	},
}
