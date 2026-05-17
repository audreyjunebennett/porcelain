# Plan: v0.3 naming phase 1 inventory

| Field | Value |
|-------|-------|
| **Doc kind** | `research/exploration` |
| **Owners / areas** | Gateway, indexer, packaging/release, docs, scripts |
| **Status** | `active` |
| **Targets** | v0.3 product naming migration |
| **Last updated** | See git history |
| **Supersedes / superseded by** | None |

## At a glance

This inventory is the Phase 1 discovery artifact for the v0.3 naming migration. It maps where naming and path contracts actually live, records packaging/runtime validation evidence, and defines the rename decision matrix that Phase 2-4 should execute.

| Phase | Outcome | Status |
|-------|---------|--------|
| [Phase 1 - Discovery and symbol inventory](#phase-1---discovery-and-symbol-inventory) | Symbol classes, binaries, paths, packaging, and decisions are documented with source references | `done` |
| [Phase 2 - Constants and source naming contracts](#phase-2---constants-and-source-naming-contracts) | Centralize naming constants and define source-level rename contracts | `done` |
| [Phase 3 - Migration and operational surfaces](#phase-3---migration-and-operational-surfaces) | Config/data/rules/CI/assets migration and packaging updates | `done` |
| [Phase 4 - Go source and tests](#phase-4---go-source-and-tests) | Runtime source + tests updated to final naming contracts | `done` |
| [Phase 5 - Markdown documentation rename pass](#phase-5---markdown-documentation-rename-pass) | Coordinated markdown rename pass aligned to implemented naming | `done` |
| [Phase 6 - Documentation consolidation](#phase-6---documentation-consolidation) | Normalize docs to deployed contracts and preserve historical notes explicitly | `done` |
| [Phase 7 - Legacy alias policy lock](#phase-7---legacy-alias-policy-lock) | Legacy compatibility policy resolved as hard cut: no legacy aliases supported | `done` |
| [Phase 8 - Topology and entrypoint restructure](#phase-8---topology-and-entrypoint-restructure) | Deferred directory/entrypoint move after naming stability | `done` |
| [Phase 9 - Final cutover and removal pass](#phase-9---final-cutover-and-removal-pass) | Remove retired aliases and finish target-state naming | `done` |

---

## Background

This document implements the discovery scope requested for [docs/version-v0.3.md](../version-v0.3.md) themes `Product naming` and `Credential file naming`: broad symbol examination (representative, not exhaustive), package/runtime path mapping, and a hard-cut migration strategy.

**Related docs:** [../version-v0.3.md](../version-v0.3.md), [../configuration.md](../configuration.md), [../packaging.md](../packaging.md), [../../README.md](../../README.md).

---

## Phase 1 - Discovery and symbol inventory

**Goal.** Establish a concrete, source-backed inventory of naming, path, and artifact contracts before rename execution.

**Deliverables**

- Symbol taxonomy for product terms, env vars, headers, binaries, and path conventions.
- Binary and artifact build/output map from make/scripts/release config.
- Runtime config/data/log/run path map from gateway/indexer/config files.
- Packaging validation evidence (what passes, what fails, and why).
- Rename decision matrix (hard-cut policy) for Phase 2-4 implementation.

### 1) Symbol taxonomy (first-party paths only)

| Class | Current families found | Primary locations |
|------|-------------------------|-------------------|
| Product naming | `Chimera`, `Porcelain`, `Locus` | [../../README.md](../../README.md), [../version-v0.3.md](../version-v0.3.md), [../../cmd/chimera/main.go](../../cmd/chimera/main.go), [../../internal/server/embedui/](../../internal/server/embedui/) |
| Env vars | `CHIMERA_*` (gateway/indexer/upstream/session), partial `CHIMERA_*` in scripts | [../../internal/config/config.go](../../internal/config/config.go), [../../internal/indexer/config.go](../../internal/indexer/config.go), [../../scripts/chimera-names.sh](../../scripts/chimera-names.sh) |
| HTTP headers | `X-Chimera-*` | [../../internal/server/ingest.go](../../internal/server/ingest.go), [../../internal/server/server.go](../../internal/server/server.go), [../../internal/indexer/client.go](../../internal/indexer/client.go) |
| Binary/command names | `chimera`, `chimera-indexer`, `locus-desktop`; make variables now `CHIMERA_*` | [../../Makefile](../../Makefile), [../../.goreleaser.yaml](../../.goreleaser.yaml), [../../scripts/chimera-names.sh](../../scripts/chimera-names.sh) |
| Indexer state/config dirs | `.chimera/*` | [../../internal/indexer/config.go](../../internal/indexer/config.go), [../../config/indexer.example.yaml](../../config/indexer.example.yaml), [../../.gitignore](../../.gitignore) |

### 2) Binary and artifact location map

| Surface | Current behavior | Source of truth |
|--------|-------------------|-----------------|
| Gateway build target | `make chimera-build` builds `-o chimera ./cmd/chimera` | [../../Makefile](../../Makefile) |
| Indexer build target | `make indexer-build` builds `chimera-indexer[.exe]` from `./porcelain/chimera/chimera-indexer` | [../../Makefile](../../Makefile) |
| Desktop build script | Builds `./${CHIMERA_CMD_GATEWAY}` where `CHIMERA_CMD_GATEWAY=cmd/chimera` | [../../scripts/chimera-names.sh](../../scripts/chimera-names.sh), [../../scripts/desktop-build.sh](../../scripts/desktop-build.sh) |
| Release archives | GoReleaser uses `project_name: chimera`, binary `chimera`, main `./cmd/chimera` | [../../.goreleaser.yaml](../../.goreleaser.yaml) |
| Personal package | `scripts/release-package.sh` expects desktop binary and emits bundle under `dist/personal/${CHIMERA_DIST_BUNDLE_PREFIX}_<os>_<arch>` | [../../scripts/release-package.sh](../../scripts/release-package.sh) |
| Bootstrap third-party bins | BiFrost + Qdrant installed into `bin/` via pinned deps | [../../scripts/install-bootstrap.sh](../../scripts/install-bootstrap.sh), [../../scripts/qdrant-from-release.sh](../../scripts/qdrant-from-release.sh) |

### 3) Runtime config/data/log/run path map

| Category | Current defaults | Source of truth |
|----------|------------------|-----------------|
| Gateway config path | `$CHIMERA_GATEWAY_CONFIG` else `./config/gateway.yaml` | [../../internal/config/config.go](../../internal/config/config.go) |
| Gateway credentials file key/path | `paths.tokens` -> `./tokens.yaml` | [../../config/gateway.example.yaml](../../config/gateway.example.yaml), [../../internal/config/config.go](../../internal/config/config.go) |
| Gateway metrics DB | `../data/gateway/metrics.sqlite` relative to gateway config | [../../internal/config/config.go](../../internal/config/config.go), [../../config/gateway.example.yaml](../../config/gateway.example.yaml) |
| Gateway operator DB | `../data/gateway/operator.sqlite` relative to gateway config | [../../internal/config/config.go](../../internal/config/config.go), [../../config/gateway.example.yaml](../../config/gateway.example.yaml) |
| Supervised indexer merged config | `../data/gateway/indexer.supervised.yaml` relative to gateway config | [../../internal/config/config.go](../../internal/config/config.go), [../../config/gateway.example.yaml](../../config/gateway.example.yaml) |
| Indexer layered config | `~/.chimera/indexer.config.yaml`, `<cwd>/.chimera/indexer.config.yaml`, optional `--config` | [../../internal/indexer/config.go](../../internal/indexer/config.go) |
| Indexer sync state | default `.chimera/indexer.sync-state.json` or next to explicit `--config` | [../../internal/indexer/config.go](../../internal/indexer/config.go) |
| Runtime directories | `run/`, `logs/`, `data/bifrost`, `data/qdrant`, `data/gateway` | [../../scripts/chimera-start.sh](../../scripts/chimera-start.sh), [../../scripts/clean-data.sh](../../scripts/clean-data.sh), [../../scripts/clean-all.sh](../../scripts/clean-all.sh), [../../.gitignore](../../.gitignore) |

### 4) Packaging and build validation evidence

Executed during this phase:

- `make release-snapshot`: succeeded and produced `dist/chimera_...` archives (linux/darwin/windows amd64+arm64 where configured), plus `checksums.txt`.
- Archive inspection verified expected packaged files: `chimera[.exe]`, `qdrant[.exe]`, `config/gateway.yaml`, `config/tokens.example.yaml`, `config/bifrost.config.json`, `config/routing-policy.yaml`, `config/provider-free-tier.yaml`, docs/readme files.
- `make chimera-build`: failed (`stat .../cmd/chimera: directory not found`).
- `make package`: failed via `scripts/desktop-build.sh` for same command path mismatch (`cmd/chimera` not found).

Interpretation:

- Release pipeline is still coherent for legacy `chimera` naming.
- Make/script naming migration is partially applied and currently inconsistent with existing `cmd/` entrypoint directories.
- Phase 2 must resolve command-path and binary-name source-of-truth first, before broad rename rollout.

### 5) Rename decision matrix (hard-cut policy)

Selected policy from planning: **hard cut** for legacy `CHIMERA_*` env vars and `X-Chimera-*` headers in this train.

| Symbol class | Current | Target direction | Phase |
|--------------|---------|------------------|-------|
| Product terms in operator docs/UI | Mixed `Chimera`/`Chimera`/`Porcelain` | Layered naming consistency (`Chimera` gateway, `Porcelain` suite, `Locus` workspace side) | 2 |
| Gateway credentials naming | `tokens.yaml`, `paths.tokens`, row field `token` language in docs/comments | `api-keys.yaml`, `paths.api_keys`, row field `secret`/`api_keys` wording | 2-3 |
| Env vars | `CHIMERA_*` | New namespace (final prefix to be defined in Phase 2 constants) with no dual read | 3-4 |
| HTTP headers | `X-Chimera-*` | New namespace (final prefix to be defined in Phase 2 constants) with no dual read | 4 |
| Binary/entrypoint names | Mixed `chimera*` and `chimera*` wiring | Single coherent command/package naming across Make/scripts/GoReleaser/cmd dirs | 2-3 |
| Runtime hidden state dirs | `.chimera/*` | New hidden directory naming (single target contract) | 3-4 |
| Release artifact names | `chimera_*` archives and `chimera` binary in release | Align release naming to chosen product/binary contract | 3 |

### 5.1) Proposed service and suite naming strategy

This section captures your proposed target naming taxonomy as the working direction for Phase 2 scoping.

#### Backend services (Chimera)

| Domain | Target name | Responsibility |
|--------|-------------|----------------|
| Supervisor | `chimera-supervisor` | Orchestrates and manages all services |
| Gateway | `chimera-gateway` | API surface, routing, auth |
| Indexer | `chimera-indexer` | Ingestion, embeddings, vectorization |
| BiFrost bridge | `chimera-bifrost` | Inter-service bridge / event bus |
| Vector store adapter | `chimera-vectorstore` | Wrapper for Qdrant/Milvus/etc. |
| Local LLM (future) | `chimera-llm` | Future local LLM server |
| Router model service (future) | `chimera-router` | Future router model service |

#### Clients (Locus)

| Domain | Target name | Responsibility |
|--------|-------------|----------------|
| Desktop | `locus-desktop` | Desktop client; launches/uses standalone supervisor binary |
| CLI (future) | `locus-cli` | Future CLI |
| Mobile (future) | `locus-mobile` | Future mobile client |

#### Suite-level (Porcelain)

| Domain | Target name | Responsibility |
|--------|-------------|----------------|
| Global config | `porcelain-config` | Global config |
| Metadata/versioning | `porcelain-meta` | Metadata and versioning |
| Docs | `porcelain-docs` | Documentation |

### 5.2) Proposed repository topology (target architecture)

This is the long-term target structure to align naming boundaries. Phase 2-4 should treat this as directional architecture, not an all-at-once move in a single PR.

```text
porcelain/
|
|-- chimera/
|   |-- chimera-supervisor/
|   |-- chimera-gateway/
|   |-- chimera-indexer/
|   |-- chimera-bifrost/
|   |-- chimera-vectorstore/
|   `-- shared/
|
|-- locus/
|   `-- locus-desktop/
|
`-- porcelain/
    |-- porcelain-config/
    |-- porcelain-meta/
    `-- porcelain-docs/
```

Migration note:

- For this repo, Phase 2 should first map these names onto current `cmd/`, `internal/`, `config/`, and `docs/` boundaries before any physical directory split/repo restructuring.
- Directory-level re-layout is higher-risk than symbol renaming; sequence it after contracts/build/test/package stability.

### 5.3) Proposed Make target namespace

Adopt this as the target-state naming catalog for Make tasks:

```make
# Porcelain (suite)
porcelain-bootstrap:
porcelain-build-all:
porcelain-clean:
porcelain-release:

# Chimera - Supervisor
chimera-supervisor-build:
chimera-supervisor-run:
chimera-supervisor-test:

# Chimera - Gateway
chimera-gateway-build:
chimera-gateway-run:
chimera-gateway-test:

# Chimera - Indexer
chimera-indexer-build:
chimera-indexer-run:
chimera-indexer-test:

# Chimera - BiFrost
chimera-bifrost-build:
chimera-bifrost-run:

# Chimera - Vectorstore
chimera-vectorstore-start:
chimera-vectorstore-stop:

# Locus - Desktop
locus-desktop-dev:
locus-desktop-build:
locus-desktop-run:

# Cross-cutting
chimera-run-all:
chimera-stop-all:
chimera-build-all:
```

Phase mapping for Make adoption:

- **Phase 2:** introduce canonical names and ownership map, plus aliases for currently used operator flows where needed during migration. This includes first-class split targets for `chimera-supervisor-*` and `locus-desktop-*`.
- **Phase 3:** align packaging/release/install/docs to new canonical targets.
- **Phase 4:** remove stale task names after code/test parity and docs migration are complete.

### 5.4) New deliverable split: supervisor binary vs desktop binary

Add an explicit two-deliverable architecture to avoid coupling desktop packaging with service orchestration internals.

| Deliverable | Target binary | Scope |
|-------------|---------------|-------|
| Supervisor runtime | `chimera-supervisor` | Service lifecycle/orchestration for gateway, indexer, BiFrost bridge, and vectorstore processes |
| Desktop client | `locus-desktop` | UI shell that calls/uses `chimera-supervisor` as an external runtime dependency |

Implementation direction:

- Create a dedicated supervisor entrypoint/binary (instead of only embedding supervisor behavior inside desktop/gateway binaries).
- Make `locus-desktop` consume supervisor via process invocation and control API/IPC contract.
- Keep this as two independently buildable/testable outputs with separate targets and packaging checks.

Phase assignment:

- **Phase 2:** define binary names, command paths, and make target ownership for both deliverables.
- **Phase 3:** package both artifacts and verify layout/runtime path contracts (desktop bundle includes supervisor binary).
- **Phase 4:** separate shared code boundaries and tests so supervisor and desktop can evolve independently.

### 6) PowerShell-assisted rollout strategy

Given your request to avoid heavy manual edits:

- Use scripted bulk transforms for mechanically safe string classes in code/config/scripts/constants first.
- Use scoped command batches by class (scripts/make, then Go constants/source, then tests, then markdown docs last).
- Keep semantic/manual review for behavior-bearing symbols (env/header parsing, config keys, migration/compat logic, filenames and paths).
- After each batch: run targeted compile/tests and check package outputs before moving to next class.

**Acceptance**

- Discovery inventory exists with concrete file-backed maps for symbols, binaries, paths, and package surfaces.
- Build/package validation evidence is captured, including the current Make/script path mismatch.
- Hard-cut decision matrix is explicit and ready to drive implementation phases.

**Status:** `done`

---

## Phase 2 - Constants and source naming contracts

**Goal.** Establish canonical constants and source-level naming contracts before broad implementation churn.

**Deliverables**

- Select final prefixes/names for env/header/bin/archive/runtime-dir contracts.
- Define shared constants in scripts/make/Go where practical.
- Apply source-code comments/help-text renames in non-Markdown files only.
- Define separate build/run/test contracts for `chimera-supervisor` and `locus-desktop`.

**Acceptance**

- Naming constants exist and are referenced from all high-traffic entry surfaces.

**Status:** `done`

---

## Phase 3 - Migration and operational surfaces

**Goal.** Move config/data/rules/CI/assets to the selected naming contract.

**Deliverables**

- Config key/file/path migrations.
- Data/runtime directory renames and cleanup scripts.
- CI/release/workflow/asset updates.
- Packaging updates that produce and validate both `chimera-supervisor` and `locus-desktop` artifacts.

**Acceptance**

- Operational surfaces and package contracts match docs and scripts.

**Status:** `done`

---

## Phase 4 - Go source and tests

**Goal.** Complete code and test migrations for renamed env/header/path/binary contracts.

**Deliverables**

- Update gateway/indexer Go source, tests, fixtures, and UI-backed constants.
- Reconcile startup logs, response headers, and validation errors with final names.
- Refactor supervisor and desktop code paths into separate deliverables with explicit integration tests across the binary boundary.

**Acceptance**

- Affected builds/tests pass with renamed contracts and no stale primary identifiers.

**Status:** `done`

---

## Phase 5 - Markdown documentation rename pass

**Goal.** Complete the coordinated markdown-only rename pass after code/config/packaging surfaces stabilize.

**Deliverables**

- Update Markdown naming across `README`, `docs/`, and plan docs to current implemented contracts.
- Refresh command examples, binary names, paths, env vars, and header names for consistency.
- Keep legacy names only where they are explicitly migration aliases (not primary identity).

**Acceptance**

- Operator-facing docs no longer present legacy names as primary.
- Docs examples match current build/package outputs and supported aliases.

**Status:** `done`

---

## Phase 6 - Documentation consolidation

**Goal.** Normalize documentation after the rename pass and clearly separate historical context from current behavior.

**Deliverables**

- Resolve stale historical statements in planning docs that still read as present-state mismatches.
- Add explicit historical labels where old behavior is retained for reference.
- Ensure docs reference the same canonical build/run/package contracts and entrypoint paths.

**Acceptance**

- No contradictory docs for current runtime/packaging behavior.
- Historical notes are explicitly marked and not mistaken for current requirements.

**Implementation notes**

- Historical snapshots in this document remain for discovery traceability, but they are historical-only and not present-state requirements.
- Canonical current contracts are hard cut (`CHIMERA_*`, `X-Chimera-*`, `paths.api_keys`, `.locus`, `chimera*`/`locus-desktop` artifacts).
- Legacy compatibility is not provided; operators are expected to use only the current naming workflow.

**Status:** `done`

---

## Phase 7 - Legacy alias policy lock

**Goal.** Define final compatibility policy for legacy names and enforce it through docs/tests.

**Deliverables**

- Decide keep/remove timelines for legacy env/header/file aliases.
- Document deprecation windows and operator migration expectations.
- Add/adjust targeted tests for retained alias behavior and removal boundaries.

**Acceptance**

- Compatibility matrix is explicit and internally consistent.
- Alias behavior in code, tests, and docs matches policy.

**Status:** `done` (hard-cut policy: no legacy alias support)

---

## Phase 8 - Topology and entrypoint restructure

**Goal.** Execute deferred directory/entrypoint restructuring only after naming/docs stability.

**Deliverables**

- Move entrypoint directories to approved target topology (if in-scope).
- Update build scripts, Make targets, GoReleaser, CI, and tests to new paths.
- Provide contributor migration notes for changed dev/build commands.

**Acceptance**

- Build/test/package workflows pass with new paths.
- No unresolved command-path drift remains in scripts or CI.

**Implementation notes**

- Entrypoint topology moved to `cmd/chimera` and `porcelain/chimera/chimera-indexer`.
- Build/release/CI contracts now target the restructured paths (`Makefile`, `.goreleaser.yaml`, `scripts/chimera-names.sh`, `.github/workflows/go.yml`).
- Desktop bridge function names were aligned to Chimera (`chimeraPickFolder`, `chimeraOpenExternalURL`, `chimeraRevealProjectPath`) with UI wiring updated.

**Status:** `done`

---

## Phase 9 - Final cutover and removal pass

**Goal.** Remove retired legacy naming and close the migration train.

**Deliverables**

- Remove aliases and fallback logic that policy marks as retired.
- Delete stale docs/examples and update migration guide closure notes.
- Run final repo-wide audit for unintended legacy-primary naming.

**Acceptance**

- Target-state naming is consistent across code/config/scripts/docs.
- Remaining legacy names (if any) are explicitly intentional.

**Implementation notes**

- Removed legacy alias/fallback contracts for env/header/config/runtime naming in active runtime surfaces.
- Completed cmd entrypoint cutover by deleting retired `cmd/chimera*` entrypoint sources after `cmd/chimera*` migration.
- Updated migration and operator docs to reflect hard-cut final state (legacy aliases are retired/unsupported).

**Status:** `done`

---

## Open questions

1. Which of the proposed new Make targets should be introduced first as migration entry points (`*-build`, `*-run`, or suite-level umbrella targets).
2. Whether Go module path/repo rename and physical directory split to the proposed topology are deferred (recommended: defer and document).

---

## References

- Build/release/package: [../../Makefile](../../Makefile), [../../.goreleaser.yaml](../../.goreleaser.yaml), [../../scripts/chimera-names.sh](../../scripts/chimera-names.sh), [../../scripts/desktop-build.sh](../../scripts/desktop-build.sh), [../../scripts/release-package.sh](../../scripts/release-package.sh), [../../scripts/release-snapshot.sh](../../scripts/release-snapshot.sh), [../../scripts/install-bootstrap.sh](../../scripts/install-bootstrap.sh), [../../scripts/qdrant-from-release.sh](../../scripts/qdrant-from-release.sh)
- Runtime/config/indexer: [../../internal/config/config.go](../../internal/config/config.go), [../../internal/indexer/config.go](../../internal/indexer/config.go), [../../internal/server/ingest.go](../../internal/server/ingest.go), [../../internal/server/server.go](../../internal/server/server.go), [../../internal/supervisor/indexer.go](../../internal/supervisor/indexer.go), [../../config/gateway.example.yaml](../../config/gateway.example.yaml), [../../config/indexer.example.yaml](../../config/indexer.example.yaml)
- Docs/product direction: [../version-v0.3.md](../version-v0.3.md), [../configuration.md](../configuration.md), [../packaging.md](../packaging.md), [../../README.md](../../README.md)
