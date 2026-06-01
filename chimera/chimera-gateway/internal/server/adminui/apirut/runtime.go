package apirut

import (
	"context"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/brokeradmin"
	gruntime "github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/runtime"
)

// BrokerProviderNames is the operator UI provider catalog roster (display order).
var BrokerProviderNames = ProviderCatalogIDs()

// BrokerProviderNamesForProbes returns provider ids registered in chimera-broker for state and
// health BFF endpoints. Uses governance when available, otherwise per-provider GET discovery.
func BrokerProviderNamesForProbes(ctx context.Context, client *brokeradmin.Client) []string {
	configured, listOK := brokeradmin.ListConfiguredProviders(ctx, client)
	return ConfiguredProviderIDsResolved(ctx, client, configured, listOK)
}

// BrokerProviderNamesForHealth is an alias for BrokerProviderNamesForProbes (health strip shows
// configured providers only).
func BrokerProviderNamesForHealth(ctx context.Context, client *brokeradmin.Client) []string {
	return BrokerProviderNamesForProbes(ctx, client)
}

// BrokerAdminClient returns a chimera-broker management API client from runtime config.
func BrokerAdminClient(rt *gruntime.Runtime) *brokeradmin.Client {
	rt.Sync()
	res, _ := rt.Snapshot()
	if res == nil {
		return &brokeradmin.Client{}
	}
	tok := ""
	if res.UpstreamAPIKeyEnv != "" {
		tok = strings.TrimSpace(os.Getenv(res.UpstreamAPIKeyEnv))
	}
	return &brokeradmin.Client{
		BaseURL:     res.UpstreamBaseURL,
		BearerToken: tok,
		HTTPClient:  &http.Client{Timeout: 8 * time.Second},
	}
}

// FormatRFC3339OrEmpty formats t in RFC3339 UTC, or "" when zero.
func FormatRFC3339OrEmpty(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// PublicGatewayBase returns the operator-visible gateway base URL for the request.
func PublicGatewayBase(r *http.Request) string {
	host := strings.TrimSpace(r.Host)
	if host == "" {
		return "http://127.0.0.1:3000"
	}
	return "http://" + host
}

// BootstrapLocked is true when no valid gateway tokens exist (admin token APIs unavailable).
func BootstrapLocked(rt *gruntime.Runtime) bool {
	return gruntime.BootstrapMode(rt)
}
