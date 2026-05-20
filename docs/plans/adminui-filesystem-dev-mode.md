# Plan: Operator UI filesystem dev mode

| Field | Value |
|-------|-------|
| **Doc kind** | `feature-plan` |
| **Owners / areas** | Gateway operator embed UI (`chimera-gateway/internal/server/adminui/embed`) |
| **Status** | `shipped` |
| **Targets** | Gateway / desktop v0.3 developer workflow |
| **Last updated** | See git history |
| **Supersedes / superseded by** | Complements [`embedui-component-gallery.md`](embedui-component-gallery.md) (CSS-only, no live APIs) |

## At a glance

Developers editing the summarized logs UI today must rebuild `chimera-gateway` (and often restart the supervised stack) to see JavaScript or CSS changes. This plan adds an optional **filesystem asset root** so the gateway serves `embedui/` from disk while production builds keep compile-time `//go:embed`. Set one environment variable, run `locus-desktop` or `chimera-supervisor` as usual, edit files under `adminui/embed/embedui/`, and refresh the browser—no gateway rebuild for static asset changes.

| Phase | Outcome | Status |
|-------|---------|--------|
| [Phase 1 — Asset source abstraction](#phase-1--asset-source-abstraction) | Gateway reads `CHIMERA_ADMINUI_ROOT` and serves the same `/ui/*` URLs from disk when set | `done` |
| [Phase 2 — Operator docs and v0.3 tracking](#phase-2--operator-docs-and-v03-tracking) | Plan, README, and version roadmap document the dev workflow | `done` |
| [Phase 3 — Optional ergonomics](#phase-3--optional-ergonomics) | Make helper, startup log line, loopback-only guard, env.example, installation docs | `done` |

---

## Background

Operator UI assets live under `chimera/chimera-gateway/internal/server/adminui/embed/embedui/` and are registered in `embed/routes.go` with paths like `embedui/logs.html` and `/ui/assets/logs/`. The component gallery under `docs/component-gallery/` already loads production CSS from disk for visual iteration but does not exercise auth, `/api/ui/*`, or supervisor log streaming.

Supervisor and desktop already forward the parent process environment to the gateway child (`mergeEnv` in `chimera-supervisor/internal/supervise/env.go`), so a single env var on the developer shell is enough—no new supervisor flags required for the first cut.

**Related docs:** [`embedui/logs/README.md`](../../chimera/chimera-gateway/internal/server/adminui/embed/embedui/logs/README.md), [`version-v0.3.md`](../version-v0.3.md), [`embedui-component-gallery.md`](embedui-component-gallery.md), [`locus-desktop-supervisor-contract.md`](locus-desktop-supervisor-contract.md).

---

## Phase 1 — Asset source abstraction

**Goal.** When `CHIMERA_ADMINUI_ROOT` points at a valid tree, `embed.ReadFile` and static handlers serve files from disk; when unset, behavior is unchanged (embedded FS only).

**Deliverables**

- `CHIMERA_ADMINUI_ROOT` constant in [`internal/naming/contracts.go`](../../internal/naming/contracts.go).
- `embed` package: resolve root (directory containing `embedui/logs.html`), `os.DirFS` backend, fallback to embedded FS on missing/invalid path.
- Unit tests: embedded default; disk mode serves a known file from the repo tree.
- Existing path traversal rules preserved in `ServePathPrefix`.

**Acceptance**

- With env set to `.../adminui/embed`, `GET /ui/assets/logs/main.js` returns on-disk bytes after a file edit and browser refresh (no gateway rebuild).
- With env unset, release behavior unchanged.
- `go test ./chimera/chimera-gateway/internal/server/adminui/embed/...` passes.

**Status:** `done`

---

## Phase 2 — Operator docs and v0.3 tracking

**Goal.** Developers can discover the workflow without reading this plan.

**Deliverables**

- Dev workflow section in [`embedui/logs/README.md`](../../chimera/chimera-gateway/internal/server/adminui/embed/embedui/logs/README.md).
- Row and section in [`version-v0.3.md`](../version-v0.3.md).
- Entry in **Related plans** table in `version-v0.3.md`.

**Acceptance**

- README documents env var, example paths (repo root and Windows), and what still requires `make chimera-gateway-build` (Go handlers, `contracts.js` generation).

**Status:** `done`

---

## Phase 3 — Optional ergonomics

**Goal.** Reduce friction for daily use; not required for the first merge.

**Deliverables**

- Structured log when disk mode is active (`gateway.startup.adminui_filesystem`).
- Refuse disk mode when gateway listen is not loopback (`gateway.startup.adminui_filesystem_remote_denied`).
- `CHIMERA_ADMINUI_ROOT` documented in `env.example`.
- Dev workflow in `docs/installation.md`.
- `make locus-desktop-dev-ui` and `make chimera-supervisor-dev-ui`.

**Acceptance**

- Operator can copy `env.example`, uncomment `CHIMERA_ADMINUI_ROOT`, and run desktop with disk UI.
- Operator can run `make locus-desktop-dev-ui` from the repo root.

**Status:** `done`

---

## Resolved decisions

1. **Auto-reload** — **Browser refresh only** for v1. No dev-only SSE/WebSocket reload in this plan; revisit if refresh friction remains high.
2. **Root path alias** — **Shipped in Phase 1:** accept `.../adminui/embed` or `.../embed/embedui`; normalized to the parent of `embedui/`.

---

## References

- Code: [`chimera/chimera-gateway/internal/server/adminui/embed/`](../../chimera/chimera-gateway/internal/server/adminui/embed/), [`embed/routes.go`](../../chimera/chimera-gateway/internal/server/adminui/embed/routes.go)
- Tests: [`embedui_test/goja_test.go`](../../chimera/chimera-gateway/internal/server/adminui/embed/embedui_test/goja_test.go) (on-disk paths today)
- Supervisor env: [`chimera-supervisor/internal/supervise/env.go`](../../chimera/chimera-supervisor/internal/supervise/env.go)
