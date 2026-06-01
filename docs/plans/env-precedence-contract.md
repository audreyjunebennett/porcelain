# Plan: Unified environment precedence contract

| Field                          | Value                                                |
|--------------------------------|------------------------------------------------------|
| **Doc kind**                   | `feature-plan`                                       |
| **Owners / areas**             | Gateway, supervisor, indexer, desktop launcher, docs |
| **Status**                     | `draft`                                              |
| **Targets**                    | v0.4 runtime contract consistency                    |
| **Last updated**               | See git history                                      |
| **Supersedes / superseded by** | None                                                 |

## At a glance

This plan defines one operator-facing environment/config precedence contract shared by all first-party binaries so startup behavior is predictable. It introduces explicit runtime profiles (secure, team dev, personal/private), including a personal/private flow where the product can generate a gateway key, persist it, and log where it was written without ever logging the key value. The contract is designed to become a reusable startup requirement for portable service wrappers in [`vectorstore-broker-wrapper-hard-cut.md`](vectorstore-broker-wrapper-hard-cut.md).

| Phase                                                                                            | Outcome                                                                                   | Status |
|--------------------------------------------------------------------------------------------------|-------------------------------------------------------------------------------------------|--------|
| [Phase 1 — Contract definition](#phase-1--contract-definition)                                   | One canonical precedence and profile contract is published for operators and implementers | `todo` |
| [Phase 2 — Single-binary implementation](#phase-2--single-binary-implementation)                 | One binary implements the contract end-to-end as reference behavior                       | `todo` |
| [Phase 3 — Contract-based validation](#phase-3--contract-based-validation)                       | Tests validate the contract itself (behavioral outcomes), not implementation details      | `todo` |
| [Phase 4 — Remediation and portability refactor](#phase-4--remediation-and-portability-refactor) | Gaps are fixed and startup logic is refactored into portable components                   | `todo` |
| [Phase 5 — Rollout to remaining binaries](#phase-5--rollout-to-remaining-binaries)               | Remaining binaries adopt and verify the same contract and profile behavior                | `todo` |

---

## Background

Current binaries are close but not fully uniform in how environment, dotenv, config files, and flags interact. That leads to surprises for operators, especially when launching desktop/supervised flows. A single contract lowers support burden, improves security posture, and creates a reliable startup foundation for wrapper-style portable binaries.

**Related docs:** [`vectorstore-broker-wrapper-hard-cut.md`](vectorstore-broker-wrapper-hard-cut.md), [`v0-3-naming-migration.md`](v0-3-naming-migration.md), [`../configuration.md`](../configuration.md), [`../supervisor.md`](../supervisor.md), [`../installation.md`](../installation.md).

---

## Phase 1 — Contract definition

**Goal.** Publish one explicit, cross-binary precedence and profile contract that operators can rely on.

**Deliverables**

- Define canonical precedence order for non-secret settings:
  1. CLI flags (non-secret only)
  2. Process environment
  3. Service dotenv (optional; profile-controlled)
  4. Repo dotenv (optional; profile-controlled)
  5. YAML/config files
  6. Built-in defaults
- Define secret handling contract:
  - secrets never accepted via CLI flags
  - secrets may come from process env and/or approved secret file paths
  - all secret-like values redacted in logs
- Define profile matrix:
  - **secure** (default for production): dotenv disabled unless explicitly enabled
  - **team_dev**: dotenv enabled, process env still wins
  - **personal_private**: dotenv enabled; optional product-managed key bootstrap behavior
- Define personal/private auto-key behavior:
  - if required key missing, generate key and persist to configured key store path
  - log that a key was created and the path used
  - never log key material
- Define startup observability contract:
  - each critical input logs source metadata (`flag|env|dotenv_service|dotenv_repo|yaml|default|generated`)
  - source log includes key name and source class, not sensitive value
- Define wrapper portability requirement:
  - wrapper startup contracts must consume this same precedence/profile model as a prerequisite for service portability.

**Acceptance**

- A single contract section exists and is referenced by gateway/supervisor/indexer/desktop docs.
- Profile behavior and secret rules are unambiguous, including generated-key logging rules.
- Wrapper plan references this contract as required startup behavior for packaged services.

**Status:** `todo`

---

## Phase 2 — Single-binary implementation

**Goal.** Implement the full contract in one binary as the reference implementation.

**Deliverables**

- Choose one first implementation target binary (recommended: `chimera-supervisor` because it launches children).
- Implement profile selection and precedence resolver in that binary.
- Implement secure secret handling and redaction in startup logs.
- Implement personal/private auto-key generation path:
  - deterministic storage path resolution
  - atomic write behavior
  - creation log with path only (no value leak)
- Emit startup source-trace logs for critical config/env inputs.
- Update operator docs for the chosen binary with exact profile and precedence behavior.

**Acceptance**

- Selected binary behavior matches Phase 1 contract across all precedence/profile combinations.
- Generated-key flow logs creation and path but never logs the secret itself.
- Existing startup behavior remains backward-compatible where contract says it should.

**Status:** `todo`

---

## Phase 3 — Contract-based validation

**Goal.** Validate contract behavior as black-box outcomes, independent of implementation internals.

**Deliverables**

- Add behavioral test matrix that exercises precedence and profile combinations:
  - conflicting values across flag/env/dotenv/yaml/default
  - secret-source enforcement
  - profile-specific dotenv enablement
- Add personal/private key-generation tests:
  - missing key triggers generation
  - persisted location is correct
  - logs include creation event and path
  - logs never include secret value
- Add regression tests for source-trace logging metadata and redaction.
- Define contract fixtures reusable by remaining binaries in Phase 5.

**Acceptance**

- Tests assert contract outcomes (effective values, source metadata, redaction), not code structure.
- Test suite fails when precedence/profile contract changes unexpectedly.
- Contract fixture set is reusable across binaries.

**Status:** `todo`

---

## Phase 4 — Remediation and portability refactor

**Goal.** Fix issues from contract testing and refactor startup logic into portable components for future wrapper-packaged services.

**Deliverables**

- Resolve all Phase 3 contract mismatches.
- Refactor precedence/profile resolution into shared reusable package(s) for startup portability.
- Define integration seam for wrapper binaries and future service-packaging runners so they can adopt this contract uniformly.
- Add portability-focused docs:
  - how packaged services consume precedence resolver
  - how profiles map to local/desktop/headless/service modes
- Record non-goals and deferred enhancements for post-v0.4 portability work.

**Acceptance**

- Reference binary passes contract suite after remediation.
- Shared startup components are suitable for wrapper/service packaging contexts.
- No contract divergence introduced between direct binaries and portable wrapper startup.

**Status:** `todo`

---

## Phase 5 — Rollout to remaining binaries

**Goal.** Apply and verify the same precedence/profile contract across all remaining first-party binaries.

**Deliverables**

- Implement shared precedence/profile resolver in remaining binaries:
  - `chimera-gateway`
  - `chimera-supervisor` or `chimera-indexer` (whichever was not Phase 2)
  - `chimera-indexer`
  - desktop launch path (`cmd/chimera`/desktop mode) as applicable
- Run the same contract fixture suite per binary with profile variants.
- Normalize operator docs so precedence/profile behavior is described once and referenced consistently.
- Add migration notes for operators moving from previous implicit precedence behavior.

**Acceptance**

- All target binaries pass the same contract suite.
- Operator documentation shows one consistent contract across binaries.
- Personal/private key-generation behavior is consistent where enabled and remains non-leaky in logs.

**Status:** `todo`

---

## Open questions

1. Which binary should be the Phase 2 reference implementation (`chimera-supervisor` recommended)?
2. Should personal/private auto-generation be enabled only in `personal_private` profile, or also as an explicit opt-in flag for other profiles?
3. What is the canonical persisted key store path when auto-generation is enabled for personal/private mode?

---

## References

- Code: `cmd/chimera-gateway`, `cmd/chimera-supervisor`, `porcelain/chimera/chimera-indexer`, `cmd/chimera`, `internal/config`, `internal/indexer`, `internal/supervisor`, `internal/tokens`
- Docs: [`vectorstore-broker-wrapper-hard-cut.md`](vectorstore-broker-wrapper-hard-cut.md), [`v0-3-naming-migration.md`](v0-3-naming-migration.md), [`../configuration.md`](../configuration.md), [`../supervisor.md`](../supervisor.md)
