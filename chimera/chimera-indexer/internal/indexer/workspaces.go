package indexer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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

// rootsSnapshotTuple is one stable row in the supervised workspace snapshot.
func rootsSnapshotTuple(workspaceID int64, absPath, projectID, flavorID string) string {
	return strings.Join([]string{
		strconv.FormatInt(workspaceID, 10),
		filepath.Clean(strings.TrimSpace(absPath)),
		strings.TrimSpace(projectID),
		strings.TrimSpace(flavorID),
	}, "\x1f")
}

func hashRootsSnapshot(tuples []string) string {
	if len(tuples) == 0 {
		return ""
	}
	sorted := append([]string(nil), tuples...)
	sort.Strings(sorted)
	sum := sha256.Sum256([]byte(strings.Join(sorted, "\n")))
	return hex.EncodeToString(sum[:6])
}

func rootsSnapshotTuplesFromResponse(resp *WorkspacesAPIResponse) []string {
	if resp == nil {
		return nil
	}
	var tuples []string
	for _, w := range resp.Workspaces {
		if len(w.Paths) == 0 {
			continue
		}
		wid := w.effectiveWorkspaceID()
		if wid == 0 {
			continue
		}
		for _, p := range w.Paths {
			pabs := strings.TrimSpace(p.Path)
			if pabs == "" {
				continue
			}
			tuples = append(tuples, rootsSnapshotTuple(
				wid,
				pabs,
				w.ProjectID,
				w.FlavorID,
			))
		}
	}
	return tuples
}

func rootsSnapshotTuplesFromRoots(roots []Root) []string {
	if len(roots) == 0 {
		return nil
	}
	tuples := make([]string, 0, len(roots))
	for _, r := range roots {
		wid, _ := strconv.ParseInt(strings.TrimSpace(r.Scope.WorkspaceID), 10, 64)
		if wid == 0 {
			continue
		}
		tuples = append(tuples, rootsSnapshotTuple(
			wid,
			r.AbsPath,
			r.Scope.ProjectID,
			r.Scope.FlavorID,
		))
	}
	return tuples
}

// WatchRootPathsFromResponse returns sorted unique absolute paths from a workspaces payload.
func WatchRootPathsFromResponse(resp *WorkspacesAPIResponse) []string {
	if resp == nil {
		return nil
	}
	seen := make(map[string]bool)
	var out []string
	for _, w := range resp.Workspaces {
		for _, p := range w.Paths {
			pabs := filepath.Clean(strings.TrimSpace(p.Path))
			if pabs == "" || seen[pabs] {
				continue
			}
			seen[pabs] = true
			out = append(out, pabs)
		}
	}
	sort.Strings(out)
	return out
}

// WorkspacesRootsFingerprint returns a stable short hex hash of the gateway
// workspace snapshot, including every watched path. Used by the supervised poll
// loop to detect add/remove/modify path operations within an existing workspace.
func WorkspacesRootsFingerprint(resp *WorkspacesAPIResponse) string {
	return hashRootsSnapshot(rootsSnapshotTuplesFromResponse(resp))
}

// RootsSnapshotFingerprint returns the same hash format as
// WorkspacesRootsFingerprint for materialised watch roots.
func RootsSnapshotFingerprint(roots []Root) string {
	return hashRootsSnapshot(rootsSnapshotTuplesFromRoots(roots))
}

// WorkspaceIDsFromResponse returns a stable comma-separated list of workspace
// ids that have at least one configured path.
func WorkspaceIDsFromResponse(resp *WorkspacesAPIResponse) string {
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

// WorkspaceIDsFromRoots returns sorted workspace ids present in materialised roots.
func WorkspaceIDsFromRoots(roots []Root) string {
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

// WorkspacesFingerprint returns a stable roots snapshot hash for materialised
// watch roots. Prefer RootsSnapshotFingerprint for new call sites.
func WorkspacesFingerprint(roots []Root) string {
	return RootsSnapshotFingerprint(roots)
}

// WorkspacesResponseFingerprint returns a stable roots snapshot hash from a
// raw gateway workspaces payload. Prefer WorkspacesRootsFingerprint for new call sites.
func WorkspacesResponseFingerprint(resp *WorkspacesAPIResponse) string {
	return WorkspacesRootsFingerprint(resp)
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
