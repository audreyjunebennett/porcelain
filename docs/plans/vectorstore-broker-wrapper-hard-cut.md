# Plan: Vectorstore and broker wrapper hard cut

| Field | Value |
|-------|-------|
| **Doc kind** | `refactor-plan` |
| **Owners / areas** | Supervisor, wrappers, logs UI, docs, build/scripts |
| **Status** | `done` |
| **Targets** | gateway v0.4 naming and supervision cutover |
| **Last updated** | See git history |
| **Supersedes / superseded by** | None |
| **As-built** | [`docs/features/chimera-wrapper-binary-contract.md`](../features/chimera-wrapper-binary-contract.md), [`product-naming-contract.md`](../features/product-naming-contract.md), [`structured-operator-log-lines.md`](../features/structured-operator-log-lines.md) |

**Behavioral source of truth:** the [wrapper binary contract](../features/chimera-wrapper-binary-contract.md) describes as-built behavior; this plan is delivery history.

## At a glance

This plan hard-cuts operator-facing naming from upstream product names to Chimera wrapper names. The supervisor will manage standalone `chimera-vectorstore` and `chimera-broker` binaries, while wrappers own translation into upstream-specific flags, env, lifecycle, and debug visibility. v1 scope is binary-only with a pluggable driver interface designed from day one, so container/remote drivers can be added later without changing operator-facing contracts. Operators see Chimera names across UI, docs, logs, and CLI; upstream names are implementation details shown only in architecture/debug/error contexts.

Execution sequencing relative to v0.3 product naming:

- Prerequisite [`v0-3-naming-migration.md`](v0-3-naming-migration.md) (hard-cut naming closeout) is **done** — Phases 2–7 below are unblocked.
- Phase 1 of this plan (contract lock) may proceed in parallel with naming closeout; later phases assume naming contracts are stable.

| Step / phase | Outcome | Status |
|--------------|---------|--------|
| [Prerequisite — v0.3 product naming migration](v0-3-naming-migration.md) | Hard-cut naming contracts closed per [version-v0.3.md](../version-v0.3.md#product-naming); unblocks wrapper implementation | `done` |
| [Phase 1 — Wrapper contract lock](#phase-1--wrapper-contract-lock) | Stable wrapper CLI, health, logging, and lifecycle contracts are frozen | `done` |
| [Phase 2 — Standalone wrapper binaries](#phase-2--standalone-wrapper-binaries) | `chimera-vectorstore` and `chimera-broker` run as first-class managed processes | `done` |
| [Phase 3 — Wrapper E2E contract matrix](#phase-3--wrapper-e2e-contract-matrix) | Executable end-to-end tests validate wrapper process lifecycle and all contract endpoints | `done` |
| [Phase 4 — Supervisor cutover](#phase-4--supervisor-cutover) | Supervisor speaks only Chimera wrapper contracts and no longer manages upstream binaries directly | `done` |
| [Phase 5 — Logs and UI naming hard cut](#phase-5--logs-and-ui-naming-hard-cut) | Operator logs/cards/chips use Chimera names with optional upstream debug drill-down | `done` |
| [Phase 6 — Make, scripts, and packaging alignment](#phase-6--make-scripts-and-packaging-alignment) | Build/run/install/package flows consistently manage wrapper binaries | `done` |
| [Phase 7 — Documentation and migration closeout](#phase-7--documentation-and-migration-closeout) | Operator docs are Chimera-only and migration validation is complete | `done` |

---

## Background

The current stack directly exposes `qdrant` and `bifrost` names in supervision, UI, logs, scripts, and docs. That makes product messaging inconsistent and increases migration cost when backend components change. The chosen direction is a hard cut now: wrappers become the public operational surface, with upstreams treated as implementation details. Design assumes backend swaps in the near future, but implementation scope for milestone one is binary mode only.

**Related docs:** [`v0-3-naming-migration.md`](v0-3-naming-migration.md), [`../supervisor.md`](../supervisor.md), [`../configuration.md`](../configuration.md), [`../packaging.md`](../packaging.md).

**Execution gate:** Prerequisite [`v0-3-naming-migration.md`](v0-3-naming-migration.md) is **done** (see [Product naming](../version-v0.3.md#product-naming)). Wrapper Phases 2–7 are unblocked.

---

## Current implementation baseline (agent-start context)

This section records the observed starting point so agents can implement in the right order without re-discovery.

- No wrapper binaries currently exist for this plan (`cmd/chimera-vectorstore`, `cmd/chimera-broker` are not present).
- Supervisor runtime is still directly coupled to upstream binaries:
  - `cmd/chimera-supervisor/main.go` starts `qdrant` via `supervisor.StartQdrant(...)`.
  - `cmd/chimera-supervisor/main.go` starts `bifrost-http` via `supervisor.StartBifrost(...)`.
  - Operator-facing supervisor flags are still upstream-specific (`-qdrant-*`, `-bifrost-*`).
- Health/readiness handling is currently direct upstream probing (no wrapper health/status layer yet):
  - shared readiness polling in `internal/supervisor/bifrost.go` (`WaitHealthy`).
  - supervisor health monitor emits `gateway.health.qdrant` and `gateway.health.bifrost`.
- Status payload and UI still encode upstream-first naming:
  - `internal/server/status.go` returns `bifrost_*` and `qdrant_*` supervisor fields.
  - logs UI components and derive logic still use `bifrost`/`qdrant` identifiers.
- Config boundary for wrappers is not implemented:
  - current runtime config still uses `rag.qdrant.*` and `upstream.bifrost_*` style fields.
  - planned stable `VECTORSTORE__*` / `BROKER__*` contract does not exist yet.
- Install/build/package flows are still upstream-binary oriented:
  - `Makefile` run targets pass upstream flags/binaries into supervisor.
  - `scripts/install-bootstrap.sh` builds/fetches upstream binaries directly.
- Wrapper conformance suite (driver parity/lifecycle/log-shape/error-normalization gate) is not present yet.

**Implementation ordering note**

1. Establish wrapper contracts and binaries first (Phase 1 + 2).
2. Prove wrapper behavior with E2E contract matrix next (Phase 3).
3. Cut supervisor over to wrappers after E2E pass (Phase 4).
4. Then complete naming/UI/docs/tooling hard-cut work (Phase 5 + 6 + 7).

---

## Phase 1 — Wrapper contract lock

**Goal.** Define and freeze the wrapper-facing contracts so all implementation work lands on one stable interface.

**Deliverables**

- Freeze v1 execution model: `binary` driver only, with driver extension points for `docker`/`remote` reserved but not shipped in this milestone.
- Define and freeze the minimal driver contract:
  - `Start(ctx) error`
  - `Stop(ctx) error`
  - `Health(ctx) (HealthStatus, error)` for `/healthz`
  - `Ready(ctx) (ReadyStatus, error)` for `/readyz`
  - `Metrics(ctx) (io.Reader, error)` for `/metrics` (Prometheus or empty)
  - `Status(ctx) (DriverStatus, error)` for summarized state
  - `DebugLogs(ctx) (io.Reader, error)` optional (`not supported` allowed)
  - contract must remain stable across backend swaps (for example qdrant/bifrost now, other engines later)
- Define health semantics and parity rule:
  - `/healthz` = wrapper process alive
  - `/readyz` = upstream backend is initialized and ready for real traffic
  - parity rule: liveness vs end-to-end readiness must be distinct and testable
  - `/metrics` must expose wrapper metrics and upstream metrics where available
- Define stable status payload schema, including:
  - required top-level fields: `component`, `backend_name`, `backend_mode`, `status` (`ok|degraded|error`), `timestamp` (RFC3339), `version.wrapper`
  - optional version fields: `version.upstream` (empty string when unknown), `version.build_sha`
  - optional operational fields: `message`, `endpoint`, `pid` (process mode), `restarts`, `last_error`, `details`
  - `component` must be exactly `chimera-vectorstore` or `chimera-broker`
  - stable enums:
    - `backend_name`: `qdrant`, `bifrost`, `milvus`, `weaviate`, `redis_vector`, `custom`
    - `backend_mode`: `binary`, `docker`, `remote`, `embedded`
- Define stable Chimera config boundary (env and/or YAML), including:
  - `VECTORSTORE__BACKEND`, `VECTORSTORE__ENDPOINT`, `VECTORSTORE__DATA_PATH`, `VECTORSTORE__LOG_LEVEL`, `VECTORSTORE__TIMEOUTS__STARTUP`, `VECTORSTORE__TIMEOUTS__SHUTDOWN`
  - matching `BROKER__*` keys with same contract shape
  - legacy upstream flag/env compatibility is explicitly unsupported in wrapper contracts (hard break)
- Define observability contract:
  - required fields: `component`, `backend_name`, `backend_mode`, `level`, `msg`, `timestamp`, `status`
  - optional correlation fields: `request_id`, `trace_id`
  - optional debug field: `upstream_raw`
  - wrappers normalize backend logs and errors into stable Chimera-facing taxonomy
- Define debug exposure policy:
  - default production profile disables `/debug/upstream/logs`
  - dev/test profiles may enable via explicit config gate
  - config gate key: `DEBUG__ENABLE_UPSTREAM_LOGS=true`
  - `/debug/*` endpoints must bind to `127.0.0.1` only by default
  - remote debug binding is refused unless explicitly overridden by env `DEBUG__ALLOW_REMOTE=true` or flag `--debug-allow-remote`
  - fixed-size in-memory ring buffer retention (`N` lines or `N` KB), configurable per wrapper
  - default retention: `10,000` lines or `1MB` (whichever comes first)
  - default mode emits Chimera-normalized logs and retains upstream raw lines only in ring buffer
  - debug mode may forward upstream logs with explicit upstream metadata
- Define lifecycle ownership split:
  - supervisor owns wrapper process lifecycle
  - wrapper owns backend lifecycle (spawn/restart/backoff/graceful shutdown)
  - wrapper CLI contract uses a backend-agnostic executable flag: `--bin` (canonical)
  - backend-specific executable flags are not part of wrapper contract (`--qdrant-bin`, `--bifrost-bin` unsupported)
  - wrapper restart/backoff defaults: initial delay `1s`, multiplier `2.0`, max delay `30s`, max retries `infinite`, reset after `60s` healthy runtime
  - graceful shutdown defaults: total timeout `15s`; stop accepting requests, graceful terminate backend, wait `10s`, then force-kill if still running and wait remaining budget
  - shutdown semantics are cross-platform: Unix uses signal-based terminate/kill; Windows uses equivalent graceful-terminate then force-kill behavior with the same timeout budget
  - optional pid/lock file behavior is wrapper-owned policy
  - wrappers emit deterministic ready log line after readiness transition
  - wrapper HTTP API surface is stable and uniform (`/healthz`, `/readyz`, `/metrics`, status, debug endpoints)
- Define failure taxonomy and exit code contract:
  - exit codes: `0` clean, `10` config, `20` backend startup, `30` backend runtime, `40` dependency, `50` internal
  - normalized status classes: `CONFIG_ERROR`, `BACKEND_STARTUP_ERROR`, `BACKEND_RUNTIME_ERROR`, `DEPENDENCY_ERROR`, `INTERNAL_ERROR`
  - startup failure semantics: startup timeout `30s`; if backend fails readiness during startup window wrapper exits `20`
  - runtime degradation semantics: if backend loses readiness after startup, wrapper remains running with `/readyz=503` and `status=degraded`
- Define security contract:
  - secrets via env or mounted files only (never CLI flags)
  - redact secret-like keys (`TOKEN`, `KEY`, `PASSWORD`, `SECRET`) in logs/status/debug
  - debug endpoints never dump raw process environment
- Define backend readiness probes for initial binary-mode targets:
  - readiness success criterion is HTTP `200` on backend readiness endpoint
  - `chimera-broker` (bifrost): `GET /models` returns `200`
  - `chimera-vectorstore` (qdrant): `GET /collections` returns `200`
- Define metrics contract:
  - required wrapper metrics:
    - `chimera_wrapper_up` (`1|0`)
    - `chimera_backend_up` (`1|0`)
    - `chimera_backend_restarts_total`
    - `chimera_requests_total{component,endpoint,status}`
    - `chimera_request_duration_seconds{component,endpoint}` (histogram)
  - endpoint label cardinality guard: `endpoint` must use bounded route names (for example `healthz`, `readyz`, `metrics`, `debug_upstream_logs`), not raw paths
  - upstream metrics are exposed pass-through with `upstream_` prefix; collisions are resolved by prefixing and streams are never merged
- Define deterministic ready log contract:
  - format: `READY: component=<...> backend=<...> mode=<...> version=<...> upstream=<...>`

**Acceptance**

- A single contract section exists in docs and is referenced by supervisor/wrapper implementation PRs.
- Contract tests (or equivalent fixtures) exist for endpoint shapes, liveness/readiness parity, and core log/lifecycle behavior.
- No unresolved ambiguity remains about naming scope, status fields, security/redaction rules, or wrapper ownership boundaries.
- Hard-cut naming scope is explicit: operator-facing UI/docs/logs/supervisor/CLI use Chimera names only; upstream names are restricted to architecture docs, debug views, and error details.

**Status:** `done`

---

## Phase 2 — Standalone wrapper binaries

**Goal.** Implement standalone wrapper binaries that manage upstream processes behind the locked Chimera contracts.

**Deliverables**

- Add `cmd/chimera-vectorstore` and `cmd/chimera-broker` entrypoints.
- Implement driver registry and driver interface in wrapper internals, shipping only `binary` driver in v1.
- Preserve extension seams for todo `docker` and `remote` drivers, and for future backend engines, without changing the wrapper-facing contract.
- Implement wrapper internals that translate stable Chimera config into upstream-specific flags/env:
  - `chimera-vectorstore` -> current qdrant integration
  - `chimera-broker` -> current bifrost integration
- Move translation logic into wrappers (not supervisor).
- Implement wrapper-owned backend lifecycle behavior (start/stop/retry/backoff/timeout/ready signal).
- Implement wrapper endpoints:
  - `/healthz` and `/readyz` using frozen parity semantics
  - `/metrics` for wrapper plus backend metrics exposure contract
  - `/debug/upstream/logs` with explicit config-gated enablement
- Implement required status/log fields and failure normalization contract.
- Add focused tests for translation correctness, lifecycle transitions, health/readiness parity, log shape requirements, and debug exposure behavior.

**Acceptance**

- Each wrapper binary can run independently and supervise its upstream successfully in binary mode.
- Wrappers expose the agreed endpoint contracts, deterministic ready signals, and required status/log fields.
- Existing upstream process control behavior is preserved or improved under wrapper control.

**Status:** `done`

---

## Phase 3 — Wrapper E2E contract matrix

**Goal.** Turn the Phase 1 contract into executable end-to-end checks that start real wrapper binaries, drive expected commands/endpoints, and validate lifecycle/health/logging/metrics behavior under success and failure.

**Deliverables**

- Add wrapper E2E harness that launches wrappers as subprocesses and controls upstream stubs/fixtures:
  - binaries under test: `chimera-broker`, `chimera-vectorstore`
  - startup command paths must mirror operator entrypoints (binary invocation, env overrides, args)
  - harness captures stdout/stderr, exit code, and elapsed runtime per scenario
- Add contract matrix tests for endpoint surface and happy-path lifecycle:
  - `GET /healthz` returns `200` while wrapper process is alive
  - `GET /readyz` returns `200` only after upstream readiness condition succeeds
  - `GET /status` schema validates required fields (`component`, `backend_name`, `backend_mode`, `status`, `timestamp`, `version.wrapper`)
  - `GET /metrics` includes required wrapper metrics and endpoint labels are bounded route names
  - deterministic ready line is emitted exactly once per successful startup transition
- Add startup-failure contract tests:
  - upstream readiness never reaches `200` within startup timeout -> wrapper exits with code `20`
  - startup failure writes normalized startup error class/status
  - `/readyz` is unavailable or `503` prior to successful startup
- Add runtime-degradation and restart/backoff contract tests:
  - after becoming ready, force upstream failure -> wrapper stays alive, `/readyz=503`, `status=degraded`
  - wrapper restart policy follows locked defaults (initial `1s`, multiplier `2.0`, cap `30s`, reset after `60s` healthy)
  - restart counters/metrics increase as expected (`chimera_backend_restarts_total`)
- Add shutdown/exit-code contract tests (cross-platform semantics):
  - graceful stop path returns exit code `0` when backend stops within timeout budget
  - forced-termination path returns exit code `30` when backend cannot stop cleanly and requires kill
  - behavior is equivalent on Unix and Windows according to wrapper contract (platform-specific mechanism, same outcome semantics)
- Add debug exposure and security contract tests:
  - `/debug/upstream/logs` disabled by default
  - enabling `DEBUG__ENABLE_UPSTREAM_LOGS=true` exposes endpoint with redacted content guarantees
  - `/debug/*` bind defaults to loopback-only; non-loopback bind is rejected unless `DEBUG__ALLOW_REMOTE=true` or `--debug-allow-remote`
  - debug endpoints never expose raw process environment
- Add metrics pass-through contract tests:
  - upstream metrics are exposed with `upstream_` prefix
  - collisions are resolved by prefixing and streams are not merged
  - contract-required wrapper metrics remain present regardless of upstream metrics availability
- Add test matrix documentation section in this plan that maps each contract clause to at least one E2E test case ID.

**Concrete E2E matrix (contract-oriented)**

- `E2E-BROKER-001` Startup happy path:
  - start `chimera-broker` with valid bifrost stub
  - expect: process alive, `/healthz=200`, `/readyz=200`, `/status.status=ok`, ready line emitted
- `E2E-BROKER-002` Status schema:
  - call `/status`
  - expect required fields and enums validate; `component=chimera-broker`, `backend_name=bifrost`, `backend_mode=binary`
- `E2E-BROKER-003` Metrics contract:
  - call `/metrics`
  - expect required wrapper metrics and bounded endpoint labels
- `E2E-BROKER-004` Debug endpoint default-off:
  - start with default config; call `/debug/upstream/logs`
  - expect disabled response (`404`/contract-disabled behavior)
- `E2E-BROKER-005` Debug enabled + redaction:
  - enable debug gate and inject upstream lines containing secret tokens
  - expect endpoint enabled and returned lines are redacted by contract rules
- `E2E-BROKER-006` Debug bind safety:
  - attempt non-loopback debug bind without override
  - expect startup refusal (config error path)
- `E2E-BROKER-007` Debug bind override:
  - set `DEBUG__ALLOW_REMOTE=true` (or flag) and non-loopback bind
  - expect startup allowed
- `E2E-BROKER-008` Startup timeout failure:
  - upstream never returns readiness `200` before timeout
  - expect wrapper exit `20`
- `E2E-BROKER-009` Runtime degradation:
  - become ready, then break upstream readiness
  - expect wrapper remains alive, `/readyz=503`, status degraded
- `E2E-BROKER-010` Restart/backoff:
  - repeatedly crash upstream
  - expect backoff sequence near `1s`, `2s`, `4s` ... capped at `30s`; restart metric increments
- `E2E-BROKER-011` Graceful shutdown:
  - send normal termination signal while upstream is responsive
  - expect clean exit `0` within shutdown timeout
- `E2E-BROKER-012` Forced kill shutdown:
  - make upstream ignore graceful stop
  - expect forced termination path and exit `30`
- `E2E-BROKER-013` Upstream metrics prefixing:
  - provide upstream metrics with known names/collisions
  - expect all upstream metrics exposed with `upstream_` prefix and no merge with wrapper metrics
- `E2E-VECTORSTORE-001..013` Mirror set for `chimera-vectorstore` using qdrant readiness endpoint (`GET /collections`) and vectorstore-specific startup flags.
  - startup invocation contract for both wrappers uses `--bin <path>` as canonical executable input

### Contract clause to E2E mapping

| Phase 1 contract clause | Broker test IDs | Vectorstore test IDs |
|---|---|---|
| Liveness and readiness parity (`/healthz` vs `/readyz`) | `E2E-BROKER-001`, `E2E-BROKER-008`, `E2E-BROKER-009` | `E2E-VECTORSTORE-001`, `E2E-VECTORSTORE-008`, `E2E-VECTORSTORE-009` |
| Status schema required fields and enums | `E2E-BROKER-002` | `E2E-VECTORSTORE-002` |
| Required wrapper metrics and bounded endpoint labels | `E2E-BROKER-003` | `E2E-VECTORSTORE-003` |
| Deterministic ready transition signal | `E2E-BROKER-001` | `E2E-VECTORSTORE-001` |
| Startup timeout exit code (`20`) and startup-failure path | `E2E-BROKER-008` | `E2E-VECTORSTORE-008` |
| Runtime degradation after successful startup (`/readyz=503`, `status=degraded`) | `E2E-BROKER-009` | `E2E-VECTORSTORE-009` |
| Restart/backoff policy and restart counters | `E2E-BROKER-010` | `E2E-VECTORSTORE-010` |
| Graceful shutdown exit code (`0`) | `E2E-BROKER-011` | `E2E-VECTORSTORE-011` |
| Forced termination exit code (`30`) | `E2E-BROKER-012` | `E2E-VECTORSTORE-012` |
| Debug endpoint disabled by default | `E2E-BROKER-004` | `E2E-VECTORSTORE-004` |
| Debug enabled + redaction guarantees | `E2E-BROKER-005` | `E2E-VECTORSTORE-005` |
| Debug bind loopback-only default and override gate | `E2E-BROKER-006`, `E2E-BROKER-007` | `E2E-VECTORSTORE-006`, `E2E-VECTORSTORE-007` |
| Upstream metrics pass-through prefixing and collision handling | `E2E-BROKER-013` | `E2E-VECTORSTORE-013` |
| Canonical startup contract (`--bin <path>`) | `E2E-BROKER-001` | `E2E-VECTORSTORE-001` |

Deterministic gated targets for this matrix:

- `make chimera-broker-test-e2e`
- `make chimera-vectorstore-test-e2e`

**Acceptance**

- A runnable E2E suite exists that launches real wrapper binaries and validates all contract endpoints and lifecycle paths.
- Every contract item from Phase 1 maps to at least one E2E test ID in this phase matrix.
- CI includes wrapper E2E jobs (or deterministic gated target) for broker and vectorstore wrappers.
- The suite catches regressions in startup readiness, degradation semantics, debug gating/binding, metrics shape/prefixing, and exit-code behavior.

**Status:** `done`

---

## Phase 4 — Supervisor cutover

**Goal.** Rewire supervisor orchestration to treat wrappers as the only managed service binaries.

**Deliverables**

- Update supervisor startup flow to manage `chimera-vectorstore` and `chimera-broker` processes.
- Replace direct upstream process configuration in supervisor with wrapper contract configuration.
- Update health polling and readiness gating to wrapper endpoints (`/healthz`, `/readyz`) and wrapper status payloads.
- Enforce lifecycle split in runtime behavior:
  - supervisor restarts wrapper if wrapper exits unexpectedly
  - wrapper handles backend restarts/backoff internally
- Remove direct ownership of upstream translation concerns from supervisor code paths.
- Preserve failure handling and shutdown guarantees while delegating backend lifecycle semantics to wrappers.

**Acceptance**

- Supervisor no longer requires direct qdrant/bifrost startup flags as operator-facing contract.
- Full stack starts and stops correctly with wrappers as children.
- Startup failures and degraded states surface through wrapper-centric logs and health checks with normalized error classes.

**Status:** `done`

---

## Phase 5 — Logs and UI naming hard cut

**Goal.** Make operator-facing logs and UI consistently use Chimera wrapper names while keeping upstream details available only where intended.

**Deliverables**

- Update service IDs, card titles, chips, subtitles, and descriptive copy to `chimera-vectorstore` and `chimera-broker`.
- Preserve stable two-card UX regardless of backend implementation; backend appears as detail field (for example, `Backend: Qdrant (binary)`).
- Update log normalization and presentation layers so default operator views show Chimera-normalized `msg` taxonomy and required fields.
- Route upstream raw details to debug paths/views only, with explicit upstream metadata labels and config-gated visibility.
- Update tests for logs UI/service card behavior, naming assertions, and required log/status field presence.
- Ensure error details can still include upstream name/version when needed for diagnostics.

**Acceptance**

- Operator-facing UI paths no longer present qdrant/bifrost as primary service names.
- Default logs view is Chimera-normalized, and debug drill-down still exposes upstream context only when enabled.
- Existing operational signal quality (status cards, counters, event timelines) remains intact.

**Status:** `done`

---

## Phase 6 — Make, scripts, and packaging alignment

**Goal.** Align build and operational tooling around wrapper binaries and Chimera naming.

**Deliverables**

- Add/update make targets for wrapper build/run/test/install flows.
- Update install/bootstrap scripts so wrapper binaries are first-class runtime artifacts.
- Keep upstream directories and fetched sources unchanged where needed, but treat them as internal implementation dependencies.
- Update packaging/release assembly to include wrapper binaries and updated runtime instructions.
- Document v1 runtime prerequisites as binary-only, with no container runtime requirement.
- Record todo container/remote driver prerequisites and fail-fast behavior contract:
  - unsupported backend mode returns `DEPENDENCY_ERROR` and explicit degraded/error status
  - Windows desktop treats container mode as capability-gated and fail-fast when unsupported
- Update script comments/help output to Chimera wrapper terminology.

**Acceptance**

- `make` workflows can build and run the supervised stack through wrapper binaries end-to-end.
- Packaged artifacts and runtime docs reference wrappers as managed components.
- No operator-facing script/help text uses upstream names as primary service identifiers.

**Status:** `done`

---

## Phase 7 — Documentation

**Goal.** Complete a hard-cut documentation and validation pass so operator guidance matches the new runtime contract.

**Deliverables**

- Update operator docs to Chimera names only (README, configuration, supervisor, installation, packaging).
- Restrict upstream-name mentions to architecture and debug-oriented sections only.
- Add wrapper-driver conformance suite as swap-readiness gate for future adapters, covering:
  - health/readiness parity checks
  - lifecycle tests (start/stop/restart/failure handling)
  - config mapping tests
  - error normalization tests
  - log shape tests (required fields)
  - baseline vector/broker operation checks per driver capability
- Run validation pass across build/test/run/package workflows and record results in this plan.
- Capture post-cutover follow-ups for adding container/remote drivers against the same conformance suite.

**Acceptance**

- Operator docs are internally consistent with wrapper-first runtime behavior.
- Migration validation confirms supervised startup, health, logs, and shutdown all work via wrappers.
- Conformance suite exists and is usable as a gate for future backend drivers.
- Plan status can move from `draft` to `active` with clear execution readiness and no blocking unknowns.

**Status:** `done`

---

## References

- Code: `cmd/chimera`, `cmd/chimera-supervisor`, `internal/supervisor`, `internal/servicelogs`, `internal/server/embedui`
- Docs: [`../supervisor.md`](../supervisor.md), [`../configuration.md`](../configuration.md), [`../packaging.md`](../packaging.md), [`v0-3-naming-migration.md`](v0-3-naming-migration.md)
