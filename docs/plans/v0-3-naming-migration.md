# Plan: v0.3 product naming migration

| Field | Value |
|-------|-------|
| **Doc kind** | `refactor-plan` |
| **Owners / areas** | Gateway, indexer, desktop, packaging/release, docs, CI |
| **Status** | `shipped` |
| **Targets** | gateway/indexer v0.3 naming train |
| **Last updated** | See git history |
| **Supersedes / superseded by** | Supersedes `naming-migration.md`, `v0.3_naming_migration_1c31a8cf.plan.md`, `v0-3-naming-phase1-inventory.md`, `v0-3-naming-hard-cut-execution.md` |

## At a glance

This plan moves operator-facing and technical naming from legacy Chimera-era surfaces to the layered **Porcelain** (suite) / **Chimera** (backend) / **Locus** (clients) model, using a **hard-cut** policy: no dual-read env vars, headers, config keys, or path aliases in this train. Work starts with discovery and a rename matrix, then executes through constants, operational surfaces, code/tests, docs, topology, and final validation.

Phase 9 closeout unblocks [`vectorstore-broker-wrapper-hard-cut.md`](vectorstore-broker-wrapper-hard-cut.md) Phases 2–7 (prerequisite row marked `done` there).

| Phase | Outcome | Status |
|-------|---------|--------|
| [Phase 1 — Discovery and symbol inventory](#phase-1--discovery-and-symbol-inventory) | Source-backed map of naming/path/package surfaces and hard-cut decision matrix | `done` |
| [Phase 2 — Constants and docs or comment rename](#phase-2--constants-and-docs-or-comment-rename) | Centralized naming constants and consistent terminology in code comments and help text | `done` |
| [Phase 3 — Migration and operational surfaces](#phase-3--migration-and-operational-surfaces) | Config, data, rules, CI, assets, and packaging aligned to canonical contracts | `done` |
| [Phase 4 — Go source and tests](#phase-4--go-source-and-tests) | Runtime code and tests updated end-to-end to new naming | `done` |
| [Phase 5 — Markdown documentation rename pass](#phase-5--markdown-documentation-rename-pass) | Coordinated markdown rename aligned to implemented naming | `done` |
| [Phase 6 — Documentation consolidation](#phase-6--documentation-consolidation) | Operator docs normalized; historical context labeled explicitly | `done` |
| [Phase 7 — Legacy alias policy lock](#phase-7--legacy-alias-policy-lock) | Hard cut: no legacy alias support in active surfaces | `done` |
| [Phase 8 — Topology and entrypoint restructure](#phase-8--topology-and-entrypoint-restructure) | Entrypoints, make namespace, and layout convergence toward target tree | `done` |
| [Phase 9 — Validation and closeout](#phase-9--validation-and-closeout) | Build, test, package, and audit proof; migration train closure | `done` |

---

## Background

The `Product naming` and `Credential file naming` themes in [version-v0.3.md](../version-v0.3.md) require a repo-wide naming transition while preserving operator clarity. Make targets and runtime contracts on this branch are largely migrated; remaining work is validation, closeout, and dependent-plan gates.

**Operator decisions (fixed for this train)**

1. Remove all legacy `chimera-*` compatibility targets and aliases.
2. Retire `tokens` naming; use only `api-keys` file and `paths.api_keys` config key.
3. Remove legacy naming from active comments, examples, and tests.
4. Adopt the new make namespace directly (no temporary alias period).
5. Ship `chimera-supervisor` as a first-class binary separate from `locus-desktop`.
6. Limit broad doc rewrite to operator-facing surfaces; planning archives may retain historical examples.
7. Hard cut on env/header/config/runtime-dir contracts (no dual read).

**Related docs:** [version-v0.3.md](../version-v0.3.md), [configuration.md](../configuration.md), [packaging.md](../packaging.md), [supervisor.md](../supervisor.md), [indexer.md](../indexer.md), [installation.md](../installation.md), [README.md](../../README.md).

---

## Phase 1 — Discovery and symbol inventory

**Goal.** Produce a high-signal, source-backed inventory of naming, binaries, config and data paths, and package manifests, then lock a hard-cut migration matrix before broad edits.

**Deliverables**

- Symbol taxonomy across first-party paths (`cmd/`, `internal/`, `docs/`, `scripts/`, `config/`, `.github/`, release manifests).
- Binary and artifact map from [Makefile](../../Makefile), [scripts/chimera-names.sh](../../scripts/chimera-names.sh), [scripts/desktop-build.sh](../../scripts/desktop-build.sh), [scripts/release-package.sh](../../scripts/release-package.sh), [.goreleaser.yaml](../../.goreleaser.yaml).
- Runtime path map from [internal/config/config.go](../../internal/config/config.go), [internal/indexer/config.go](../../internal/indexer/config.go), example configs, [scripts/clean-data.sh](../../scripts/clean-data.sh), [.gitignore](../../.gitignore).
- Packaging validation evidence (release snapshot, archive contents, personal bundle layout).
- Rename decision matrix keyed by symbol class.

**Discovery summary**

| Class | Families found | Primary locations |
|-------|----------------|-------------------|
| Product naming | Chimera, Porcelain, Locus | README, version doc, `cmd/`, embed UI |
| Env vars | `CHIMERA_*` | `internal/config`, `internal/indexer`, `scripts/chimera-names.sh` |
| HTTP headers | `X-Chimera-*` | `internal/server`, `internal/indexer/client.go` |
| Binaries | `chimera-gateway`, `chimera-supervisor`, `chimera-indexer`, `locus-desktop` | Makefile, GoReleaser, release scripts |
| Indexer hidden state | `.locus/*` (was `.chimera/*`) | `internal/indexer/config.go`, indexer example config |

| Surface | Canonical behavior | Source of truth |
|---------|---------------------|-----------------|
| Gateway build | `make chimera-gateway-build` → `chimera-gateway` | Makefile |
| Supervisor build | `make chimera-supervisor-build` → `chimera-supervisor` | Makefile, `cmd/chimera-supervisor` |
| Indexer build | `make chimera-indexer-build` → `chimera-indexer` from `chimera/chimera-indexer` | Makefile |
| Desktop | `locus-desktop` from `cmd/chimera` | `scripts/desktop-build.sh` |
| Release archives | GoReleaser `project_name: chimera`, binary `chimera-gateway` | `.goreleaser.yaml` |
| Personal bundle | `dist/personal/chimera-bundle_<os>_<arch>/` | `scripts/release-package.sh` |

**Hard-cut decision matrix**

| Symbol class | Retired | Canonical |
|--------------|---------|-----------|
| Gateway binary or targets | `chimera*` (legacy umbrella) | `chimera-gateway*` |
| Supervisor | serve-only embedding | `chimera-supervisor*` (dedicated binary) |
| Indexer | `chimera-index`, `cmd/chimera-index` | `chimera-indexer`, `chimera/chimera-indexer` |
| Desktop | `chimera-desktop*` | `locus-desktop*` |
| Env vars | legacy `CHIMERA_*` family (removed) | `CHIMERA_*` |
| HTTP headers | legacy `X-Chimera-*` family (removed) | `X-Chimera-*` |
| Credentials file | `tokens.yaml`, `tokens.example.yaml` | `api-keys.yaml`, `api-keys.example.yaml` |
| Gateway config key | `paths.tokens` | `paths.api_keys` |
| Credential schema | row field `token` | `api_keys` / row field `secret` |
| Indexer hidden dir | `.chimera/` | `.locus/` |

**Target naming taxonomy (directional)**

| Target name | Responsibility |
|-------------|----------------|
| `chimera-supervisor` | Orchestrates managed services |
| `chimera-gateway` | API, routing, auth |
| `chimera-indexer` | Ingestion, embeddings |
| `chimera-bifrost` | BiFrost bridge |
| `chimera-vectorstore` | Vector store adapter |
| `locus-desktop` | Native shell; uses supervisor externally |

**Acceptance**

- Migration matrix exists with concrete symbol classes, owners, and target names.
- Binary, archive, and runtime path behavior documented with source references.
- Bounded open decisions only (no hidden assumptions).

**Status:** `done`

---

## Phase 2 — Constants and docs or comment rename

**Goal.** Centralize product naming constants and apply terminology updates to non-Markdown docs, comments, and help text in controlled batches.

**Deliverables**

- Shared constants in scripts, make, release, and Go ([internal/naming/contracts.go](../../internal/naming/contracts.go)).
- Minimal duplication of binary and display naming.
- Comment and help-text renames in `cmd/` entrypoints without behavior changes.
- Scripted bulk transforms where safe; manual review for semantic symbols.

**Acceptance**

- Naming constants discoverable from a small set of source locations.
- Top-level help and comments no longer present legacy product naming as primary identity.

**Status:** `done`

---

## Phase 3 — Migration and operational surfaces

**Goal.** Apply hard-cut naming to config, data, rules, CI, assets, and packaging; align runtime contracts with operator docs.

**Deliverables**

- Config keys, example files, and generated examples (`api-keys`, `paths.api_keys`, `.locus`).
- Data and runtime directory names in cleanup scripts and `.gitignore`.
- Cursor rules, GitHub workflows, and release metadata on canonical names.
- Packaging and GoReleaser outputs including `api-keys.example.yaml` only (no `tokens.example.yaml`).
- Operator migration notes for every breaking path or file name (see [Operator migration reference](#operator-migration-reference)).

**Canonical contracts (active surfaces)**

| Contract | Canonical value |
|----------|-----------------|
| Gateway config env | `CHIMERA_GATEWAY_CONFIG` (default `./config/gateway.yaml`) |
| Gateway URL or token env | `CHIMERA_GATEWAY_URL`, `CHIMERA_GATEWAY_TOKEN` |
| Upstream API key env | `CHIMERA_BROKER_API_KEY` |
| Credentials path in gateway.yaml | `paths.api_keys: "./api-keys.yaml"` |
| Indexer config or sync state | `~/.locus/indexer.config.yaml`, `.locus/indexer.sync-state.json` |
| Personal bundle binaries | `chimera-supervisor[.exe]`, `locus-desktop[.exe]` |
| Archive or binary prefix | `chimera` (GoReleaser project); gateway binary `chimera-gateway` |

**Acceptance**

- Config, data, runtime directories, and package artifacts match scripts and operator docs.
- CI and release metadata emit canonical naming on in-scope surfaces.
- Breaking changes documented for operators.

**Status:** `done`

---

## Phase 4 — Go source and tests

**Goal.** Complete runtime code and test-suite updates so behavior, APIs, and fixtures use coherent naming.

**Deliverables**

- HTTP header constants and request handling on `X-Chimera-*`.
- Environment variable lookups and startup config resolution on `CHIMERA_*`.
- Startup logs and UI or API-facing strings on Chimera branding.
- Tests, fixtures, and snapshots updated; targeted test groups for config, server, indexer, supervisor, packaging checks.
- Provider key prefix, collection prefix, and session cookie naming aligned to `chimera-*` contracts where applicable.

**Acceptance**

- Go build and test pass for affected packages.
- No stale legacy naming on high-traffic runtime or test surfaces from Phase 1 matrix.
- Matrix checked off: implemented vs intentionally deferred.

**Status:** `done`

---

## Phase 5 — Markdown documentation rename pass

**Goal.** Complete a coordinated markdown rename pass after code, config, and packaging stabilize.

**Deliverables**

- Update naming across `README.md`, `docs/`, and plan docs to implemented contracts.
- Refresh command examples, binary names, paths, env vars, and headers.
- Legacy names only where explicitly marked historical (not primary identity).

**Acceptance**

- Operator-facing docs no longer present legacy names as primary.
- Examples match current build and package outputs.

**Status:** `done`

---

## Phase 6 — Documentation consolidation

**Goal.** Normalize documentation after the rename pass; separate historical context from current behavior.

**Deliverables**

- Resolve stale present-tense statements in planning docs.
- Label historical snapshots explicitly in this plan where retained for traceability.
- Align docs to the same canonical build, run, and package contracts.

**Acceptance**

- No contradictory docs for current runtime or packaging behavior.
- Historical notes are explicit and not mistaken for requirements.

**Status:** `done`

---

## Phase 7 — Legacy alias policy lock

**Goal.** Lock compatibility policy and enforce it through code, tests, and docs.

**Policy.** Hard cut: **no legacy alias support** on active surfaces (retired `CHIMERA_*` / `X-Chimera-*` families, `paths.tokens`, `tokens.yaml`, `.chimera/*`, legacy make targets, legacy binary lookup fallbacks).

**Deliverables**

- Remove dual-read and fallback logic for retired names.
- Operator docs state current contracts only; migration history preserved where useful.
- Targeted tests assert canonical behavior without alias paths.

**Acceptance**

- Compatibility matrix is explicit and internally consistent.
- Code, tests, and operator docs match hard-cut policy.

**Status:** `done`

---

## Phase 8 — Topology and entrypoint restructure

**Goal.** Converge entrypoints, make namespace, and repository layout toward the directional target tree without a single risky all-at-once move.

**Directional target layout**

```text
porcelain/
|-- chimera/
|   |-- chimera-supervisor/
|   |-- chimera-gateway/
|   |-- chimera-indexer/
|   |-- chimera-bifrost/
|   |-- chimera-vectorstore/
|   `-- internal/
|-- locus/
|   `-- locus-desktop/
`-- porcelain/
    |-- porcelain-config/
    |-- porcelain-meta/
    `-- porcelain-docs/
```

**Incremental slices (completed)**

| Slice | Scope | Outcome |
|-------|-------|---------|
| S1 | Ownership map | Documented boundaries for `cmd/`, `internal/`, scripts, config, docs |
| S2 | Internal package prep | Stabilized APIs behind existing imports |
| S3 | Path indirection | Makefile and scripts use canonical variables only |
| S4 | Docs or config abstraction | Operator docs reference contracts, not final tree paths |
| S5-1 | Indexer entrypoint move | `chimera/chimera-indexer/main.go`; removed `cmd/chimera-index` |
| S5-2 | Indexer binary name | `chimera-indexer[.exe]`; supervisor resolves canonical name only |
| S5-3 | Active-surface consistency | Docs, env.example, config examples on `chimera-indexer` |
| S5-4 | Legacy audit | Active surfaces clean; historical refs only in plan archives |
| S5-5 | Scaffold bootstrap | `porcelain/chimera/*`, `porcelain/locus/*`, `porcelain/porcelain/*` directories created |

**Entrypoints (current)**

| Binary | Package path |
|--------|----------------|
| `chimera-gateway` | `chimera/chimera-gateway` |
| `chimera-supervisor` | `chimera/chimera-supervisor` |
| `chimera-indexer` | `chimera/chimera-indexer` |
| `locus-desktop` | `locus/locus-desktop` |
| `tokencount` | `chimera/cmd/tokencount` |

**Make namespace (canonical targets only)**

- `chimera-gateway-build`, `chimera-gateway-run`, `chimera-gateway-test`
- `chimera-supervisor-build`, `chimera-supervisor-run`, `chimera-supervisor-test`
- `chimera-indexer-build`, `chimera-indexer-run`, `chimera-indexer-install`, `chimera-indexer-test`
- `locus-desktop-install`, `locus-desktop-build`, `locus-desktop-run`
- Cross-cutting: `chimera-build-all`, `chimera-run-all`, `chimera-stop-all`
- Supervisor state: `run/chimera-supervisor.pid`, `logs/chimera-supervisor.log`

**Parallel agent ownership map**

| Area | Primary paths | Boundary |
|------|---------------|----------|
| Entrypoints or runtime | `cmd/`, `internal/server/`, `internal/supervisor/` | Binaries, orchestration, help text |
| Make, scripts, release | `Makefile`, `scripts/`, `.goreleaser.yaml`, `.github/workflows/` | Targets, artifact names |
| Config examples | `config/*.example.yaml`, `env.example` | Canonical examples only |
| Tests | `internal/**/*_test.go`, `cmd/**/*_test.go` | Rename symbols; preserve behavior |
| Operator docs | `README.md`, selected `docs/*.md` | Current contract language |

**Acceptance**

- Build, test, and package workflows pass with restructured paths.
- No unresolved command-path drift in scripts or CI on touched surfaces.
- Naming hard-cut contracts consistent with directory-boundary work.

**Status:** `done`

---

## Phase 9 — Validation and closeout

**Goal.** Prove end-to-end consistency across build, runtime, tests, packaging, and operator docs; close the migration train and unblock dependent plans.

**Deliverables**

- Record validation for changed surfaces:
  - `make chimera-gateway-build`, `make chimera-supervisor-build`, `make chimera-indexer-build`, `make chimera-build` (desktop)
  - `go test` on supervisor, indexer, config, server, and related packages
  - `make release-snapshot` or package flow; verify archive contains `chimera-gateway`, `api-keys.example.yaml`, canonical configs
- Final naming audit on active source, docs, tests, and scripts (legacy `chimera` umbrella targets, retired `CHIMERA_*` / `X-Chimera-*`, `.chimera`, `paths.tokens`, `tokens.yaml`, `cmd/chimera-index`).
- Mark [`vectorstore-broker-wrapper-hard-cut.md`](vectorstore-broker-wrapper-hard-cut.md) Phases 2+ unblocked in closeout notes.
- Update this plan status table and remove superseded plan files from the tree.

**Acceptance**

- Build, test, and package paths pass with canonical names only.
- Final audit finds no unintended legacy naming on active surfaces.
- Wrapper-plan execution gate explicitly cleared.
- Plan ready for closure (`Status: shipped`).

**Status:** `done`

---

## Operator migration reference

Breaking changes for operators upgrading to v0.3 naming (hard cut only).

### Config and credential files

- `config/tokens.yaml` → `config/api-keys.yaml`
- `config/tokens.example.yaml` → `config/api-keys.example.yaml`
- In `gateway.yaml`: `paths.api_keys: "./api-keys.yaml"`
- Schema: top-level `api_keys`; per-row field `secret`

### Environment variables

Use the `CHIMERA_*` family only:

- `CHIMERA_GATEWAY_CONFIG`
- `CHIMERA_GATEWAY_URL`
- `CHIMERA_GATEWAY_TOKEN`
- `CHIMERA_BROKER_API_KEY`

### Hidden state directories

Indexer state under `.locus/`:

- `.locus/indexer.config.yaml`
- `.locus/indexer.sync-state.json`

(Migrate from `.chimera/` equivalents; no dual-read.)

### Release and package artifacts

- GoReleaser project or archive prefix: `chimera`
- `make package` → `dist/personal/chimera-bundle_<os>_<arch>/`
- Bundle includes `chimera-supervisor[.exe]` and `locus-desktop[.exe]`
- Package config includes `api-keys.example.yaml`

### CI and operational surfaces

- Build or release workflows use `chimera` archive or binary names and `locus-desktop` for desktop-tagged builds.

### Optional clean reset

```bash
make clean-data CONFIRM=1
```

Clears `data/bifrost`, `data/qdrant`, and `data/gateway` for a clean local stack.

### Final cutover status

- v0.3 uses hard-cut naming only.
- Retired and unsupported: pre-v0.3 `CHIMERA_*` and `X-Chimera-*` env/header families, `paths.tokens`, `tokens.yaml`, `.chimera/*`, legacy `chimera-*` make targets, legacy binary lookup fallbacks.

---

## Open questions

1. Which proposed suite-level Make targets (`porcelain-bootstrap`, `porcelain-build-all`, etc.) ship in a follow-on train vs remain directional only.
2. Whether Go module path rename and full physical split to the target topology are deferred (recommended: defer and document in a follow-on plan).

---

## References

- Runtime and config: `chimera/chimera-gateway/`, `chimera/chimera-supervisor/`, `chimera/chimera-indexer/`, `locus/locus-desktop/`, `chimera/internal/config/`, `chimera/chimera-indexer/indexer/`, `chimera/chimera-gateway/internal/server/`
- Build, scripts, release: `Makefile`, `scripts/chimera-names.sh`, `scripts/print-make-help.sh`, `scripts/release-package.sh`, `scripts/release-snapshot.sh`, `.goreleaser.yaml`, `.github/workflows/go.yml`
- Operator docs: `README.md`, `docs/configuration.md`, `docs/supervisor.md`, `docs/indexer.md`, `docs/installation.md`, `docs/packaging.md`, `docs/version-v0.3.md`
- Related plans: [`vectorstore-broker-wrapper-hard-cut.md`](vectorstore-broker-wrapper-hard-cut.md), [`locus-desktop-supervisor-contract.md`](locus-desktop-supervisor-contract.md), [`env-precedence-contract.md`](env-precedence-contract.md)
