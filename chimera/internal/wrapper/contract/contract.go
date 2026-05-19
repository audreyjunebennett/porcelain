package contract

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/lynn/porcelain/internal/naming"
)

const (
	ComponentVectorstore = naming.ProductVectorstoreName
	ComponentBroker      = naming.ProductBrokerName
	ComponentSupervisor  = naming.ProductSupervisorName
	ComponentGateway     = naming.ProductGatewayBinName
	ComponentIndexer     = naming.ProductIndexerBinName
)

var AllowedComponents = map[string]struct{}{
	ComponentVectorstore: {},
	ComponentBroker:      {},
	ComponentSupervisor:  {},
	ComponentGateway:     {},
	ComponentIndexer:     {},
}

var AllowedBackendNames = map[string]struct{}{
	naming.ProductQdrantBinName: {},
	"bifrost":                   {}, // legacy alias; new broker wrapper reports chimera-broker
	naming.ProductBrokerName:    {},
	"milvus":                    {},
	"weaviate":                  {},
	"redis_vector":              {},
	"custom":                    {},
}

var AllowedBackendModes = map[string]struct{}{
	"binary":   {},
	"docker":   {},
	"remote":   {},
	"embedded": {},
}

var AllowedStatus = map[string]struct{}{
	"ok":       {},
	"degraded": {},
	"error":    {},
}

const (
	ReadyPath   = "/readyz"
	HealthPath  = "/healthz"
	MetricsPath = "/metrics"
)

const (
	DebugBrokerLogsPath              = "/debug/broker/logs"
	DebugVectorstoreLogsPath         = "/debug/vectorstore/logs"
	DebugEnableBrokerLogsEnvKey      = "DEBUG__ENABLE_BROKER_LOGS"
	DebugEnableVectorstoreLogsEnvKey = "DEBUG__ENABLE_VECTORSTORE_LOGS"
	DebugAllowRemoteEnv              = "DEBUG__ALLOW_REMOTE"
	DebugAllowRemoteFlag             = "--debug-allow-remote"
)

const (
	ExitClean          = 0
	ExitConfigError    = 10
	ExitBackendStartup = 20
	ExitBackendRuntime = 30
	ExitDependency     = 40
	ExitInternal       = 50
)

const (
	ErrorClassConfig         = "CONFIG_ERROR"
	ErrorClassBackendStartup = "BACKEND_STARTUP_ERROR"
	ErrorClassBackendRuntime = "BACKEND_RUNTIME_ERROR"
	ErrorClassDependency     = "DEPENDENCY_ERROR"
	ErrorClassInternal       = "INTERNAL_ERROR"
)

const (
	DefaultStartupTimeout  = 30 * time.Second
	DefaultShutdownTimeout = 15 * time.Second
	DefaultTerminateWait   = 10 * time.Second

	DefaultBackoffInitial    = 1 * time.Second
	DefaultBackoffMultiplier = 2.0
	DefaultBackoffMax        = 30 * time.Second
	DefaultBackoffResetAfter = 60 * time.Second
	DefaultBackoffMaxRetries = -1 // -1 means infinite retries
)

const (
	DefaultDebugRingBufferMaxLines = 10_000
	DefaultDebugRingBufferMaxBytes = 1_000_000
)

// LegacyCompatibilitySupported is intentionally false for wrapper contracts.
// Phase 1 hard-cuts upstream compatibility flags/env from wrapper public contract.
const LegacyCompatibilitySupported = false

var RedactedSecretTokens = []string{"TOKEN", "KEY", "PASSWORD", "SECRET"}

var AllowedEndpointMetricLabels = map[string]string{
	"/healthz":                "healthz",
	"/readyz":                 "readyz",
	"/metrics":                "metrics",
	"/debug/broker/logs":      "debug_broker_logs",
	"/debug/vectorstore/logs": "debug_vectorstore_logs",
}

// DebugLogsPath returns the ring-buffer debug endpoint for a wrapper component.
func DebugLogsPath(component string) string {
	switch component {
	case ComponentVectorstore:
		return DebugVectorstoreLogsPath
	default:
		return DebugBrokerLogsPath
	}
}

// DebugEnableEnvKey returns the env var that gates debug log endpoints for component.
func DebugEnableEnvKey(component string) string {
	switch component {
	case ComponentVectorstore:
		return DebugEnableVectorstoreLogsEnvKey
	default:
		return DebugEnableBrokerLogsEnvKey
	}
}

type Version struct {
	Wrapper  string `json:"wrapper"`
	Upstream string `json:"upstream,omitempty"` // empty string means unknown
	BuildSHA string `json:"build_sha,omitempty"`
}

type StatusPayload struct {
	Component   string         `json:"component"`
	BackendName string         `json:"backend_name"`
	BackendMode string         `json:"backend_mode"`
	Status      string         `json:"status"`
	Timestamp   time.Time      `json:"timestamp"`
	Version     Version        `json:"version"`
	Message     string         `json:"message,omitempty"`
	Endpoint    string         `json:"endpoint,omitempty"`
	PID         *int           `json:"pid,omitempty"`
	Restarts    *int           `json:"restarts,omitempty"`
	LastError   string         `json:"last_error,omitempty"`
	Details     map[string]any `json:"details,omitempty"`
}

func (s StatusPayload) Validate() error {
	var errs []string
	if _, ok := AllowedComponents[s.Component]; !ok {
		errs = append(errs, "invalid component")
	}
	if _, ok := AllowedBackendNames[s.BackendName]; !ok {
		errs = append(errs, "invalid backend_name")
	}
	if _, ok := AllowedBackendModes[s.BackendMode]; !ok {
		errs = append(errs, "invalid backend_mode")
	}
	if _, ok := AllowedStatus[s.Status]; !ok {
		errs = append(errs, "invalid status")
	}
	if s.Timestamp.IsZero() {
		errs = append(errs, "missing timestamp")
	}
	if strings.TrimSpace(s.Version.Wrapper) == "" {
		errs = append(errs, "missing version.wrapper")
	}
	if s.Restarts != nil && *s.Restarts < 0 {
		errs = append(errs, "restarts must be >= 0")
	}
	if len(errs) == 0 {
		return nil
	}
	sort.Strings(errs)
	return fmt.Errorf("status payload invalid: %s", strings.Join(errs, ", "))
}

func ReadyLogLine(component, backend, mode, version, upstream string) string {
	return fmt.Sprintf("READY: component=<%s> backend=<%s> mode=<%s> version=<%s> upstream=<%s>", component, backend, mode, version, upstream)
}

func EndpointMetricLabel(path string) (string, bool) {
	v, ok := AllowedEndpointMetricLabels[path]
	return v, ok
}

func DebugMustBindLoopback(allowRemote bool) bool {
	return !allowRemote
}

func IsDebugPath(path string) bool {
	return strings.HasPrefix(path, "/debug/")
}
