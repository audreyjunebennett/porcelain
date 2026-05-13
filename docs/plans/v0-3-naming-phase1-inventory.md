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
| [Phase 2 - Constants and source naming contracts](#phase-2---constants-and-source-naming-contracts) | Centralize naming constants and define source-level rename contracts | `todo` |
| [Phase 3 - Migration and operational surfaces](#phase-3---migration-and-operational-surfaces) | Config/data/rules/CI/assets migration and packaging updates | `todo` |
| [Phase 4 - Go source and tests](#phase-4---go-source-and-tests) | Runtime source + tests updated to final naming contracts | `todo` |
| [Phase 5 - Markdown documentation rename pass](#phase-5---markdown-documentation-rename-pass) | Markdown docs updated after source/folder churn settles | `todo` |

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
| Product naming | `Claudia`, `Chimera`, `Porcelain`, `Locus` | [../../README.md](../../README.md), [../version-v0.3.md](../version-v0.3.md), [../../cmd/claudia/main.go](../../cmd/claudia/main.go), [../../internal/server/embedui/](../../internal/server/embedui/) |
| Env vars | `CLAUDIA_*` (gateway/indexer/upstream/session), partial `CHIMERA_*` in scripts | [../../internal/config/config.go](../../internal/config/config.go), [../../internal/indexer/config.go](../../internal/indexer/config.go), [../../scripts/chimera-names.sh](../../scripts/chimera-names.sh) |
| HTTP headers | `X-Claudia-*` | [../../internal/server/ingest.go](../../internal/server/ingest.go), [../../internal/server/server.go](../../internal/server/server.go), [../../internal/indexer/client.go](../../internal/indexer/client.go) |
| Binary/command names | `claudia`, `claudia-index`, `claudia-desktop`; make variables now `CHIMERA_*` | [../../Makefile](../../Makefile), [../../.goreleaser.yaml](../../.goreleaser.yaml), [../../scripts/chimera-names.sh](../../scripts/chimera-names.sh) |
| Indexer state/config dirs | `.claudia/*` | [../../internal/indexer/config.go](../../internal/indexer/config.go), [../../config/indexer.example.yaml](../../config/indexer.example.yaml), [../../.gitignore](../../.gitignore) |

### 2) Binary and artifact location map

| Surface | Current behavior | Source of truth |
|--------|-------------------|-----------------|
| Gateway build target | `make chimera-build` builds `-o chimera ./cmd/chimera` (currently mismatched with existing `./cmd/claudia`) | [../../Makefile](../../Makefile) |
| Indexer build target | `make indexer-build` builds `chimera-index[.exe]` from `./cmd/chimera-index` (currently mismatched with existing `./cmd/claudia-index`) | [../../Makefile](../../Makefile) |
| Desktop build script | Builds `./${CHIMERA_CMD_GATEWAY}` where `CHIMERA_CMD_GATEWAY=cmd/chimera` | [../../scripts/chimera-names.sh](../../scripts/chimera-names.sh), [../../scripts/desktop-build.sh](../../scripts/desktop-build.sh) |
| Release archives | GoReleaser still uses `project_name: claudia`, binary `claudia`, main `./cmd/claudia` | [../../.goreleaser.yaml](../../.goreleaser.yaml) |
| Personal package | `scripts/release-package.sh` expects desktop binary and emits bundle under `dist/personal/${CHIMERA_DIST_BUNDLE_PREFIX}_<os>_<arch>` | [../../scripts/release-package.sh](../../scripts/release-package.sh) |
| Bootstrap third-party bins | BiFrost + Qdrant installed into `bin/` via pinned deps | [../../scripts/install-bootstrap.sh](../../scripts/install-bootstrap.sh), [../../scripts/qdrant-from-release.sh](../../scripts/qdrant-from-release.sh) |

### 3) Runtime config/data/log/run path map

| Category | Current defaults | Source of truth |
|----------|------------------|-----------------|
| Gateway config path | `$CLAUDIA_GATEWAY_CONFIG` else `./config/gateway.yaml` | [../../internal/config/config.go](../../internal/config/config.go) |
| Gateway credentials file key/path | `paths.tokens` -> `./tokens.yaml` | [../../config/gateway.example.yaml](../../config/gateway.example.yaml), [../../internal/config/config.go](../../internal/config/config.go) |
| Gateway metrics DB | `../data/gateway/metrics.sqlite` relative to gateway config | [../../internal/config/config.go](../../internal/config/config.go), [../../config/gateway.example.yaml](../../config/gateway.example.yaml) |
| Gateway operator DB | `../data/gateway/operator.sqlite` relative to gateway config | [../../internal/config/config.go](../../internal/config/config.go), [../../config/gateway.example.yaml](../../config/gateway.example.yaml) |
| Supervised indexer merged config | `../data/gateway/indexer.supervised.yaml` relative to gateway config | [../../internal/config/config.go](../../internal/config/config.go), [../../config/gateway.example.yaml](../../config/gateway.example.yaml) |
| Indexer layered config | `~/.claudia/indexer.config.yaml`, `<cwd>/.claudia/indexer.config.yaml`, optional `--config` | [../../internal/indexer/config.go](../../internal/indexer/config.go) |
| Indexer sync state | default `.claudia/indexer.sync-state.json` or next to explicit `--config` | [../../internal/indexer/config.go](../../internal/indexer/config.go) |
| Runtime directories | `run/`, `logs/`, `data/bifrost`, `data/qdrant`, `data/gateway` | [../../scripts/chimera-start.sh](../../scripts/chimera-start.sh), [../../scripts/clean-data.sh](../../scripts/clean-data.sh), [../../scripts/clean-all.sh](../../scripts/clean-all.sh), [../../.gitignore](../../.gitignore) |

### 4) Packaging and build validation evidence

Executed during this phase:

- `make release-snapshot`: succeeded and produced `dist/claudia_...` archives (linux/darwin/windows amd64+arm64 where configured), plus `checksums.txt`.
- Archive inspection verified expected packaged files: `claudia[.exe]`, `qdrant[.exe]`, `config/gateway.yaml`, `config/tokens.example.yaml`, `config/bifrost.config.json`, `config/routing-policy.yaml`, `config/provider-free-tier.yaml`, docs/readme files.
- `make chimera-build`: failed (`stat .../cmd/chimera: directory not found`).
- `make package`: failed via `scripts/desktop-build.sh` for same command path mismatch (`cmd/chimera` not found).

Interpretation:

- Release pipeline is still coherent for legacy `claudia` naming.
- Make/script naming migration is partially applied and currently inconsistent with existing `cmd/` entrypoint directories.
- Phase 2 must resolve command-path and binary-name source-of-truth first, before broad rename rollout.

### 5) Rename decision matrix (hard-cut policy)

Selected policy from planning: **hard cut** for legacy `CLAUDIA_*` env vars and `X-Claudia-*` headers in this train.

| Symbol class | Current | Target direction | Phase |
|--------------|---------|------------------|-------|
| Product terms in operator docs/UI | Mixed `Claudia`/`Chimera`/`Porcelain` | Layered naming consistency (`Chimera` gateway, `Porcelain` suite, `Locus` workspace side) | 2 |
| Gateway credentials naming | `tokens.yaml`, `paths.tokens`, row field `token` language in docs/comments | `api-keys.yaml`, `paths.api_keys`, row field `secret`/`api_keys` wording | 2-3 |
| Env vars | `CLAUDIA_*` | New namespace (final prefix to be defined in Phase 2 constants) with no dual read | 3-4 |
| HTTP headers | `X-Claudia-*` | New namespace (final prefix to be defined in Phase 2 constants) with no dual read | 4 |
| Binary/entrypoint names | Mixed `claudia*` and `chimera*` wiring | Single coherent command/package naming across Make/scripts/GoReleaser/cmd dirs | 2-3 |
| Runtime hidden state dirs | `.claudia/*` | New hidden directory naming (single target contract) | 3-4 |
| Release artifact names | `claudia_*` archives and `claudia` binary in release | Align release naming to chosen product/binary contract | 3 |

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

**Status:** `todo`

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

**Status:** `todo`

---

## Phase 4 - Go source and tests

**Goal.** Complete code and test migrations for renamed env/header/path/binary contracts.

**Deliverables**

- Update gateway/indexer Go source, tests, fixtures, and UI-backed constants.
- Reconcile startup logs, response headers, and validation errors with final names.
- Refactor supervisor and desktop code paths into separate deliverables with explicit integration tests across the binary boundary.

**Acceptance**

- Affected builds/tests pass with renamed contracts and no stale primary identifiers.

**Status:** `todo`

---

## Phase 5 - Markdown documentation rename pass

**Goal.** Apply Markdown documentation renames only after source/config/folder changes stabilize, to reduce repeated churn.

**Deliverables**

- Update Markdown documentation naming across `README`, `docs/`, and plan documents in one coordinated pass.
- Refresh command examples, binary names, paths, environment variables, and header names to match the final implemented contracts.
- Remove stale transitional wording introduced during earlier phases.

**Acceptance**

- Markdown docs align with final source/config/package behavior from Phases 2-4.
- Documentation-only pass minimizes rework caused by earlier file/path renames.

**Status:** `todo`

---

## Open questions

1. Final target prefixes for environment variables and headers under hard-cut policy.
2. Which of the proposed new Make targets should be introduced first as migration entry points (`*-build`, `*-run`, or suite-level umbrella targets).
3. Whether hidden state directory rename from `.claudia` is in this train or a follow-up.
4. Whether Go module path/repo rename and physical directory split to the proposed topology are deferred (recommended: defer and document).

---

## References

- Build/release/package: [../../Makefile](../../Makefile), [../../.goreleaser.yaml](../../.goreleaser.yaml), [../../scripts/chimera-names.sh](../../scripts/chimera-names.sh), [../../scripts/desktop-build.sh](../../scripts/desktop-build.sh), [../../scripts/release-package.sh](../../scripts/release-package.sh), [../../scripts/release-snapshot.sh](../../scripts/release-snapshot.sh), [../../scripts/install-bootstrap.sh](../../scripts/install-bootstrap.sh), [../../scripts/qdrant-from-release.sh](../../scripts/qdrant-from-release.sh)
- Runtime/config/indexer: [../../internal/config/config.go](../../internal/config/config.go), [../../internal/indexer/config.go](../../internal/indexer/config.go), [../../internal/server/ingest.go](../../internal/server/ingest.go), [../../internal/server/server.go](../../internal/server/server.go), [../../internal/supervisor/indexer.go](../../internal/supervisor/indexer.go), [../../config/gateway.example.yaml](../../config/gateway.example.yaml), [../../config/indexer.example.yaml](../../config/indexer.example.yaml)
- Docs/product direction: [../version-v0.3.md](../version-v0.3.md), [../configuration.md](../configuration.md), [../packaging.md](../packaging.md), [../../README.md](../../README.md)
