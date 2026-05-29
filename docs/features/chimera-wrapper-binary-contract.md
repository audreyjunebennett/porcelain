# Feature: Chimera wrapper binary contract

| Field | Value |
|-------|-------|
| **Doc kind** | `platform-contract` |
| **Areas** | `chimera-broker`, `chimera-vectorstore`, `chimera-gateway`, `chimera-indexer`, `chimera-supervisor`, `internal/wrapper` |
| **Status** | `current` |
| **Introduced** | Wrapper hard cut (gateway v0.4 naming train) |
| **Originated from** | [`plans/vectorstore-broker-wrapper-hard-cut.md`](../plans/vectorstore-broker-wrapper-hard-cut.md) |
| **Related features** | [Product naming contract](product-naming-contract.md), [Structured operator log lines](structured-operator-log-lines.md) |
| **Depends on** | `internal/naming` env prefixes |
| **Last updated** | See git history |

## At a glance

Every managed Chimera service binary that wraps an upstream backend (or a split backend process) implements a **shared wrapper contract**: stable HTTP probes, status JSON, Prometheus metrics, normalized structured logs, deterministic ready line, exit codes, and backend lifecycle ownership. New `chimera-*` wrappers adopt `chimera/internal/wrapper/runtime` and `chimera/internal/wrapper/contract` rather than inventing per-binary health or shutdown semantics. v1 ships **binary driver only**; `docker` / `remote` modes are reserved in enums but not implemented.

## Operator-visible behavior

Operators and the settings UI see **Chimera component names** (`chimera-broker`, `chimera-vectorstore`, `chimera-gateway`, …) in logs and health cards. Upstream names (`qdrant`, `bifrost`, …) appear only in debug views, architecture docs, or error detail — not as the primary service label.

## System behavior and contracts

**Invariants**

- **`/healthz`** — wrapper HTTP server is alive (process liveness).
- **`/readyz`** — upstream backend accepts real traffic; returns `503` when degraded.
- **Lifecycle split** — supervisor owns wrapper **process** spawn; wrapper owns backend spawn, restart/backoff, graceful shutdown.
- **Canonical CLI** — backend executable is `--bin` (never `--qdrant-bin`, `--bifrost-bin`, etc.).
- **Legacy upstream flags/env on wrapper public surface** — unsupported (`LegacyCompatibilitySupported = false`).
- **Secrets** — env or mounted files only; never CLI flags. Keys containing `TOKEN`, `KEY`, `PASSWORD`, `SECRET` are redacted in logs/status/debug.
- **Debug endpoints** — disabled by default; loopback bind unless `DEBUG__ALLOW_REMOTE=true` or `--debug-allow-remote`.

**Exit codes**

| Code | Meaning |
|------|---------|
| `0` | Clean shutdown |
| `10` | Config error |
| `20` | Backend startup failure (readiness timeout during startup window) |
| `30` | Backend runtime failure / forced kill after shutdown budget |
| `40` | Dependency error |
| `50` | Internal error |

**Status payload** (`StatusPayload` in contract) — required fields: `component`, `backend_name`, `backend_mode`, `status` (`ok` \| `degraded` \| `error`), `timestamp` (RFC3339), `version.wrapper`. Allowed `component` values: `chimera-vectorstore`, `chimera-broker`, `chimera-supervisor`, `chimera-gateway`, `chimera-indexer`.

**Ready line** — emitted once per successful readiness transition:

`READY: component=<…> backend=<…> mode=<…> version=<…> upstream=<…>`

**Backoff defaults** — initial `1s`, multiplier `2.0`, max `30s`, reset after `60s` healthy, infinite retries (`-1`).

**Shutdown defaults** — startup timeout `30s`, shutdown timeout `10s`, terminate wait `5s`; cross-platform graceful-then-force semantics.

**Decisions**

| Topic | Decision |
|-------|----------|
| Driver interface | `wrapper/runtime.Adapter`: `Start`, `ReadyURL`, `MetricsURL`, `BackendName` |
| Metrics | `chimera_wrapper_up`, `chimera_backend_up`, `chimera_backend_restarts_total`, bounded `endpoint` labels |
| Debug ring buffer | Default 10_000 lines or 1 MB |
| Readiness probes (binary v1) | broker → `GET /models` 200; vectorstore → `GET /collections` 200 |

## Interfaces

| Surface | Detail |
|---------|--------|
| HTTP | `GET /healthz`, `GET /readyz`, `GET /metrics`, optional `GET /status`, gated `GET /debug/broker/logs` or `/debug/vectorstore/logs` |
| Env (broker) | `BROKER__LISTEN`, `BROKER__BIN`, `BROKER__ENDPOINT`, `BROKER__DATA_PATH`, `BROKER__TIMEOUTS__*` — see [product naming](product-naming-contract.md) |
| Env (vectorstore) | `VECTORSTORE__*` — same shape |
| Env (gateway wrapper) | `GATEWAY__LISTEN`, `GATEWAY__BIN`, `GATEWAY__BACKEND_LISTEN`, … |
| Debug gates | `DEBUG__ENABLE_BROKER_LOGS`, `DEBUG__ENABLE_VECTORSTORE_LOGS`, `DEBUG__ALLOW_REMOTE` |

## Code map

| Concern | Location |
|---------|----------|
| Contract constants, validation | `chimera/internal/wrapper/contract/contract.go` |
| Shared runtime (HTTP, lifecycle, metrics) | `chimera/internal/wrapper/runtime/runtime.go` |
| Reference implementations | `chimera/chimera-broker/main.go`, `chimera-vectorstore/main.go`, `chimera-gateway/main.go`, `chimera-indexer/main.go` |
| Backend adapters | `chimera/chimera-broker/adapter/`, `chimera-vectorstore/adapter/`, … |
| Supervisor spawns wrappers | `chimera/chimera-supervisor/internal/supervise/children.go` |
| Env key names | `internal/naming/contracts.go` |

## Verification

```bash
go test ./chimera/internal/wrapper/contract/...
go test ./chimera/internal/wrapper/runtime/...
go test ./chimera/chimera-broker/ -run E2E -count=1
go test ./chimera/chimera-vectorstore/ -run E2E -count=1
go test ./chimera/chimera-gateway/ -run E2E -count=1
go test ./chimera/chimera-supervisor/ -run E2E -count=1
```

## Out of scope and known gaps

- `docker` and `remote` backend drivers — enum only.
- Unified env precedence ([`env-precedence-contract`](../plans/env-precedence-contract.md) draft) — wrappers read env today but no shared profile resolver.
- [`docs/supervisor.md`](../supervisor.md) defers stack layout detail to this doc and [Locus desktop ↔ supervisor](locus-desktop-supervisor.md).

## References

- Delivery plan (historical): [`plans/vectorstore-broker-wrapper-hard-cut.md`](../plans/vectorstore-broker-wrapper-hard-cut.md)
- Operator runbook: [`supervisor.md`](../supervisor.md)
- Naming env keys: [`product-naming-contract.md`](product-naming-contract.md)
