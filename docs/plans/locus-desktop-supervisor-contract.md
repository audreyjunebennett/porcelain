# Plan: Locus desktop supervisor contract

| Field | Value |
|-------|-------|
| **Doc kind** | `refactor-plan` |
| **Owners / areas** | Desktop launcher, supervisor runtime, packaging, docs |
| **Status** | `done` |
| **Targets** | v0.4 desktop/supervisor boundary |
| **Last updated** | See git history |
| **Supersedes / superseded by** | None |

## At a glance

This plan defines a stable contract between `locus-desktop` and `chimera-supervisor` so desktop UI behavior no longer depends on embedded supervisor internals. The goal is to make desktop and supervisor independently buildable, testable, and releasable while keeping startup, health, and failure behavior predictable for operators. The contract covers process ownership, readiness handshake, shutdown policy, version compatibility, and packaging/runtime path rules.

| Phase | Outcome | Status |
|-------|---------|--------|
| [Phase 1 — Contract lock](#phase-1--contract-lock) | One canonical desktop-to-supervisor contract is defined and frozen for implementation | `done` |
| [Phase 2 — External supervisor launch path](#phase-2--external-supervisor-launch-path) | `locus-desktop` launches/attaches to `chimera-supervisor` via explicit process contract | `done` |
| [Phase 3 — Handshake and lifecycle behavior](#phase-3--handshake-and-lifecycle-behavior) | Desktop startup, readiness, degradation, and shutdown behavior are contract-driven and deterministic | `done` |
| [Phase 4 — Packaging and runtime layout](#phase-4--packaging-and-runtime-layout) | Desktop bundles and install flows enforce binary location and startup assumptions across platforms | `todo` |
| [Phase 5 — Validation and migration closeout](#phase-5--validation-and-migration-closeout) | Contract is validated by tests/docs and legacy embedded assumptions are retired | `done` |

---

## Background

The current desktop path still performs supervisor-style startup work in the same runtime path, which blurs ownership between UI shell and service orchestration. Naming and build targets already distinguish `locus-desktop` and `chimera-supervisor`, but runtime boundaries are not yet fully enforced as a contract. This plan formalizes the split so desktop can remain a client of supervisor instead of carrying orchestration logic.

**Related docs:** [`v0-3-naming-migration.md`](v0-3-naming-migration.md), [`vectorstore-broker-wrapper-hard-cut.md`](vectorstore-broker-wrapper-hard-cut.md), [`desktop-ui.md`](desktop-ui.md), [`log-supervisor-normalization-fidelity.md`](log-supervisor-normalization-fidelity.md), [`../supervisor.md`](../supervisor.md), [`../packaging.md`](../packaging.md).

---

## Phase 1 — Contract lock

**Goal.** Define and freeze one operator-facing contract for how `locus-desktop` starts, connects to, monitors, and optionally stops `chimera-supervisor`.

**Deliverables**

- Define process ownership contract:
  - `locus-desktop` owns native window lifecycle and user-facing startup UX.
  - `chimera-supervisor` owns gateway and managed service lifecycle.
- Define launch contract:
  - binary discovery order (bundle path, sibling path, PATH fallback policy).
  - desktop first attempts to connect to an existing local supervisor endpoint; if unavailable, desktop starts supervisor.
  - allowed CLI/env surface passed from desktop to supervisor is pass-through (no curated allowlist in v1).
  - working directory/runtime directory expectations.
- Define handshake contract:
  - required readiness endpoints and status semantics used by desktop startup flow.
  - startup timeout, retry/backoff, and fail-fast thresholds.
- Define shutdown contract:
  - attach mode is read-only for process ownership and lifecycle control.
  - if desktop attached to an existing supervisor, desktop close does not stop supervisor.
  - if desktop started supervisor, desktop close stops that owned supervisor by default.
- Define compatibility contract:
  - strict version gating is required for incompatible supervisor versions.
  - version checks should avoid churn from commit SHA-only differences; compatibility should key off stable version contract fields rather than build SHA.
  - incompatibility behavior and user-facing message requirements.
- Define security contract:
  - local supervisor targets only in v1.
  - remote supervisor targets are out of scope.
  - token/session handling boundaries between desktop shell and supervisor HTTP surface.

**Acceptance**

- A single contract section exists in docs and is referenced by desktop/supervisor implementation work.
- No ambiguity remains about ownership, startup readiness, shutdown behavior, or version checks.
- Contract language is operator-focused and avoids implementation-specific assumptions.

### Frozen v1 contract (locked)

This section is the implementation contract for Phase 2+ work. Changes to these rules require an explicit plan update.

- **Startup mode and ownership**
  - `locus-desktop` is a launcher/client for `chimera-supervisor`; it is not the owner of service orchestration internals.
  - Desktop startup is `connect-first`: attempt to connect to a local running supervisor first.
  - If no reachable local supervisor is found, desktop starts supervisor.
  - Remote supervisor targets are out of scope in v1.

- **Lifecycle control**
  - Attach mode is read-only for process ownership and lifecycle controls.
  - If desktop attached to an existing supervisor, desktop close does not stop supervisor.
  - If desktop started supervisor, desktop close stops that owned supervisor by default.

- **Supervisor launch surface**
  - Desktop uses pass-through startup flags/environment for supervisor in v1.
  - No curated launcher-only allowlist is required in v1.
  - Invalid combinations still fail fast with clear user-facing error messaging.

- **Readiness and connection contract**
  - Desktop connection flow relies on supervisor HTTP health/readiness/status contract endpoints.
  - Desktop opens a dedicated "cannot connect to supervisor" state/page when connect/start flow fails.
  - Failure messaging must include actionable remediation (for example: missing binary, version mismatch, port conflict, startup timeout).

- **Version compatibility contract**
  - Compatibility enforcement is strict version gating for incompatible supervisor versions.
  - Compatibility checks must be stable across local rebuilds and must not churn on commit SHA-only differences.
  - Build metadata can be shown for diagnostics but is not the primary compatibility key.

- **Runtime directory contract (desktop bundle launch)**
  - Desktop binary output path is `porcelain/locus/bin`.
  - On double-click launch, desktop resolves runtime root as parent of the binary directory.
  - Runtime-root-relative directories are canonical for `config`, `data`, `logs`, and `run`.
  - PID/lock files are stored under runtime-root `run`.

- **Supervisor log mirror (`data/locus-desktop-supervisor.log`)**
  - When desktop starts supervisor, child stdout/stderr are tee'd to this append-only file (see `locus/locus-desktop/internal/launcher/launcher.go`).
  - Each line is **normalized JSON** (`_chimera_norm: 1`) produced by per-service `*line` normalizers and a **lossless reorder** on supervisor ingest ([`log-supervisor-normalization-fidelity.md`](log-supervisor-normalization-fidelity.md)).
  - Structured fields (`progress_detail`, `method`, `path`, `collection`, indexer `rel`, `queue_depth`, etc.) must survive the wrapper → supervisor double-normalize path; bare `msg`-only rows indicate a regression.
  - The gateway logs UI reads the same normalized lines from the supervisor log buffer via HTTP (`supervisorlogs`), not a separate richer format.

**Status:** `done`

---

## Phase 2 — External supervisor launch path

**Goal.** Move desktop runtime to a true client-launcher model where `locus-desktop` invokes `chimera-supervisor` as an external dependency.

**Deliverables**

- Enforce location/build contract before additional Phase 2 implementation:
  - desktop launcher/source code lives under `porcelain/locus/locus-desktop`.
  - desktop binary output path is `porcelain/locus/bin`.
  - Phase 2 work should not continue with desktop launcher code rooted under legacy `porcelain/chimera/*` paths.
- Implement desktop launcher mode that:
  - resolves supervisor binary path from the contract.
  - starts supervisor process with contract-approved args/env.
  - records launch metadata for diagnostics (without secret values).
- Add attach-first behavior for already-running local supervisor:
  - attempt connect first.
  - start supervisor only if connect fails.
- Add single-instance coordination policy that prevents accidental multi-instance supervisor starts from desktop.
- Keep webview/UI startup independent from supervisor internals except through contract-defined endpoints and state.
- Update build/run target docs to clarify launcher vs supervisor direct-run responsibilities.

**Acceptance**

- Desktop can launch and connect to supervisor without relying on embedded supervisor startup internals.
- Startup behavior is deterministic: connect-first, start-if-missing.
- Failure states in launch path are surfaced with clear, actionable operator messages.

**Implementation note (current)**

- Before continuing Phase 2 implementation, move desktop launcher implementation and tests to `porcelain/locus/locus-desktop`.
- Build outputs for desktop artifacts must land in `porcelain/locus/bin`.
- After relocation, continue/confirm behavior:
  - connect-first to existing local supervisor endpoint.
  - start `chimera-supervisor --headless` only when attach fails.
  - track ownership and stop only desktop-owned supervisor on desktop close.
  - pass through launch args/env into supervisor process.

**Status:** `done`

---

## Phase 3 — Handshake and lifecycle behavior

**Goal.** Ensure desktop startup and runtime behavior is entirely driven by contract health/readiness/status rules.

**Deliverables**

- Implement startup state machine for desktop:
  - launch/attach, wait for liveness, wait for readiness, open login/setup/panel route.
- Implement degraded/unavailable supervisor handling:
  - unreachable, startup-timeout, runtime-loss, and partial-readiness scenarios.
- Define and implement restart strategy boundaries:
  - what desktop retries itself.
  - what remains supervisor responsibility.
- Implement controlled shutdown semantics:
  - close window after attach path keeps supervisor running.
  - close window after launch-owned path stops supervisor.
- Keep attach mode read-only for lifecycle controls (no stop/restart operations against externally-owned supervisor).
- Define observable events for lifecycle transitions to support supportability and tests.
- Add required unreachable-state UX:
  - desktop opens and shows a dedicated "cannot connect to supervisor" page when connect/startup flow fails.

**Acceptance**

- Desktop behavior for ready, not-ready, degraded, and disconnected states matches the contract matrix.
- Closing desktop follows ownership rules and does not stop externally-owned supervisors.
- Lifecycle events are testable and diagnosable with stable message taxonomy.

**Status:** `done`

---

## Phase 4 — Packaging and runtime layout

**Goal.** Make desktop packaging/install flows enforce the supervisor dependency contract consistently on each platform.

**Deliverables**

- Define canonical bundle/runtime layout for:
  - desktop binary output path under `porcelain/locus/bin`.
  - when launched by double-click, desktop resolves runtime root as the parent directory of the binary directory.
  - `config`, `data`, `logs`, and `run` paths are resolved from that runtime root.
  - PID/lock state is stored under runtime-root `run`.
- Update packaging scripts and install docs to reflect the launcher contract and path resolution behavior.
- Define platform-specific path and permission constraints (Windows/macOS/Linux) for launching supervisor from desktop bundle locations.
- Define integrity checks for missing or incompatible supervisor artifact at launch time.
- Add migration notes for operators moving from embedded/combined behavior to launcher model.

**Acceptance**

- Packaged desktop artifacts include or reliably resolve the supervisor binary per contract.
- Installer and runbook docs match actual runtime behavior.
- Missing/incompatible artifacts fail fast with clear remediation guidance.
- Double-click launch path uses runtime-root-relative config/data/log/run directories predictably.

**Status:** `done`

---

## Phase 5 — Validation and migration closeout

**Goal.** Validate the contract end-to-end and close migration from implicit embedded behavior.

**Deliverables**

- Add contract-oriented test matrix:
  - launch success
  - attach success
  - readiness timeout
  - degraded runtime
  - controlled shutdown variants (attach-owned vs launch-owned supervisor)
  - version/capability mismatch
  - missing binary/path mismatch
  - unreachable fallback page visibility and remediation actions
- Add cross-platform smoke validation checklist and deterministic make targets.
- Update operator docs (`README`, supervisor, installation, packaging, desktop plan docs) with final launcher contract behavior.
- Remove or deprecate stale docs/messages that imply desktop owns service orchestration internals.

**Acceptance**

- Contract matrix tests pass for desktop-supervisor handshake and lifecycle semantics.
- Operator docs describe one consistent model: desktop client + external supervisor runtime.
- Migration can be considered complete with no unresolved contract blockers.

**Status:** `done`

---

## Resolved decisions

1. Desktop launch policy: connect to an existing local supervisor first; start supervisor only when no local instance is reachable.
2. Close behavior: if desktop attached to an existing supervisor, do not stop it; if desktop started supervisor, stop it on desktop close.
3. Version policy: strict version gating, but compatibility checks should not churn on commit SHA-only build differences.
4. Launch surface: supervisor startup flags/env are pass-through from desktop in v1.
5. Attach-mode ownership: read-only lifecycle control (no stop/restart of externally-owned supervisor).
6. Runtime directory contract: desktop binary writes to `porcelain/locus/bin`; on double-click launch, runtime paths resolve from the parent directory (`config`, `data`, `logs`, `run`).
7. Unreachable UX: desktop must open and show a "cannot connect to supervisor" page when connect/start fails.
8. Scope boundary: remote supervisor targets are out of scope for v1 (local-only).

---

## References

- Code: `porcelain/locus/locus-desktop`, `cmd/chimera-supervisor`, `internal/supervisor`, `internal/server`, `porcelain/Makefile`, `porcelain/scripts/desktop-run.sh`, `porcelain/locus/bin`
- Docs: [`v0-3-naming-migration.md`](v0-3-naming-migration.md), [`vectorstore-broker-wrapper-hard-cut.md`](vectorstore-broker-wrapper-hard-cut.md), [`desktop-ui.md`](desktop-ui.md), [`../supervisor.md`](../supervisor.md), [`../packaging.md`](../packaging.md), [`../installation.md`](../installation.md)
