// Package indexer implements the workspace file indexer for Chimera.
//
// Scope per docs/plans/indexer.plan.md:
package indexer

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/lynn/porcelain/internal/naming"
	"gopkg.in/yaml.v3"
)

const (
	// Defaults aligned with docs/plans/indexer.plan.md § Failure handling.
	defaultRetryAttempts        = 5
	defaultRetryBaseDelay       = 500 * time.Millisecond
	defaultRetryMaxDelay        = 30 * time.Second
	defaultRecoveryPollInterval = 30 * time.Second

	defaultDebounce       = 750 * time.Millisecond
	defaultWorkers        = 4
	defaultQueueDepth     = 1024
	defaultMaxFileBytes   = int64(8 * 1024 * 1024)
	defaultRequestTimeout = 60 * time.Second

	defaultStorageStatsPoll     = 2 * time.Minute
	defaultScopeStatusPoll      = 45 * time.Second
	defaultScopeActiveFileMinMs = 2000

	defaultQueueFanoutHWMPercent = 75
)

const (
	// IndexerConfigFileName is the layered YAML basename under .locus (and explicit --config paths).
	IndexerConfigFileName = "indexer.config.yaml"
	// IndexerSyncStateFileName is the default configured sync-state path (legacy .json name).
	// OpenSyncState stores checkpoints in sync-state.sqlite beside this path.
	IndexerSyncStateFileName = "indexer.sync-state.json"

	indexerHiddenStateDir = naming.IndexerHiddenStateDirTarget
)

// HiddenIndexerConfigPath returns dir/.locus/indexer.config.yaml.
func HiddenIndexerConfigPath(dir string) string {
	return filepath.Join(dir, indexerHiddenStateDir, IndexerConfigFileName)
}

// HiddenIndexerSyncStatePath returns dir/.locus/indexer.sync-state.json.
func HiddenIndexerSyncStatePath(dir string) string {
	return filepath.Join(dir, indexerHiddenStateDir, IndexerSyncStateFileName)
}

// EnvGatewayURL and EnvGatewayToken are the preferred environment variables for
// gateway connectivity. They map directly to the indexer plan and to the
// Bearer-token model used by the gateway's other APIs.
const (
	EnvGatewayURL   = naming.EnvGatewayURLTarget
	EnvGatewayToken = naming.EnvGatewayTokenTarget
)

// DefaultsYAML is the optional top-level `defaults:` block (v0.3).
type DefaultsYAML struct {
	ProjectID   string `yaml:"project_id"`
	FlavorID    string `yaml:"flavor_id"`
	WorkspaceID string `yaml:"workspace_id"`
}

// RootYAML is one roots[] entry after YAML parse (path + optional scope).
type RootYAML struct {
	Path        string `yaml:"path"`
	ProjectID   string `yaml:"project_id"`
	FlavorID    string `yaml:"flavor_id"`
	WorkspaceID string `yaml:"workspace_id"`
}

// FlexibleRoots unmarshals roots as either a list of directory strings or a
// list of mappings with `path` and optional scope fields (v0.3).
type FlexibleRoots []RootYAML

// UnmarshalYAML implements yaml.Unmarshaler for backward-compatible roots.
func (fr *FlexibleRoots) UnmarshalYAML(n *yaml.Node) error {
	*fr = nil
	if n.Kind != yaml.SequenceNode {
		return fmt.Errorf("roots: must be a sequence")
	}
	for _, el := range n.Content {
		switch el.Kind {
		case yaml.ScalarNode:
			p := strings.TrimSpace(el.Value)
			if p != "" {
				*fr = append(*fr, RootYAML{Path: p})
			}
		case yaml.MappingNode:
			var row RootYAML
			if err := el.Decode(&row); err != nil {
				return fmt.Errorf("roots entry: %w", err)
			}
			row.Path = strings.TrimSpace(row.Path)
			if row.Path == "" {
				return fmt.Errorf("roots entry: missing path")
			}
			*fr = append(*fr, row)
		default:
			return fmt.Errorf("roots: unsupported YAML node kind %v", el.Kind)
		}
	}
	return nil
}

// OverrideYAML is one `overrides:` entry (v0.3).
type OverrideYAML struct {
	Glob        string `yaml:"glob"`
	ProjectID   string `yaml:"project_id"`
	FlavorID    string `yaml:"flavor_id"`
	WorkspaceID string `yaml:"workspace_id"`
}

// FileConfig is the on-disk YAML schema (v0.2 minimal + v0.3 scope fields).
type FileConfig struct {
	GatewayURL  string         `yaml:"gateway_url"`
	Defaults    *DefaultsYAML  `yaml:"defaults"`
	Roots       FlexibleRoots  `yaml:"roots"`
	Overrides   []OverrideYAML `yaml:"overrides"`
	IgnoreExtra []string       `yaml:"ignore_extra"`

	// RecoveryIncludeRootHealth, when non-nil after merge, controls whether the
	// recovery poll also requires GET /health to be non-degraded. When absent
	// from all merged files, Resolve defaults to true.
	RecoveryIncludeRootHealth *bool `yaml:"recovery_include_root_health"`

	// RetryShortCircuitOnEmbed, when true (default), skips remaining per-file
	// retries after a 502/503 embed-classified ingest failure and opens the
	// global ingest gate instead of burning retry_max_attempts.
	RetryShortCircuitOnEmbed *bool `yaml:"retry_short_circuit_on_embed"`

	RetryMaxAttempts     int     `yaml:"retry_max_attempts"`
	RetryBaseDelayMS     int     `yaml:"retry_base_delay_ms"`
	RetryMaxDelayMS      int     `yaml:"retry_max_delay_ms"`
	RecoveryPollMS       int     `yaml:"recovery_poll_interval_ms"`
	DebounceMS           int     `yaml:"debounce_ms"`
	Workers              int     `yaml:"workers"`
	QueueDepth           int     `yaml:"queue_depth"`
	MaxFileBytes         int64   `yaml:"max_file_bytes"`
	RequestTimeoutMS     int     `yaml:"request_timeout_ms"`
	FollowSymlinks       bool    `yaml:"follow_symlinks"` // v0.2 forces false at Resolve time.
	BinaryNullByteSample int     `yaml:"binary_null_byte_sample_bytes"`
	BinaryNullByteRatio  float64 `yaml:"binary_null_byte_ratio"`

	// v0.4: optional local overrides (see GET /v1/indexer/config for gateway defaults).
	SyncStatePath     string `yaml:"sync_state_path"`
	MaxWholeFileBytes int64  `yaml:"max_whole_file_bytes"` // 0 = use gateway max_whole_file_bytes only

	// VerboseJobLogs is deprecated; prefer job_skip_log (info | debug | off).
	// When job_skip_log is unset and VerboseJobLogs is non-nil after merge:
	// true → info, false → debug.
	VerboseJobLogs *bool `yaml:"verbose_job_logs"`

	// LogLevel is stderr minimum level for chimera-indexer (debug, info, warn, error).
	// Empty defaults to info.
	LogLevel string `yaml:"log_level"`

	// JobSkipLog selects per-file skip / upload verbosity: info (default),
	// debug (DEBUG indexer.skip.* only), or off (no skip/upload INFO lines).
	JobSkipLog string `yaml:"job_skip_log"`

	// JobIngestLog selects per-file ingest success verbosity: info (default) or debug.
	JobIngestLog string `yaml:"job_ingest_log"`

	// StorageStatsPollMS sets GET /v1/indexer/storage/stats polling. Zero = default (~2 min).
	// Negative = disable polling entirely.
	StorageStatsPollMS int `yaml:"storage_stats_poll_ms"`

	// QueueFanoutHWMPercent is p×100 for fair-share bulk fan-out (default 75 → p=0.75).
	// Valid range 1–100; zero/absent resolves to 75.
	QueueFanoutHWMPercent int `yaml:"queue_fanout_high_water_mark_percent"`

	// ScopeStatusPollMS emits periodic indexer.scope.status lines (per project+flavor scope).
	// Zero uses default (~45s); negative disables.
	ScopeStatusPollMS int `yaml:"scope_status_poll_ms"`

	// ScopeActiveFileLogMinIntervalMS rate-limits indexer.scope.active_file per scope (default 2000).
	ScopeActiveFileLogMinIntervalMS int `yaml:"scope_active_file_log_min_interval_ms"`

	// SkipSummaryMinIntervalMS controls batched indexer.job.skipped.summary lines (default 5000).
	// Zero uses default; negative disables summaries.
	SkipSummaryMinIntervalMS int `yaml:"skip_summary_min_interval_ms"`

	// ScopeStatusIngestMilestone emits edge scope.status every N successful ingests (default 25).
	// Zero uses default; negative disables ingest milestones.
	ScopeStatusIngestMilestone int `yaml:"scope_status_ingest_milestone"`

	// ScopeStatusEdgeMinIntervalMS rate-limits edge-triggered scope.status lines per scope (default 2000).
	ScopeStatusEdgeMinIntervalMS int `yaml:"scope_status_edge_min_interval_ms"`

	// QueueSnapshotIdleInfoIntervalMS controls how often unchanged idle indexer.queue.snapshot
	// lines repeat at INFO (default 300000 = 5 min). Negative disables idle INFO heartbeats.
	QueueSnapshotIdleInfoIntervalMS int `yaml:"queue_snapshot_idle_info_interval_ms"`

	// WorkspacesPollIntervalMS is how often supervised mode re-fetches GET /v1/indexer/workspaces.
	// Zero uses default (30s). Negative is treated as default.
	WorkspacesPollIntervalMS int `yaml:"workspaces_poll_interval_ms"`
}

// Resolved is the runtime indexer configuration after merging YAML, env vars,
// and CLI overrides. All durations are normalized.
type Resolved struct {
	GatewayURL  string
	Token       string
	Roots       []Root
	IgnoreExtra []string

	DefaultScope  ScopeFragment
	GlobOverrides []GlobOverride

	RetryMaxAttempts     int
	RetryBaseDelay       time.Duration
	RetryMaxDelay        time.Duration
	RecoveryPollInterval time.Duration
	Debounce             time.Duration
	Workers              int
	QueueDepth           int
	MaxFileBytes         int64
	RequestTimeout       time.Duration

	BinaryNullByteSample int
	BinaryNullByteRatio  float64

	SyncStatePath     string
	MaxWholeFileBytes int64

	// RecoveryIncludeRootHealth gates an extra GET /health probe while workers
	// are paused after exhausting ingest retries (defaults true when unset in YAML).
	RecoveryIncludeRootHealth bool

	// RetryShortCircuitOnEmbed skips remaining per-file retries after an
	// embed-classified 502/503 and opens the ingest gate (defaults true).
	RetryShortCircuitOnEmbed bool

	// LogLevel is the minimum slog level for indexer stderr output.
	LogLevel slog.Level

	// JobSkipLog controls per-file skip / pre-upload lines (see ParseJobSkipLog).
	JobSkipLog JobSkipLogMode

	// JobIngestLog controls per-file indexer.job.ingested lines (see ParseJobIngestLog).
	JobIngestLog JobIngestLogMode

	// StorageStatsPoll is how often to call GET /v1/indexer/storage/stats and
	// emit indexer.state / storage stats logs. Zero disables polling.
	StorageStatsPoll time.Duration

	// QueueFanoutHWMPercent is the percentage of queue capacity reserved across
	// all bulk scopes for fair-share fan-out (default 75).
	QueueFanoutHWMPercent int

	// ScopeStatusPoll cadence for indexer.scope.status. Zero only when YAML sets scope_status_poll_ms < 0.
	ScopeStatusPoll time.Duration

	// ScopeActiveFileLogMinInterval rate-limits indexer.scope.active_file lines per scope.
	ScopeActiveFileLogMinInterval time.Duration

	// SkipSummaryMinInterval is the minimum window between indexer.job.skipped.summary
	// emissions per scope. Zero disables summaries.
	SkipSummaryMinInterval time.Duration

	// ScopeStatusIngestMilestone triggers edge scope.status every N ingests (0 = default 25).
	ScopeStatusIngestMilestone int

	// ScopeStatusEdgeMinInterval rate-limits edge scope.status emissions per scope (0 = default 2s).
	ScopeStatusEdgeMinInterval time.Duration

	// QueueSnapshotIdleInfoInterval is how often unchanged idle queue snapshots may repeat at INFO.
	// Zero disables idle INFO heartbeats (DEBUG only while idle and unchanged).
	QueueSnapshotIdleInfoInterval time.Duration

	// SupervisedLayer is true when --config names an explicit YAML file (desktop supervised).
	// Effective watch roots come from GET /v1/indexer/workspaces; YAML roots and --root are ignored.
	SupervisedLayer bool

	// WorkspacesPollInterval is the supervised workspace list poll cadence.
	WorkspacesPollInterval time.Duration
}

// Root is a watched directory and its stable, slug-form identifier used in
// logs. Scope holds per-root v0.3 overrides only (defaults applied separately).
type Root struct {
	// ID is a slug derived from the root's basename; it never appears in
	// payloads sent to the gateway and exists only for local logging.
	ID string
	// AbsPath is the cleaned absolute path on this host. It must never be
	// transmitted to the gateway.
	AbsPath string
	// Scope is optional per-root project / flavor / workspace overrides.
	Scope ScopeFragment
}

// RootIDsCSV returns watch-root IDs comma-separated for structured logs / UI.
func RootIDsCSV(roots []Root) string {
	if len(roots) == 0 {
		return ""
	}
	var b strings.Builder
	for i, r := range roots {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strings.TrimSpace(r.ID))
	}
	return b.String()
}

// LoadFile reads a YAML config file. Returns a zero-value FileConfig if path
// is empty so callers can compose with environment-only setups.
func LoadFile(path string) (FileConfig, error) {
	var fc FileConfig
	if path == "" {
		return fc, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return fc, fmt.Errorf("read indexer config %q: %w", path, err)
	}
	if err := yaml.Unmarshal(b, &fc); err != nil {
		return fc, fmt.Errorf("parse indexer config %q: %w", path, err)
	}
	return fc, nil
}

// Overrides captures CLI-flag overrides applied last in the precedence chain.
type Overrides struct {
	GatewayURL string
	Roots      []string
	// ExplicitConfigPath is the --config path when the operator passes one (including
	// supervised desktop). When sync_state_path is unset in merged YAML, Resolve
	// defaults sync state to filepath.Join(filepath.Dir(absExplicit), IndexerSyncStateFileName).
	// The on-disk store is SQLite at sync-state.sqlite next to a .json configured path.
	ExplicitConfigPath string
	// AllowEmptyRoots, when true, skips the "at least one watch root" check so supervised
	// mode can stay alive and wait for the UI to append roots (desktop bootstrap).
	AllowEmptyRoots bool
}

// Resolve produces a Resolved config from merged YAML (see LoadLayeredConfig),
// environment lookup, and CLI overrides. Gateway URL: merged YAML <
// CHIMERA_GATEWAY_URL < --gateway-url. Roots:
// merged YAML < --root (when any --root is set). Token is from
// CHIMERA_GATEWAY_TOKEN only (no token-in-YAML).
// When sync_state_path is empty, default is indexer.sync-state.json next to
// Overrides.ExplicitConfigPath when set, else .locus/indexer.sync-state.json.
func Resolve(fc FileConfig, env func(string) string, ov Overrides) (Resolved, error) {
	if env == nil {
		env = os.Getenv
	}
	r := Resolved{
		GatewayURL:           strings.TrimSpace(fc.GatewayURL),
		IgnoreExtra:          append([]string(nil), fc.IgnoreExtra...),
		RetryMaxAttempts:     fc.RetryMaxAttempts,
		RetryBaseDelay:       msOr(fc.RetryBaseDelayMS, defaultRetryBaseDelay),
		RetryMaxDelay:        msOr(fc.RetryMaxDelayMS, defaultRetryMaxDelay),
		RecoveryPollInterval: msOr(fc.RecoveryPollMS, defaultRecoveryPollInterval),
		Debounce:             msOr(fc.DebounceMS, defaultDebounce),
		Workers:              fc.Workers,
		QueueDepth:           fc.QueueDepth,
		MaxFileBytes:         fc.MaxFileBytes,
		RequestTimeout:       msOr(fc.RequestTimeoutMS, defaultRequestTimeout),
		BinaryNullByteSample: fc.BinaryNullByteSample,
		BinaryNullByteRatio:  fc.BinaryNullByteRatio,
		SyncStatePath:        strings.TrimSpace(fc.SyncStatePath),
		MaxWholeFileBytes:    fc.MaxWholeFileBytes,
		LogLevel:             slog.LevelInfo,
		JobSkipLog:           JobSkipLogInfo,
		JobIngestLog:         JobIngestLogInfo,
	}
	if strings.TrimSpace(fc.LogLevel) != "" {
		r.LogLevel = ParseLogLevel(fc.LogLevel)
	}
	if js := strings.TrimSpace(fc.JobSkipLog); js != "" {
		m, err := ParseJobSkipLog(js)
		if err != nil {
			return Resolved{}, err
		}
		r.JobSkipLog = m
	} else if fc.VerboseJobLogs != nil {
		if *fc.VerboseJobLogs {
			r.JobSkipLog = JobSkipLogInfo
		} else {
			r.JobSkipLog = JobSkipLogDebug
		}
	}
	if ji := strings.TrimSpace(fc.JobIngestLog); ji != "" {
		m, err := ParseJobIngestLog(ji)
		if err != nil {
			return Resolved{}, err
		}
		r.JobIngestLog = m
	} else if r.JobSkipLog == JobSkipLogDebug {
		r.JobIngestLog = JobIngestLogDebug
	}
	switch {
	case fc.StorageStatsPollMS < 0:
		r.StorageStatsPoll = 0
	case fc.StorageStatsPollMS > 0:
		r.StorageStatsPoll = time.Duration(fc.StorageStatsPollMS) * time.Millisecond
	default:
		r.StorageStatsPoll = defaultStorageStatsPoll
	}
	switch {
	case fc.ScopeStatusPollMS < 0:
		r.ScopeStatusPoll = 0
	case fc.ScopeStatusPollMS > 0:
		r.ScopeStatusPoll = time.Duration(fc.ScopeStatusPollMS) * time.Millisecond
	default:
		r.ScopeStatusPoll = defaultScopeStatusPoll
	}
	switch {
	case fc.ScopeActiveFileLogMinIntervalMS < 0:
		r.ScopeActiveFileLogMinInterval = 0
	case fc.ScopeActiveFileLogMinIntervalMS > 0:
		r.ScopeActiveFileLogMinInterval = time.Duration(fc.ScopeActiveFileLogMinIntervalMS) * time.Millisecond
	default:
		r.ScopeActiveFileLogMinInterval = time.Duration(defaultScopeActiveFileMinMs) * time.Millisecond
	}
	switch {
	case fc.SkipSummaryMinIntervalMS < 0:
		r.SkipSummaryMinInterval = 0
	case fc.SkipSummaryMinIntervalMS > 0:
		r.SkipSummaryMinInterval = time.Duration(fc.SkipSummaryMinIntervalMS) * time.Millisecond
	default:
		r.SkipSummaryMinInterval = time.Duration(defaultSkipSummaryMinIntervalMs) * time.Millisecond
	}
	if fc.ScopeStatusIngestMilestone != 0 {
		r.ScopeStatusIngestMilestone = fc.ScopeStatusIngestMilestone
	}
	switch {
	case fc.ScopeStatusEdgeMinIntervalMS < 0:
		r.ScopeStatusEdgeMinInterval = 0
	case fc.ScopeStatusEdgeMinIntervalMS > 0:
		r.ScopeStatusEdgeMinInterval = time.Duration(fc.ScopeStatusEdgeMinIntervalMS) * time.Millisecond
	default:
		r.ScopeStatusEdgeMinInterval = time.Duration(defaultScopeStatusEdgeMinIntervalMs) * time.Millisecond
	}
	switch {
	case fc.QueueFanoutHWMPercent >= 1 && fc.QueueFanoutHWMPercent <= 100:
		r.QueueFanoutHWMPercent = fc.QueueFanoutHWMPercent
	default:
		r.QueueFanoutHWMPercent = defaultQueueFanoutHWMPercent
	}
	switch {
	case fc.QueueSnapshotIdleInfoIntervalMS < 0:
		r.QueueSnapshotIdleInfoInterval = 0
	case fc.QueueSnapshotIdleInfoIntervalMS > 0:
		r.QueueSnapshotIdleInfoInterval = time.Duration(fc.QueueSnapshotIdleInfoIntervalMS) * time.Millisecond
	default:
		r.QueueSnapshotIdleInfoInterval = time.Duration(defaultQueueSnapshotIdleInfoIntervalMs) * time.Millisecond
	}
	switch {
	case fc.WorkspacesPollIntervalMS > 0:
		r.WorkspacesPollInterval = time.Duration(fc.WorkspacesPollIntervalMS) * time.Millisecond
	default:
		r.WorkspacesPollInterval = defaultWorkspacesPollInterval
	}
	if fc.Defaults != nil {
		r.DefaultScope = ScopeFragment{
			ProjectID:   strings.TrimSpace(fc.Defaults.ProjectID),
			FlavorID:    strings.TrimSpace(fc.Defaults.FlavorID),
			WorkspaceID: strings.TrimSpace(fc.Defaults.WorkspaceID),
		}
	}
	for i, o := range fc.Overrides {
		pat := strings.TrimSpace(o.Glob)
		if pat == "" {
			return r, fmt.Errorf("overrides[%d]: glob is required", i)
		}
		if _, err := doublestar.Match(pat, "probe"); err != nil {
			return r, fmt.Errorf("overrides[%d]: invalid glob %q: %w", i, pat, err)
		}
		r.GlobOverrides = append(r.GlobOverrides, GlobOverride{
			Pattern: pat,
			Scope: ScopeFragment{
				ProjectID:   strings.TrimSpace(o.ProjectID),
				FlavorID:    strings.TrimSpace(o.FlavorID),
				WorkspaceID: strings.TrimSpace(o.WorkspaceID),
			},
		})
	}
	if r.RetryMaxAttempts <= 0 {
		r.RetryMaxAttempts = defaultRetryAttempts
	}
	if r.Workers <= 0 {
		r.Workers = defaultWorkers
	}
	if r.QueueDepth <= 0 {
		r.QueueDepth = defaultQueueDepth
	}
	if r.MaxFileBytes <= 0 {
		r.MaxFileBytes = defaultMaxFileBytes
	}
	if r.BinaryNullByteSample <= 0 {
		r.BinaryNullByteSample = 8000
	}
	if r.BinaryNullByteRatio <= 0 || r.BinaryNullByteRatio > 1 {
		r.BinaryNullByteRatio = 0.001 // any NUL byte in the sample marks binary
	}

	if v := strings.TrimSpace(env(EnvGatewayURL)); v != "" {
		r.GatewayURL = v
	}
	if ov.GatewayURL != "" {
		r.GatewayURL = ov.GatewayURL
	}
	r.Token = strings.TrimSpace(env(EnvGatewayToken))

	supervised := strings.TrimSpace(ov.ExplicitConfigPath) != ""
	r.SupervisedLayer = supervised

	var rootEntries []RootYAML
	if supervised {
		// Supervised --config uses gateway workspaces API only (no YAML roots, no --root).
		rootEntries = nil
	} else if ov.Roots != nil {
		for _, p := range ov.Roots {
			p = strings.TrimSpace(p)
			if p != "" {
				rootEntries = append(rootEntries, RootYAML{Path: p})
			}
		}
	} else {
		rootEntries = append(rootEntries, fc.Roots...)
	}

	for _, entry := range rootEntries {
		p := strings.TrimSpace(entry.Path)
		if p == "" {
			continue
		}
		abs, err := filepath.Abs(p)
		if err != nil {
			return r, fmt.Errorf("resolve root %q: %w", p, err)
		}
		abs = filepath.Clean(abs)
		st, err := os.Stat(abs)
		if err != nil {
			return r, fmt.Errorf("stat root %q: %w", abs, err)
		}
		if !st.IsDir() {
			return r, fmt.Errorf("root %q is not a directory", abs)
		}
		scope := ScopeFragment{
			ProjectID:   strings.TrimSpace(entry.ProjectID),
			FlavorID:    strings.TrimSpace(entry.FlavorID),
			WorkspaceID: strings.TrimSpace(entry.WorkspaceID),
		}
		r.Roots = append(r.Roots, Root{ID: rootSlug(abs), AbsPath: abs, Scope: scope})
	}

	if r.GatewayURL == "" {
		return r, errors.New("gateway URL is required (config gateway_url, --gateway-url, or " + EnvGatewayURL + ")")
	}
	if r.Token == "" {
		return r, errors.New("gateway bearer token is required (set " + EnvGatewayToken + ")")
	}
	if len(r.Roots) == 0 && !ov.AllowEmptyRoots {
		return r, errors.New("at least one watch root is required (config roots or --root)")
	}
	if r.SyncStatePath == "" {
		if p := strings.TrimSpace(ov.ExplicitConfigPath); p != "" {
			abs, err := filepath.Abs(p)
			if err != nil {
				r.SyncStatePath = filepath.Join(indexerHiddenStateDir, IndexerSyncStateFileName)
			} else {
				r.SyncStatePath = filepath.Join(filepath.Dir(abs), IndexerSyncStateFileName)
			}
		} else {
			r.SyncStatePath = filepath.Join(indexerHiddenStateDir, IndexerSyncStateFileName)
		}
	}
	if fc.RecoveryIncludeRootHealth != nil {
		r.RecoveryIncludeRootHealth = *fc.RecoveryIncludeRootHealth
	} else {
		r.RecoveryIncludeRootHealth = true
	}
	if fc.RetryShortCircuitOnEmbed != nil {
		r.RetryShortCircuitOnEmbed = *fc.RetryShortCircuitOnEmbed
	} else {
		r.RetryShortCircuitOnEmbed = true
	}
	return r, nil
}

// LayeredConfigPaths returns config file paths to merge in order: global user
// file (~/.locus/indexer.config.yaml), workspace-local (.locus/ under
// cwd), then explicitPath last when non-empty. Duplicate paths are de-duped
// with the last position winning (so --config overrides an earlier copy).
func LayeredConfigPaths(cwd, explicitPath string) []string {
	add := func(list []string, p string) []string {
		p = filepath.Clean(p)
		if p == "." || p == "" {
			return list
		}
		out := list[:0]
		for _, x := range list {
			if x != p {
				out = append(out, x)
			}
		}
		return append(out, p)
	}
	var paths []string
	if home, err := os.UserHomeDir(); err == nil {
		paths = add(paths, HiddenIndexerConfigPath(home))
	}
	if cwd != "" {
		paths = add(paths, HiddenIndexerConfigPath(cwd))
	}
	if strings.TrimSpace(explicitPath) != "" {
		paths = add(paths, explicitPath)
	}
	return paths
}

// LoadLayeredConfig merges all existing YAML files from LayeredConfigPaths.
// If explicitPath is set, that file must exist (even if empty YAML). Missing
// global or local files are skipped silently.
func LoadLayeredConfig(cwd, explicitPath string) (FileConfig, error) {
	paths := LayeredConfigPaths(cwd, explicitPath)
	explicitClean := filepath.Clean(strings.TrimSpace(explicitPath))
	var acc FileConfig
	for _, p := range paths {
		p = filepath.Clean(p)
		if _, err := os.Stat(p); err != nil {
			if explicitClean != "" && p == explicitClean {
				if errors.Is(err, os.ErrNotExist) {
					return FileConfig{}, fmt.Errorf("indexer config not found: %s", p)
				}
				return FileConfig{}, fmt.Errorf("indexer config %q: %w", p, err)
			}
			continue
		}
		fc, err := LoadFile(p)
		if err != nil {
			return FileConfig{}, err
		}
		acc = MergeFileConfig(acc, fc)
	}
	return acc, nil
}

// MergeFileConfig merges overlay onto base (later wins for overlapping keys).
// effectiveQueueFanoutHWMPercent returns a valid 1–100 fan-out high-water mark.
func effectiveQueueFanoutHWMPercent(pct int) int {
	if pct >= 1 && pct <= 100 {
		return pct
	}
	return defaultQueueFanoutHWMPercent
}

func MergeFileConfig(base, overlay FileConfig) FileConfig {
	out := base
	if strings.TrimSpace(overlay.GatewayURL) != "" {
		out.GatewayURL = overlay.GatewayURL
	}
	if overlay.Defaults != nil {
		if out.Defaults == nil {
			cp := *overlay.Defaults
			out.Defaults = &cp
		} else {
			if strings.TrimSpace(overlay.Defaults.ProjectID) != "" {
				out.Defaults.ProjectID = overlay.Defaults.ProjectID
			}
			if strings.TrimSpace(overlay.Defaults.FlavorID) != "" {
				out.Defaults.FlavorID = overlay.Defaults.FlavorID
			}
			if strings.TrimSpace(overlay.Defaults.WorkspaceID) != "" {
				out.Defaults.WorkspaceID = overlay.Defaults.WorkspaceID
			}
		}
	}
	if len(overlay.Roots) > 0 {
		out.Roots = append(FlexibleRoots(nil), overlay.Roots...)
	}
	if overlay.Overrides != nil {
		out.Overrides = append([]OverrideYAML(nil), overlay.Overrides...)
	}
	if overlay.IgnoreExtra != nil {
		out.IgnoreExtra = append([]string(nil), overlay.IgnoreExtra...)
	}
	if overlay.RecoveryIncludeRootHealth != nil {
		v := *overlay.RecoveryIncludeRootHealth
		out.RecoveryIncludeRootHealth = &v
	}
	if overlay.RetryShortCircuitOnEmbed != nil {
		v := *overlay.RetryShortCircuitOnEmbed
		out.RetryShortCircuitOnEmbed = &v
	}
	if overlay.RetryMaxAttempts != 0 {
		out.RetryMaxAttempts = overlay.RetryMaxAttempts
	}
	if overlay.RetryBaseDelayMS != 0 {
		out.RetryBaseDelayMS = overlay.RetryBaseDelayMS
	}
	if overlay.RetryMaxDelayMS != 0 {
		out.RetryMaxDelayMS = overlay.RetryMaxDelayMS
	}
	if overlay.RecoveryPollMS != 0 {
		out.RecoveryPollMS = overlay.RecoveryPollMS
	}
	if overlay.DebounceMS != 0 {
		out.DebounceMS = overlay.DebounceMS
	}
	if overlay.Workers != 0 {
		out.Workers = overlay.Workers
	}
	if overlay.QueueDepth != 0 {
		out.QueueDepth = overlay.QueueDepth
	}
	if overlay.MaxFileBytes != 0 {
		out.MaxFileBytes = overlay.MaxFileBytes
	}
	if overlay.RequestTimeoutMS != 0 {
		out.RequestTimeoutMS = overlay.RequestTimeoutMS
	}
	if overlay.BinaryNullByteSample != 0 {
		out.BinaryNullByteSample = overlay.BinaryNullByteSample
	}
	if overlay.BinaryNullByteRatio != 0 {
		out.BinaryNullByteRatio = overlay.BinaryNullByteRatio
	}
	if strings.TrimSpace(overlay.SyncStatePath) != "" {
		out.SyncStatePath = overlay.SyncStatePath
	}
	if overlay.MaxWholeFileBytes != 0 {
		out.MaxWholeFileBytes = overlay.MaxWholeFileBytes
	}
	if overlay.VerboseJobLogs != nil {
		v := *overlay.VerboseJobLogs
		out.VerboseJobLogs = &v
	}
	if strings.TrimSpace(overlay.LogLevel) != "" {
		out.LogLevel = overlay.LogLevel
	}
	if strings.TrimSpace(overlay.JobSkipLog) != "" {
		out.JobSkipLog = overlay.JobSkipLog
	}
	if strings.TrimSpace(overlay.JobIngestLog) != "" {
		out.JobIngestLog = overlay.JobIngestLog
	}
	if overlay.QueueFanoutHWMPercent >= 1 && overlay.QueueFanoutHWMPercent <= 100 {
		out.QueueFanoutHWMPercent = overlay.QueueFanoutHWMPercent
	}
	if overlay.ScopeStatusPollMS != 0 {
		out.ScopeStatusPollMS = overlay.ScopeStatusPollMS
	}
	if overlay.ScopeActiveFileLogMinIntervalMS != 0 {
		out.ScopeActiveFileLogMinIntervalMS = overlay.ScopeActiveFileLogMinIntervalMS
	}
	if overlay.SkipSummaryMinIntervalMS != 0 {
		out.SkipSummaryMinIntervalMS = overlay.SkipSummaryMinIntervalMS
	}
	if overlay.ScopeStatusIngestMilestone != 0 {
		out.ScopeStatusIngestMilestone = overlay.ScopeStatusIngestMilestone
	}
	if overlay.ScopeStatusEdgeMinIntervalMS != 0 {
		out.ScopeStatusEdgeMinIntervalMS = overlay.ScopeStatusEdgeMinIntervalMS
	}
	return out
}

func msOr(ms int, def time.Duration) time.Duration {
	if ms <= 0 {
		return def
	}
	return time.Duration(ms) * time.Millisecond
}

// rootSlug returns a short, filesystem-safe identifier derived from the
// basename of an absolute root path. It is intended for human logs only and
// must never be sent to the gateway.
func rootSlug(abs string) string {
	base := filepath.Base(abs)
	base = strings.ToLower(base)
	var b strings.Builder
	for _, r := range base {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_' || r == '.':
			b.WriteRune('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		out = "root"
	}
	return out
}
