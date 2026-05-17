package contract

var RequiredMetrics = []string{
	"chimera_wrapper_up",
	"chimera_backend_up",
	"chimera_backend_restarts_total",
	"chimera_requests_total",
	"chimera_request_duration_seconds",
}

const UpstreamMetricsPrefix = "upstream_"
