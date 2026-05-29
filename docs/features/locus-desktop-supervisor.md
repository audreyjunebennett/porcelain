# Feature: Locus desktop ↔ supervisor contract

| Field | Value |
|-------|-------|
| **Doc kind** | `platform-contract` |
| **Areas** | `locus-desktop`, `chimera-supervisor`, packaging |
| **Status** | `partial` |
| **Introduced** | v0.4 desktop/supervisor boundary |
| **Originated from** | [`plans/locus-desktop-supervisor-contract.md`](../plans/locus-desktop-supervisor-contract.md) |
| **Related features** | [Chimera wrapper binary contract](chimera-wrapper-binary-contract.md), [Structured operator log lines](structured-operator-log-lines.md) |
| **Depends on** | Wrapper health/readiness endpoints on supervisor control plane |
| **Last updated** | See git history |

## At a glance

`locus-desktop` is a **client launcher** for `chimera-supervisor` — it owns the native window and startup UX; supervisor owns gateway and managed wrapper children. Desktop uses **connect-first**: probe an existing local supervisor, else start one. Shutdown policy depends on ownership: attaching to an existing supervisor does **not** stop it on window close; a desktop-started supervisor is stopped by default. Readiness uses supervisor `/healthz` and `/readyz`; entry URL comes from `/status` operator UI hints.

## Operator-visible behavior

Double-clicking `locus-desktop` opens the gateway operator UI in a webview after supervisor readiness. Failures show a dedicated cannot-connect page with actionable errors (missing binary, timeout, version mismatch). When desktop starts supervisor, child logs mirror to `data/locus-desktop-supervisor.log` as normalized JSON.

## System behavior and contracts

**Process ownership**

- `locus-desktop` — window lifecycle, folder picker bridges, external URL open, startup UX.
- `chimera-supervisor` — spawns `chimera-gateway`, `chimera-broker`, `chimera-vectorstore`, optional `chimera-indexer`; exposes control HTTP plane.

**Startup**

- Binary discovery: sibling of `locus-desktop`, bundle path, PATH (`ResolveSupervisorBinary`).
- Connect-first with attach timeout `60s`; launch lock timeout `4s`; readiness wait `45s`.
- Supervisor args: pass-through from desktop except launcher-only flags stripped (`FilterSupervisorArgs` removes `-log-dir`, legacy `desktop` arg).
- Base URL derived from supervisor `-listen` (default from `internal/locus`).

**Readiness handshake**

- `Reachable` → `GET /healthz` returns 200.
- `Ready` → `GET /readyz` 200; narrow `/status` bootstrap fallback when `503`.
- `EntryURL` → `/ui/setup` when bootstrap mode; else `/ui/login?next=…` from `/status.details.operator_ui`.

**Shutdown**

- `RequestShutdown` → `POST /shutdown` on supervisor control URL (owned supervisor path).
- Owned supervisor stop timeout `40s`.
- Attach mode: desktop close does **not** stop an existing supervisor.

**Version compatibility**

- Strict gating for incompatible supervisor versions; stable version fields — not commit SHA alone.

**Runtime layout (bundle launch)**

- Desktop binary under `porcelain/locus/bin`; runtime root = parent of bin directory.
- `config/` and `data/` relative to runtime root.
- Desktop state under `data/locus-desktop/` (lock, launch metadata, lifecycle trace).

**Decisions**

| Topic | Decision |
|-------|----------|
| Remote supervisor | Out of scope v1 — local only |
| Operator UI host | Gateway (`chimera-gateway`), not supervisor control plane |
| Log mirror format | Same normalized JSON as supervisor buffer — see [structured log lines](structured-operator-log-lines.md) |

## Interfaces

| Surface | Detail |
|---------|--------|
| Supervisor probes | `GET /healthz`, `GET /readyz`, `GET /status`, `POST /shutdown` |
| Desktop env | `LOCUS_DESKTOP_TRACE`, `LOCUS_DESKTOP_LOG_DIR` |
| Webview bridges | `chimeraPickFolder`, `chimeraOpenExternalURL`, `chimeraRevealProjectPath` |
| Log file | `<logDir>/locus-desktop-supervisor.log` (default log dir `data/`) |

## Code map

| Concern | Location |
|---------|----------|
| Launcher + ownership | `locus/locus-desktop/internal/launcher/launcher.go` |
| Supervisor HTTP client | `locus/locus-desktop/internal/supervisor/client.go` |
| Shared names/paths | `internal/locus/res.go` |
| App shell | `locus/locus-desktop/internal/app/app.go` |
| Supervisor control plane | `chimera/chimera-supervisor/internal/control/` |

## Verification

```bash
go test ./locus/locus-desktop/internal/launcher/...
go test ./locus/locus-desktop/internal/supervisor/...
```

Manual: `make locus-desktop-run` — confirm connect-first, login route, owned shutdown.

## Out of scope and known gaps

- **Phase 4 packaging** ([`locus-desktop-supervisor-contract`](../plans/locus-desktop-supervisor-contract.md)) — cross-platform bundle layout enforcement still `todo`.
- Curated supervisor arg allowlist — explicitly deferred (pass-through v1).

## References

- Delivery plan: [`locus-desktop-supervisor-contract.md`](../plans/locus-desktop-supervisor-contract.md)
- Stack runbook: [`supervisor.md`](../supervisor.md)
- Packaging: [`packaging.md`](../packaging.md)
