# Plan: v0.3 naming hard-cut execution

| Field | Value |
|-------|-------|
| **Doc kind** | `refactor-plan` |
| **Owners / areas** | Gateway, indexer, desktop, packaging/release, operator docs |
| **Status** | `active` |
| **Targets** | v0.3 naming hard-cut completion |
| **Last updated** | See git history |
| **Supersedes / superseded by** | Related to `docs/plans/v0-3-naming-phase1-inventory.md` |

## At a glance

This plan finishes the v0.3 naming migration as a true hard cut: remove legacy `chimera-*` contracts, complete the dedicated supervisor binary split, and standardize command/build/package/doc surfaces on `chimera` and `locus`. The work is split into independent phases so multiple agents can execute in parallel without re-discovery.

Sequencing decision: the follow-on wrapper refactor in [`vectorstore-broker-wrapper-hard-cut.md`](vectorstore-broker-wrapper-hard-cut.md) is split into two waves relative to this plan:

1. Contract/spec preparation may run in parallel (wrapper plan Phase 1 only).
2. Runtime implementation/cutover work (wrapper plan Phases 2-6) is gated until this plan reaches Phase 9 closeout.

| Phase | Outcome | Status |
|-------|---------|--------|
| [Phase 1 -- Baseline and contract lock](#phase-1----baseline-and-contract-lock) | Single canonical naming contract and execution map for all agents | `done` |
| [Phase 2 -- Entrypoints and binary ownership](#phase-2----entrypoints-and-binary-ownership) | Dedicated `chimera-supervisor` binary and explicit runtime ownership boundaries | `done` |
| [Phase 3 -- Make and scripts namespace cutover](#phase-3----make-and-scripts-namespace-cutover) | New make namespace only, no `chimera-*` aliases | `done` |
| [Phase 4 -- Packaging, config examples, and file contracts](#phase-4----packaging-config-examples-and-file-contracts) | Artifacts/config use `api-keys` and current naming only | `done` |
| [Phase 5 -- Source and test rename pass](#phase-5----source-and-test-rename-pass) | Legacy naming removed from code/tests/comments/help text | `done` |
| [Phase 6 -- Operator documentation normalization](#phase-6----operator-documentation-normalization) | Operator-facing docs describe only current commands/contracts | `done` |
| [Phase 7 -- Repository layout alignment](#phase-7----repository-layout-alignment) | Define and stage incremental layout convergence slices toward the target `porcelain/chimera/locus` architecture | `done` |
| [Phase 8 -- Repository layout implementation](#phase-8----repository-layout-implementation) | Execute physical directory/path convergence slices from the staged plan | `done` |
| [Phase 9 -- Validation and closeout](#phase-9----validation-and-closeout) | Build/test/package verification and migration closure notes | `todo` |

---

## Background

The current repo has significant progress on runtime contract migration (`CHIMERA_*`, `X-Chimera-*`, `.locus`, `api_keys`) but still contains legacy `chimera` naming on operator/build/package/doc surfaces. The target is full hard cut with no backward aliases and no legacy wording in active surfaces.

Operator decisions already fixed for this execution:

1. Remove all `chimera-*` compatibility targets/aliases.
2. Retire `tokens` naming where no longer required; if runtime expects a file, keep only the canonical file name.
3. Remove legacy naming mentions in comments/examples/tests as part of this pass.
4. Adopt new make namespace directly (no temporary alias period).
5. Introduce a real supervisor binary now.
6. Limit doc rewrite to operator-facing docs in this pass.
7. No extra "legacy unsupported" messaging requirement needed beyond practical current-state docs.
8. Apply renames in tests as well.

Current-state evidence snapshot used for this plan:

- Runtime naming constants already exist in `internal/naming/contracts.go`.
- Gateway/indexer config loaders are mostly on `api_keys` and `CHIMERA_*` (`internal/config/config.go`, `internal/indexer/config.go`).
- `Makefile` still defines many `chimera-*` aliases and does not yet match the full new namespace catalog.
- Packaging still includes legacy config file naming in places (`scripts/release-package.sh`, `.goreleaser.yaml`).
- `config/gateway.example.yaml` and `config/indexer.example.yaml` still include legacy naming in comments/examples.
- Docs include mixed historical/current naming, including operator docs.
- Supervisor binary exists as a build output name but is still built from the gateway entrypoint package path.

**Related docs:** [`v0-3-naming-phase1-inventory.md`](v0-3-naming-phase1-inventory.md), [`../migration-v0-3-naming.md`](../migration-v0-3-naming.md), [`../version-v0.3.md`](../version-v0.3.md), [`../configuration.md`](../configuration.md), [`../supervisor.md`](../supervisor.md), [`../indexer.md`](../indexer.md), [`../packaging.md`](../packaging.md), [`../../README.md`](../../README.md).

---

## Phase 1 -- Baseline and contract lock

**Goal.** Freeze the exact rename contract and establish a shared implementation map so parallel agents do not diverge.

**Implementation notes**

### Canonical rename map (active surfaces)

| Contract class | Retired | Canonical now |
|---|---|---|
| Gateway binary/target namespace | `chimera*` | `chimera-gateway*` |
| Supervisor binary/target namespace | mixed `chimera` serve path only | `chimera-supervisor*` (first-class binary) |
| Indexer binary/target namespace | `chimera-indexer*` | `chimera-indexer*` |
| Desktop binary/target namespace | `chimera-desktop*` | `locus-desktop*` |
| Env vars | `CHIMERA_*` | `CHIMERA_*` |
| HTTP headers | `X-Chimera-*` | `X-Chimera-*` |
| Gateway credential file naming | `tokens.yaml`, `tokens.example.yaml` | `api-keys.yaml`, `api-keys.example.yaml` |
| Gateway config key | `paths.tokens` | `paths.api_keys` |
| Credentials schema wording | row field `token` | row field `secret` under `api_keys` |
| Indexer hidden state dir | `.chimera/` | `.locus/` |

### Resolved decisions (from operator)

1. Canonical gateway naming is fully split now to `chimera-gateway-*`.
2. Retired legacy files are deleted in this train; no historical compatibility files are kept for active flows.

### Execution ownership map for parallel agents

| Owner area | Primary files/dirs | Responsibility boundary |
|---|---|---|
| Entrypoints/runtime wiring | `cmd/`, `internal/server/`, `internal/supervisor/` | Binary/package paths, process orchestration, command help/runtime wiring |
| Make/scripts/release | `Makefile`, `scripts/`, `.goreleaser.yaml`, `.github/workflows/` | Target namespace, script vars/messages, release/package artifact names |
| Config examples/env/operator text | `config/*.example.yaml`, `env.example` | Canonical examples only; remove legacy aliases/comments |
| Tests | `internal/**/*_test.go`, `cmd/**/*_test.go` | Rename legacy symbols/messages/assertions; preserve behavior |
| Operator docs | `README.md`, `docs/configuration.md`, `docs/supervisor.md`, `docs/indexer.md`, `docs/installation.md`, `docs/packaging.md`, `docs/migration-v0-3-naming.md`, `docs/version-v0.3.md` | Operator-facing contract language only; no broad historical rewrite |

### Exclusions for this train

- Historical/archive plans and notes outside the operator-facing doc set above.
- Third-party/vendor content and external snapshots.
- Generated artifacts under `dist/` (re-generated by build/package flows, not manually edited).

### Agent handoff checklist (copy/paste)

1. Confirm your assigned area and do not cross ownership boundaries unless coordinated.
2. Apply canonical map only (`chimera-gateway-*`, `chimera-supervisor-*`, `chimera-indexer-*`, `locus-desktop-*`, `CHIMERA_*`, `X-Chimera-*`, `api-keys*`, `.locus`).
3. Remove legacy aliases and compatibility paths in your scope; do not add temporary dual support.
4. Update user-visible text/comments/examples/tests in scope to remove `chimera` legacy wording.
5. Keep behavior stable unless rename contract requires structural change (for example, dedicated supervisor binary).
6. Validate only your touched surfaces, then run a scoped legacy scan before handoff.
7. Report any blocked dependency on another area as a concrete file-level handoff item.

**Deliverables**

- Produce a canonical rename map for symbols and files (old -> new) scoped to active surfaces.
- Partition execution ownership by subsystem:
  - Entrypoints/runtime wiring
  - Make/scripts/release
  - Config examples/env/operator text
  - Tests
  - Operator docs
- Mark explicit exclusions for this train:
  - Historical/archive plans outside operator path
  - Third-party/vendor content
  - Generated release artifacts under `dist/`
- Create a short "handoff checklist" artifact in this plan section (copyable by each agent).

**Acceptance**

- All agent work uses the same canonical targets and file contracts.
- No agent needs fresh discovery to begin implementation.
- Phase owners and boundaries are explicit.

**Status:** `done`

---

## Phase 2 -- Entrypoints and binary ownership

**Goal.** Implement dedicated runtime binaries with clear ownership (`chimera-gateway`, `chimera-supervisor`, `locus-desktop`) and remove legacy entrypoint assumptions.

**Implementation notes**

- Added first-class gateway entrypoint package: `cmd/chimera-gateway`.
- Added first-class supervisor entrypoint package: `cmd/chimera-supervisor`.
- Kept desktop entrypoint behavior under `cmd/chimera` for `locus-desktop` builds and `tokencount`.
- Rewired build/run contracts to explicit package ownership:
  - gateway build/run: `./cmd/chimera-gateway`
  - supervisor build/run: `./cmd/chimera-supervisor`
  - desktop build/test/vet and tokencount: `./cmd/chimera`
- Updated startup/packaging scripts to resolve and execute the supervisor binary for supervised background flows.
- Updated canonical names script:
  - gateway binary base now `chimera-gateway`
  - added `CHIMERA_CMD_SUPERVISOR`, `CHIMERA_CMD_DESKTOP`, and `CHIMERA_CMD_TOKENCOUNT`
  - log path now tracks supervisor runtime log basename.
- Validation run:
  - `go build ./cmd/chimera ./cmd/chimera-gateway ./cmd/chimera-supervisor`
  - `make chimera-build`
  - `make chimera-supervisor-build`

**Deliverables**

- Add dedicated supervisor entrypoint package (`cmd/chimera-supervisor`) with runtime behavior currently launched via `serve` mode.
- Ensure gateway-only entrypoint contract is explicit and stable (`cmd/chimera` and/or dedicated gateway package per final implementation).
- Update runtime/help text/comments so naming and command examples no longer reference `chimera`.
- Remove legacy command wording from command help and startup hints.
- Adjust any process-spawn logic expecting old binary names/paths.

**Acceptance**

- `chimera-supervisor` builds/runs as a first-class binary.
- Desktop path invokes/uses supervisor via explicit binary contract.
- No active entrypoint/help text references `chimera`.

**Status:** `done`

---

## Phase 3 -- Make and scripts namespace cutover

**Goal.** Replace mixed/legacy make and script names with the new namespace only.

**Implementation notes**

- Replaced old make target namespace with canonical targets:
  - `chimera-gateway-build`, `chimera-gateway-run`, `chimera-gateway-test`
  - `chimera-supervisor-build`, `chimera-supervisor-run`, `chimera-supervisor-test`
  - `chimera-indexer-build`, `chimera-indexer-run`, `chimera-indexer-install`, `chimera-indexer-test`
  - `locus-desktop-install`, `locus-desktop-build`, `locus-desktop-run`
  - Cross-cutting: `chimera-build-all`, `chimera-run-all`, `chimera-stop-all`
- Removed `chimera-*` and legacy alias targets from `Makefile` (`.PHONY` and target bodies).
- Updated script constants in `scripts/chimera-names.sh` to canonical make-target names and added canonical helper vars:
  - `CHIMERA_MAKE_BUILD_ALL_TARGET`
  - `CHIMERA_MAKE_INDEXER_*_TARGET`
  - `CHIMERA_MAKE_DESKTOP_INSTALL_TARGET`
  - `CHIMERA_MAKE_PID_BASENAME`
- Updated script behavior/messages to use canonical target names:
  - `scripts/print-make-help.sh`
  - `scripts/desktop-run.sh`
  - `scripts/desktop-build.sh`
  - `scripts/desktop-install.sh`
  - `scripts/chimera-start.sh`
  - `scripts/chimera-stop.sh`
  - `scripts/clean.sh`
  - `scripts/clean-all-confirm.sh`
  - `scripts/install-bootstrap.sh`
  - `scripts/msys2-gcc-path.sh`
  - `scripts/clean-data.sh`
- Canonical background supervisor state now uses:
  - PID path: `run/chimera-supervisor.pid`
  - log path: `logs/chimera-supervisor.log`

**Validation run**

- `make help`
- `make chimera-gateway-build && make chimera-supervisor-build && make chimera-indexer-build`

**Deliverables**

- Replace old make target aliases and publish only canonical targets:
  - `chimera-supervisor-*`, `chimera-gateway-*`, `chimera-indexer-*`, `locus-desktop-*`, and selected cross-cutting targets.
- Update scripts consuming make variables/target names (`scripts/chimera-names.sh`, `scripts/print-make-help.sh`, shell helpers).
- Ensure script comments/messages/examples use canonical names only.
- Remove stale references to retired targets from make help and script usage text.

**Acceptance**

- `make help` shows only canonical target namespace.
- No `chimera-*` targets remain in `Makefile`.
- Build/run/install/package scripts resolve canonical targets and binaries without aliases.

**Status:** `done`

---

## Phase 4 -- Packaging, config examples, and file contracts

**Goal.** Normalize packaging and config artifacts to current naming and remove retired credential file contracts.

**Implementation notes**

- Retired legacy credentials example file deleted:
  - `config/tokens.example.yaml` (removed from repo)
- Packaging/release contracts updated to canonical credential naming only:
  - removed `config/tokens.example.yaml` from `.goreleaser.yaml` archive files
  - removed `config/tokens.example.yaml` copy step from `scripts/release-package.sh`
- GoReleaser primary binary contract aligned to canonical gateway binary:
  - build id renamed to `chimera-gateway`
  - `main` switched to `./cmd/chimera-gateway`
  - release binary switched to `chimera-gateway`
- Config examples normalized:
  - `config/gateway.example.yaml` legacy comments updated (`X-Chimera-*`, `chimera-supervisor`, `chimera-index`, virtual Chimera wording)
  - `config/indexer.example.yaml` legacy header/comment naming updated (`X-Chimera-*`, `.chimeraignore`)
  - `config/api-keys.example.yaml` removed legacy compatibility block (`tokens` / `token`)
  - `env.example` now references canonical binaries (`chimera-gateway`, `chimera-supervisor`, `chimera-index`)
- Packaging readme normalized to canonical names:
  - `packaging/archive-readme.txt` now references `chimera-gateway`, `chimera-supervisor`, `api-keys.yaml`, `api-keys.example.yaml`
- Ignore contracts normalized:
  - `.gitignore` removed `config/tokens.yaml`, removed `/.chimera/`, removed legacy binary names (`chimera*`)
  - `.gitignore` now tracks canonical gateway/indexer binaries (`/chimera-gateway*`, `/chimera-index*`) and existing supervisor/desktop outputs

**Validation run**

- `make chimera-gateway-build && make chimera-supervisor-build && make chimera-indexer-build`
- Scoped contract scan on changed Phase 4 surfaces confirms no remaining `tokens.example.yaml` / `paths.tokens` / `.chimera` in active packaging/config examples.

**Deliverables**

- Remove legacy credential packaging paths and copies:
  - retire `config/tokens.example.yaml` from release/package outputs where unnecessary.
- Ensure package/release includes canonical credentials example only (`api-keys.example.yaml`) and matching docs.
- Update `.goreleaser.yaml` and packaging scripts for final file list and names.
- Update config examples to remove legacy comment aliases:
  - `config/gateway.example.yaml`
  - `config/indexer.example.yaml`
  - `env.example` when applicable
- Remove stale ignore/runtime path references tied to `.chimera` or retired names where still present.

**Acceptance**

- Packaged bundles contain only canonical credentials file contracts.
- Config examples contain no `chimera` legacy wording on active paths.
- Release/package flow aligns with operator-facing migration docs.

**Status:** `done`

---

## Phase 5 -- Source and test rename pass

**Goal.** Remove remaining legacy naming in runtime source, tests, and user-visible strings.

**Implementation notes**

- Renamed remaining legacy runtime/test naming on active code paths:
  - provider key naming contract from `chimera-<provider>-key-<n>` to `chimera-<provider>-key-<n>` in merge/sort logic and UI-facing tests.
  - vector collection prefix from `chimera-` to `chimera-` in runtime derivation plus Go and logs-UI derivation tests.
  - UI session cookie default from `chimera_ui_session` to `chimera_ui_session` and aligned login/session tests.
- Updated user-facing strings and comments tied to active behavior:
  - login page title/header changed to Chimera branding.
  - supervised/runtime hints/comments updated from `chimera serve` / `chimera-indexer` / `chimera-gateway` wording to canonical names.
  - temporary config filename prefixes normalized from `chimera-gw-*` to `chimera-gw-*`.
- Validation run:
  - `go test ./internal/bifrostadmin ./internal/vectorstore ./internal/vectorstore/qdrant ./internal/server ./internal/indexer ./internal/config ./internal/supervisor ./internal/freecatalog ./internal/servicelogs`
  - `ReadLints` on touched packages (clean)

**Deliverables**

- Rename remaining `chimera` terminology in code comments/log strings/user messages where tied to active behavior.
- Update test names, fixtures, assertions, and snapshots to canonical naming.
- Remove stale legacy constants/vars/function names where they survive as non-functional leftovers.
- Keep behavior unchanged except where required for hard-cut naming contract.

**Acceptance**

- Repo-wide active-source/test scan for `chimera`, `CHIMERA_`, `X-Chimera-`, `.chimera`, `paths.tokens`, `tokens.yaml` is clean or explicitly justified.
- Tests compile and pass with updated naming.

**Status:** `done`

---

## Phase 6 -- Operator documentation normalization

**Goal.** Update operator-facing docs only so they describe the current canonical behavior and commands.

**Deliverables**

- Update operator docs:
  - `README.md`
  - `docs/configuration.md`
  - `docs/supervisor.md`
  - `docs/indexer.md`
  - `docs/installation.md`
  - `docs/packaging.md`
  - `docs/migration-v0-3-naming.md`
  - `docs/version-v0.3.md` (operator-relevant sections)
- Align command examples with final make namespace and binary contracts.
- Remove legacy aliases from operator examples and migration instructions.
- Preserve historical context only where still useful for operator migration; avoid broad rewrite of non-operator historical plans in this train.

**Acceptance**

- Operator docs show one coherent contract: canonical binaries, targets, env vars, headers, and file names.
- No contradictory old command/file guidance remains in operator path docs.

**Implementation notes**

- Normalized operator-facing docs to canonical naming contracts across:
  - `README.md`
  - `docs/configuration.md`
  - `docs/supervisor.md`
  - `docs/indexer.md`
  - `docs/installation.md`
  - `docs/packaging.md`
- Removed legacy alias wording from active operator instructions (commands, env/header references, and credential file guidance), while preserving migration history in `docs/migration-v0-3-naming.md`.
- Aligned command and operational examples to canonical make targets and runtime artifacts (`chimera-supervisor-run`, `locus-desktop-*`, `chimera-indexer-*`, `chimera-index`, `logs/chimera-supervisor.log`, `run/chimera-supervisor.pid`).
- Validation run:
  - Scoped legacy scans across Phase 6 operator docs for `CHIMERA_*`, `X-Chimera-*`, `paths.tokens`, `tokens.yaml`, and legacy alias command wording.
  - `ReadLints` on changed docs (clean).

**Status:** `done`

---

## Phase 7 -- Repository layout alignment

**Goal.** Add explicit execution work for the long-term target structure captured in `docs/plans/v0-3-naming-phase1-inventory.md`, and move toward those boundaries without forcing an all-at-once repo move.

**Directional target layout** (from Phase 1 inventory, lines 147-167):

```text
porcelain/
|
|-- chimera/
|   |-- chimera-supervisor/
|   |-- chimera-gateway/
|   |-- chimera-indexer/
|   |-- chimera-broker (currently bifrost)/
|   |-- chimera-vectorstore (currently qdrant)/
|   `-- internal/
|
|-- locus/
|   `-- locus-desktop/
|
`-- porcelain/
    |-- porcelain-config/
    |-- porcelain-meta/
    `-- porcelain-docs/
```

**Deliverables**

- Define and document the incremental cut plan from current repo paths (`cmd/`, `internal/`, `docs/`, `config/`, `scripts/`) into the directional target layout above.
- Identify first safe extraction/move slices that can land independently (for example package boundary moves, module/internal boundary prep, path ownership docs) without breaking runtime contracts.
- Record required compatibility decisions for moved paths (imports, build targets, packaging inputs, CI paths) so later phases avoid ad-hoc moves.
- Keep this as directional architecture work; do not require a single PR to fully materialize the entire tree.

**Implementation notes**

### Sequencing model

Layout alignment executes as a set of low-risk preparation slices before any physical top-level directory split. The required order is:

1. Ownership and boundary documentation.
2. Package/API boundary stabilization.
3. Build/path indirection hardening.
4. Optional directory moves after all call-sites use stable indirection.

This keeps runtime and release surfaces stable while converging toward target structure.

### Incremental cut plan (execution slices)

| Slice | Primary scope | Why this lands safely now | Exit criteria |
|---|---|---|---|
| S1 -- Ownership map freeze | `docs/plans/`, `docs/` references to package ownership | Documentation-only; no runtime risk | Clear ownership matrix for `cmd/`, `internal/`, `scripts/`, `docs/`, `config/` approved |
| S2 -- Internal package boundary prep | `internal/server`, `internal/supervisor`, `internal/indexer`, `internal/config` | Refactor-only moves behind existing imports and interfaces | No call-site behavior changes; tests pass unchanged |
| S3 -- Entrypoint path indirection | `Makefile`, scripts, release config, CI paths | Existing canonical vars already present; strengthens move readiness | All build/run/package flows consume variable indirection only (no hard-coded `cmd/*` path assumptions) |
| S4 -- Docs/config path abstraction | `docs/*.md`, config examples, packaging docs | Operator docs can reference contracts rather than concrete repo paths | Docs explain contract paths and generated artifacts without depending on final tree move |
| S5 -- Optional physical moves (deferred by default) | selected `cmd/*` and supporting packages | Highest risk; only after S1-S4 completed and validated | Move can happen as mechanical path rewrite with no runtime semantic change |

### Compatibility decisions for later moves

1. **Go imports:** preserve package APIs during boundary prep; avoid rename+behavior changes in same PR.
2. **Build targets:** `Makefile` and scripts must resolve package paths through canonical variables/constants only.
3. **Release inputs:** `.goreleaser.yaml` and packaging scripts must reference canonical build outputs, not transient source layout assumptions.
4. **CI paths:** workflow steps should key off make targets and package lists, not fragile directory assumptions.
5. **Desktop/supervisor coupling:** maintain current binary contracts while path internals are reorganized.

### Explicit non-goals for this phase

- No single-step repo tree rewrite to the full target topology.
- No module split into multiple Go modules in this phase.
- No behavioral runtime changes hidden inside path-move PRs.
- No vendor/third-party content relocation.

### Handoff contract for Phase 8

Before Phase 8 implementation starts, record for each completed slice:

- files/surfaces touched,
- compatibility risks introduced/retired,
- validation commands executed,
- deferred move items that remain for post-v0.3 execution.

**Acceptance**

- `docs/plans/v0-3-naming-hard-cut-execution.md` includes the target layout as an explicit execution phase artifact.
- A concrete incremental sequence exists for layout convergence, with clear boundaries and non-goals.
- No contradiction between naming hard-cut contracts and proposed directory-boundary migration.

**Status:** `done`

---

## Phase 8 -- Repository layout implementation

**Goal.** Implement physical repository layout convergence slices so directory structure starts matching the directional target, with directory scaffold creation gated until binary and path convergence work is complete.

**Deliverables**

- Execute one or more approved physical move slices from Phase 7 (`S5`) with explicit scope boundaries.
- Update all affected surfaces for each executed slice:
  - Go package/import paths
  - `Makefile` package path variables and targets
  - shell script path references
  - `.goreleaser.yaml` and packaging path inputs
  - CI workflow path assumptions
- Keep runtime behavior unchanged while moving paths (path move PRs should be mechanical; behavior changes are separate).
- Record move ledger in this phase:
  - moved paths and ownership area
  - compatibility shims (if any)
  - follow-up cleanup required

**Implementation notes**

- Executed physical convergence slice **S5-1 (indexer entrypoint path cutover)**:
  - moved `cmd/chimera-index/main.go` -> `porcelain/chimera/chimera-indexer/main.go`
  - removed old `cmd/chimera-index/main.go`
- Executed physical convergence slice **S5-2 (indexer binary name + runtime default cutover)**:
  - canonical build output renamed from `chimera-index[.exe]` -> `chimera-indexer[.exe]`
  - supervisor default indexer binary resolution now uses canonical `chimera-indexer` only (no legacy fallback)
- Updated path consumers for the moved slice:
  - `Makefile`: `CHIMERA_CMD_INDEXER := ./porcelain/chimera/chimera-indexer`
  - `Makefile`: `CHIMERA_INDEX_BIN := chimera-indexer`
  - `scripts/chimera-names.sh`: `CHIMERA_CMD_INDEXER=porcelain/chimera/chimera-indexer`
  - `scripts/chimera-names.sh`: `CHIMERA_INDEX_BIN_BASE=chimera-indexer`
  - `internal/indexer/indexer_test.go` comment reference updated to `porcelain/chimera/chimera-indexer`
  - `.cursor/rules/post-change-make-targets.mdc` updated to `porcelain/chimera/chimera-indexer`
  - `cmd/chimera/serve_defaults.go`: default supervised indexer binary changed to `chimera-indexer` first
  - `cmd/chimera-supervisor/main.go`: default supervised indexer binary changed to `chimera-indexer` first
  - `.gitignore`: root artifact ignore updated to `/chimera-indexer*`
- Compatibility shim policy for this slice:
  - no duplicate entrypoint directory kept; hard cut to canonical path
  - binary name hard cut to canonical `chimera-indexer`; legacy runtime lookup support is removed
- Follow-up cleanup completed:
  - updated `docs/plans/v0-3-naming-phase1-inventory.md` references from `cmd/chimera-index` to `porcelain/chimera/chimera-indexer`.
- Runtime fix bundled with this slice:
  - supervisor now resolves indexer token from `CHIMERA_GATEWAY_TOKEN` env key via naming contract and can pass token to supervised indexer child startup.

**Validation run**

- `go build ./porcelain/chimera/chimera-indexer`
- `make chimera-indexer-build`
- `go test ./internal/indexer -count=1`
- `go test ./internal/supervisor -count=1`
- `go build ./cmd/chimera-supervisor`
- `make chimera-indexer-build`
- `go build ./cmd/chimera`
- `make chimera-build`

### Slice S5-4 execution log (legacy-path and legacy-name audit closure)

Audit commands run:

- `rg "cmd/chimera-index\\b"`
- `rg "\\bchimera-index\\b"`
- `rg "cmd/chimera\\b|cmd/chimera-indexer\\b"`

Classification of remaining hits:

- **Historical/archival and intentionally retained**
  - `cmd/chimera-index` command-path hits are in historical plan ledger context in this file.
  - `cmd/chimera*` command-path hits are in historical/archive docs/plans and earlier version notes.
- **Updated in this phase (S5-3 consistency pass complete)**
  - Runtime/operator docs and config examples now use canonical `chimera-indexer` naming.
  - Code comments/help/version strings in active indexer/supervisor surfaces now use canonical `chimera-indexer` naming.
- **Hard-cut policy enforcement**
  - No legacy runtime lookup fallback is allowed on active paths.
  - Active binaries and launch resolution must use canonical names only:
    - `chimera-gateway`
    - `chimera-supervisor`
    - `chimera-indexer`
    - `locus-desktop`

### Slice S5-5 execution log (directory scaffold bootstrap)

Created scaffold directories in required order:

1. Parent namespace:
   - `porcelain/chimera/`
2. Binary/domain ownership subdirectories:
   - `porcelain/chimera/chimera-supervisor/`
   - `porcelain/chimera/chimera-gateway/`
   - `porcelain/chimera/chimera-indexer/`
   - `porcelain/chimera/chimera-bifrost/`
   - `porcelain/chimera/chimera-vectorstore/`
3. Shared boundary:
   - `porcelain/chimera/inernal/`

Shared code/packages/namespaces initial classification baseline:

- **Binary-owned candidates**
  - `cmd/chimera-supervisor`, `internal/supervisor`
  - `cmd/chimera-gateway`, `internal/server` (gateway entry/runtime wiring)
  - `porcelain/chimera/chimera-indexer`, `internal/indexer`
- **Domain-owned candidates**
  - `internal/bifrostadmin` -> `chimera-bifrost` domain boundary candidate
  - `internal/vectorstore` -> `chimera-vectorstore` domain boundary candidate
- **Cross-binary shared candidates**
  - `internal/config`, `internal/naming`, `internal/tokens`, `internal/conversationmerge`, `internal/servicelogs`

### Phase 8 closeout notes

This phase is complete. Physical convergence slices were executed, active-surface naming/path consistency is canonical, and validation/audit gates pass on current HEAD.

#### Slice S5-3 — Canonical indexer identifier completion (`chimera-indexer`)

**Objective:** finish the post-move consistency pass after `porcelain/chimera/chimera-indexer` and `chimera-indexer[.exe]` cutover.

**Required updates (active surfaces first)**

1. **Runtime/operator docs (active path)**
   - `README.md`
   - `docs/configuration.md`
   - `docs/supervisor.md`
   - `docs/indexer.md`
   - `docs/README.md`
   - `env.example`
   - `config/gateway.example.yaml`
   - `config/indexer.example.yaml`
2. **Version/plan docs that define canonical naming contracts**
   - `docs/version-v0.3.md`
   - `docs/plans/v0-3-naming-hard-cut-execution.md` (this file; remove stale “follow-up required” wording once complete)
3. **Code comments/help strings tied to active behavior**
   - `internal/supervisor/indexer.go`
   - `internal/indexer/config.go`
   - `internal/indexer/log_level.go`
   - `internal/indexer/supervised_file.go`
   - `porcelain/chimera/chimera-indexer/main.go` (usage/version/help text consistency review)

**Compatibility policy**

- Do not keep binary lookup fallback (`chimera-index`) in supervisor/default launchers; active runtime must resolve canonical `chimera-indexer` only.
- Do not change runtime behavior in this slice beyond naming/path consistency.

#### Slice S5-4 — Legacy-path and legacy-name audit closure

**Objective:** close Phase 8 with a reproducible audit showing no unintended hard-coded legacy paths on active surfaces.

**Audit commands (record outputs in this phase)**

- Search for legacy command path:
  - `rg "cmd/chimera-index\\b"`
- Search for legacy indexer binary label where canonical should now be `chimera-indexer`:
  - `rg "\\bchimera-index\\b"`
- Search for legacy chimera cmd-path references on active surfaces:
  - `rg "cmd/chimera\\b|cmd/chimera-indexer\\b"`

For each remaining hit: mark as either

- **updated in this phase**, or
- **historical/archival and intentionally retained** (with explicit justification).

#### Validation gate for Phase 8 completion

Before setting Phase 8 to `done`, run and record:

- `make chimera-indexer-build`
- `go build ./cmd/chimera-supervisor`
- `go build ./cmd/chimera`
- `go test ./internal/supervisor -count=1`
- `go test ./internal/indexer -count=1`
- `make chimera-build`

Final validation re-run after hard-cut fallback removal:

- `make chimera-indexer-build` (pass)
- `go build ./cmd/chimera-supervisor` (pass)
- `go build ./cmd/chimera` (pass)
- `go test ./internal/supervisor -count=1` (pass)
- `go test ./internal/indexer -count=1` (pass)
- `make chimera-build` (pass)

#### Slice S5-5 -- Directory scaffold bootstrap (gated after S5-3/S5-4)

**Objective:** start materializing the target layout only after binary identity and path consistency are fully resolved.

**Prerequisites (must all be true before creating directories):**

1. Slice S5-3 is complete and canonical binary naming is stable across active surfaces (`chimera-gateway`, `chimera-supervisor`, `chimera-indexer`, `locus-desktop`).
2. Slice S5-4 audit is complete with remaining legacy path/name hits either updated or explicitly marked historical.
3. Validation gate commands above pass on current HEAD.

**Execution order once prerequisites are met:**

1. Create parent namespace directory first:
   - `porcelain/chimera/`
2. Create binary/ownership subdirectories under that parent:
   - `porcelain/chimera/chimera-supervisor/`
   - `porcelain/chimera/chimera-gateway/`
   - `porcelain/chimera/chimera-indexer/`
   - `porcelain/chimera/chimera-bifrost/`
   - `porcelain/chimera/chimera-vectorstore/`
3. Create shared boundary directory:
   - `porcelain/chimera/internal/`
4. Create remaining top-level directional namespaces:
   - `porcelain/locus/`
   - `porcelain/locus/locus-desktop/`
   - `porcelain/porcelain/`
   - `porcelain/porcelain/porcelain-config/`
   - `porcelain/porcelain/porcelain-meta/`
   - `porcelain/porcelain/porcelain-docs/`

**Shared code/packages/namespaces identification pass (required during S5-5):**

- For each moved or candidate package, classify as one of:
  - binary-owned (`chimera-supervisor`, `chimera-gateway`, `chimera-indexer`),
  - domain-owned (`chimera-bifrost`, `chimera-vectorstore`), or
  - cross-binary internal (`internal`).
- Record classification rationale in this phase ledger so future moves remain mechanical and consistent.
- Avoid mixing behavior changes with namespace/path moves; re-home code first, then do behavior deltas in separate slices.

Then update this section with:

- files changed by S5-3/S5-4,
- files changed by S5-5 scaffold/bootstrap work,
- unresolved deferred items (if any),
- rationale for any intentionally retained historical references.

Current ledger snapshot:

- files changed by S5-3/S5-4:
  - previously recorded in this phase (`porcelain/chimera/chimera-indexer`, `Makefile`, `scripts/chimera-names.sh`, supervisor defaults, docs/plans updates), plus this S5-4 audit record update.
- files changed by S5-5 scaffold/bootstrap work:
  - `porcelain/chimera/`
  - `porcelain/chimera/chimera-supervisor/`
  - `porcelain/chimera/chimera-gateway/`
  - `porcelain/chimera/chimera-indexer/`
  - `porcelain/chimera/chimera-bifrost/`
  - `porcelain/chimera/chimera-vectorstore/`
  - `porcelain/chimera/internal/`
  - `porcelain/locus/`
  - `porcelain/locus/locus-desktop/`
  - `porcelain/porcelain/`
  - `porcelain/porcelain/porcelain-config/`
  - `porcelain/porcelain/porcelain-meta/`
  - `porcelain/porcelain/porcelain-docs/`
- unresolved deferred items:
  - none.
- intentionally retained historical references rationale:
  - plan/version historical narrative and archival docs retain old command-path examples for historical traceability.
  - no active runtime legacy naming support is retained; binaries and launch resolution are canonical-only.

Final S5-4 audit outcome (active surfaces):

- `rg "cmd/chimera-index\\b"`: historical plan-ledger references only.
- `rg "\\bchimera-index\\b"`: historical docs/plans references only; active runtime launch resolution fallback removed.
- `rg "cmd/chimera\\b|cmd/chimera-indexer\\b"`: historical/archive docs/plans references only (outside active runtime/operator contract surfaces).

**Acceptance**

- At least one physical directory/path convergence slice is completed and merged.
- Build/run/test/package flows pass after path updates.
- No unresolved hard-coded path assumptions remain in touched surfaces.

**Status:** `done`

---

## Phase 9 -- Validation and closeout

**Goal.** Prove end-to-end consistency across build, runtime, tests, packaging, and operator docs.

**Deliverables**

- Run and record validation for changed surfaces:
  - Build targets (gateway/supervisor/indexer/desktop as applicable)
  - Unit/integration tests affected by rename
  - Packaging/release-snapshot checks for canonical artifact names
- Execute final naming audit on active source/docs/tests/scripts.
- Record formal handoff gate to wrapper plan:
  - mark [`vectorstore-broker-wrapper-hard-cut.md`](vectorstore-broker-wrapper-hard-cut.md) Phase 2+ as unblocked once this phase passes.
- Update plan status table and closeout notes with any deferred follow-ups.

**Acceptance**

- Build/test/package paths pass with canonical names only.
- Final audit finds no unintended legacy naming in active surfaces.
- Wrapper-plan execution gate is explicitly cleared in closeout notes (Phase 1 allowed in parallel; Phases 2-6 unblocked after this phase).
- Plan is ready for execution handoff or closure update.

**Status:** `todo`

---

## Open questions

None pending for Phase 1 baseline. The previous two decisions are resolved in this plan:

1. Canonical gateway naming is fully split to `chimera-gateway-*`.
2. Retired legacy files are deleted in this train.

---

## References

- Runtime/config: `cmd/chimera/`, `porcelain/chimera/chimera-indexer/`, `internal/config/`, `internal/indexer/`, `internal/server/`, `internal/naming/contracts.go`
- Build/scripts/release: `Makefile`, `scripts/chimera-names.sh`, `scripts/print-make-help.sh`, `scripts/release-package.sh`, `.goreleaser.yaml`, `.github/workflows/go.yml`
- Operator-facing docs: `README.md`, `docs/configuration.md`, `docs/supervisor.md`, `docs/indexer.md`, `docs/installation.md`, `docs/packaging.md`, `docs/migration-v0-3-naming.md`, `docs/version-v0.3.md`
- Source of migration contract: `docs/plans/v0-3-naming-phase1-inventory.md`
