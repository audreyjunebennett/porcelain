package operatorapi

// IndexerConfigResponse is GET /api/ui/indexer/config.
type IndexerConfigResponse struct {
	Path              string           `json:"path"`
	YAML              string           `json:"yaml"`
	Roots             []map[string]any `json:"roots"`
	Workspaces        []map[string]any `json:"workspaces"`
	SupervisedEnabled bool             `json:"supervised_enabled"`
	OperatorStore     bool             `json:"operator_store"`
}

// IndexerConfigPutRequest is PUT /api/ui/indexer/config body.
type IndexerConfigPutRequest struct {
	YAML string `json:"yaml"`
}

// IndexerConfigPutResponse is PUT /api/ui/indexer/config success JSON.
type IndexerConfigPutResponse struct {
	OK   bool   `json:"ok"`
	Path string `json:"path"`
}

// IndexerWorkspacesResponse is GET /api/ui/indexer/workspaces.
type IndexerWorkspacesResponse struct {
	Workspaces []map[string]any `json:"workspaces"`
}

// IndexerWorkspaceCreateRequest is POST /api/ui/indexer/workspaces body.
type IndexerWorkspaceCreateRequest struct {
	ProjectID string   `json:"project_id"`
	FlavorID  string   `json:"flavor_id"`
	Paths     []string `json:"paths"`
}

// IndexerWorkspaceCreateResponse is POST /api/ui/indexer/workspaces success JSON.
type IndexerWorkspaceCreateResponse struct {
	OK        bool             `json:"ok"`
	Workspace map[string]any   `json:"workspace"`
	Roots     []map[string]any `json:"roots"`
}

// IndexerWorkspaceUpdateRequest is PUT /api/ui/indexer/workspaces/{id} body.
type IndexerWorkspaceUpdateRequest struct {
	ProjectID string `json:"project_id"`
	FlavorID  string `json:"flavor_id"`
}

// IndexerWorkspaceUpdateResponse is PUT /api/ui/indexer/workspaces/{id} success JSON.
type IndexerWorkspaceUpdateResponse struct {
	OK    bool             `json:"ok"`
	Roots []map[string]any `json:"roots"`
}

// IndexerRootsResponse is shared by several workspace/path mutations.
type IndexerRootsResponse struct {
	OK    bool             `json:"ok"`
	Roots []map[string]any `json:"roots"`
}

// IndexerWorkspacePathCreateRequest is POST .../workspaces/{id}/paths body.
type IndexerWorkspacePathCreateRequest struct {
	Path string `json:"path"`
}

// IndexerWorkspacePathUpdateRequest is PUT /api/ui/indexer/workspace-paths/{pathid} body.
type IndexerWorkspacePathUpdateRequest struct {
	Path      *string `json:"path"`
	ProjectID *string `json:"project_id"`
	FlavorID  *string `json:"flavor_id"`
}
