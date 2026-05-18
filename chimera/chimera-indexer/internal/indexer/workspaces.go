package indexer

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// WorkspacesAPIResponse is the JSON body from GET /v1/indexer/workspaces.
type WorkspacesAPIResponse struct {
	Object     string              `json:"object"`
	Workspaces []WorkspaceAPIEntry `json:"workspaces"`
}

// WorkspaceAPIEntry is one workspace row from the gateway.
type WorkspaceAPIEntry struct {
	WorkspaceID int64              `json:"workspace_id"`
	ID          int64              `json:"id"`
	ProjectID   string             `json:"project_id"`
	FlavorID    string             `json:"flavor_id"`
	Paths       []WorkspacePathAPI `json:"paths"`
}

// WorkspacePathAPI is one watched directory under a workspace.
type WorkspacePathAPI struct {
	PathID int64  `json:"path_id"`
	ID     int64  `json:"id"`
	Path   string `json:"path"`
}

func (w *WorkspaceAPIEntry) effectiveWorkspaceID() int64 {
	if w.WorkspaceID != 0 {
		return w.WorkspaceID
	}
	return w.ID
}

func (p *WorkspacePathAPI) effectivePathID() int64 {
	if p.PathID != 0 {
		return p.PathID
	}
	return p.ID
}

// RetryPolicyFromResolved maps resolved indexer retry settings to HTTP client policy.
func RetryPolicyFromResolved(r Resolved) SessionRetryPolicy {
	return SessionRetryPolicy{
		MaxAttempts: r.RetryMaxAttempts,
		BaseDelay:   r.RetryBaseDelay,
		MaxDelay:    r.RetryMaxDelay,
	}
}

// FetchWorkspaces calls GET /v1/indexer/workspaces with bounded retries for transient errors.
func (c *GatewayClient) FetchWorkspaces(ctx context.Context, hdrs map[string]string, pol SessionRetryPolicy) (*WorkspacesAPIResponse, error) {
	if c == nil {
		return nil, fmt.Errorf("gateway client is nil")
	}
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	body, err := c.httpDoWithPolicy(ctx, http.MethodGet, apiPathIndexerWorkspaces, "", nil, hdrs, pol, rng)
	if err != nil {
		return nil, err
	}
	var out WorkspacesAPIResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("decode indexer workspaces: %w", err)
	}
	return &out, nil
}

// RootsFromWorkspacesResponse builds watch roots from a gateway workspaces payload.
// Each path must exist as a directory on this host (same rules as YAML roots).
func RootsFromWorkspacesResponse(resp *WorkspacesAPIResponse) ([]Root, error) {
	if resp == nil {
		return nil, nil
	}
	var roots []Root
	for wi, w := range resp.Workspaces {
		wid := w.effectiveWorkspaceID()
		if wid == 0 {
			return nil, fmt.Errorf("workspaces[%d]: missing workspace id", wi)
		}
		wsStr := strconv.FormatInt(wid, 10)
		if len(w.Paths) == 0 {
			continue
		}
		for pi, p := range w.Paths {
			pabs := strings.TrimSpace(p.Path)
			if pabs == "" {
				return nil, fmt.Errorf("workspace %d paths[%d]: empty path", wid, pi)
			}
			abs := filepath.Clean(pabs)
			st, err := os.Stat(abs)
			if err != nil {
				return nil, fmt.Errorf("workspace %d path %q: %w", wid, abs, err)
			}
			if !st.IsDir() {
				return nil, fmt.Errorf("workspace %d path %q is not a directory", wid, abs)
			}
			_ = p.effectivePathID()
			roots = append(roots, Root{
				ID:      rootSlug(abs),
				AbsPath: abs,
				Scope: ScopeFragment{
					ProjectID:   strings.TrimSpace(w.ProjectID),
					FlavorID:    strings.TrimSpace(w.FlavorID),
					WorkspaceID: wsStr,
				},
			})
		}
	}
	return roots, nil
}

// WorkspacesFingerprint returns a stable comma-separated string of sorted
// workspace IDs present in a Root slice. Used by the supervised workspace poll
// loop to detect when the active set changes between outer loop iterations.
func WorkspacesFingerprint(roots []Root) string {
	seen := make(map[string]bool, len(roots))
	for _, r := range roots {
		wid := strings.TrimSpace(r.Scope.WorkspaceID)
		if wid != "" {
			seen[wid] = true
		}
	}
	ids := make([]string, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return strings.Join(ids, ",")
}

// WorkspacesResponseFingerprint returns a stable comma-separated string of
// sorted workspace IDs from a raw API response. Only workspaces that have at
// least one path are counted, matching the behaviour of RootsFromWorkspacesResponse.
// Safe to call without materialising paths to disk.
func WorkspacesResponseFingerprint(resp *WorkspacesAPIResponse) string {
	if resp == nil {
		return ""
	}
	seen := make(map[string]bool, len(resp.Workspaces))
	for _, w := range resp.Workspaces {
		if len(w.Paths) == 0 {
			continue
		}
		wid := strconv.FormatInt(w.effectiveWorkspaceID(), 10)
		if wid != "0" {
			seen[wid] = true
		}
	}
	ids := make([]string, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return strings.Join(ids, ",")
}

// MaterializeRootsFromGateway fetches workspaces and replaces cfg.Roots.
func MaterializeRootsFromGateway(ctx context.Context, c *GatewayClient, cfg *Resolved, pol SessionRetryPolicy) error {
	if cfg == nil {
		return fmt.Errorf("resolved config is nil")
	}
	if !cfg.SupervisedLayer {
		return fmt.Errorf("MaterializeRootsFromGateway: not supervised layer")
	}
	resp, err := c.FetchWorkspaces(ctx, cfg.DefaultIndexerHeaders(), pol)
	if err != nil {
		return err
	}
	roots, err := RootsFromWorkspacesResponse(resp)
	if err != nil {
		return err
	}
	cfg.Roots = roots
	return nil
}
