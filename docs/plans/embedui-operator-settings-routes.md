# Plan: Operator UI settings routes and app shell

| Field | Value |
|-------|-------|
| **Doc kind** | `refactor-plan` |
| **Owners / areas** | Gateway admin UI (`adminui/embed`), Locus desktop login defaults, embed UI tests |
| **Status** | `done` |
| **Targets** | Gateway / operator UI v0.3 |
| **Last updated** | See git history |
| **Supersedes / superseded by** | Supersedes scattered notes in [`embedui-component-system.md`](embedui-component-system.md) and [`chimera-gateway-package-boundaries.md`](chimera-gateway-package-boundaries.md) for `/ui/logs` / `/ui/desktop` entry URLs |

## At a glance

Operators land on a simple **app shell** at `/ui` (top bar with settings, iframe showing the PWA placeholder). Configuration and observability live on **`/ui/settings`** (today’s logs page), opened from the settings control like the old desktop shell—not as a separate “logs” product surface. Routes, HTML filenames, and static asset URLs align 1:1 so paths are predictable; legacy redirects and deep links are removed (no prior users). **`/api/ui/*` JSON/SSE endpoints stay unchanged.**

| Phase | Outcome | Status |
|-------|---------|--------|
| [Phase 1 — Routes and aligned page files](#phase-1--routes-and-aligned-page-files) | `/ui`, `/ui/pwa`, `/ui/settings`, `/ui/settings/gallery` only; files renamed to match | `done` |
| [Phase 2 — Shell, settings UX, and deep-link removal](#phase-2--shell-settings-ux-and-deep-link-removal) | Default app entry, embedded settings, no `?focus=` / legacy chrome | `done` |
| [Phase 3 — Gallery consolidation and refresh icons](#phase-3--gallery-consolidation-and-refresh-icons) | Single settings gallery; token swatches removed; Material `refresh` replaces `reload.svg` | `done` |
| [Phase 4 — Tests, tooling, and login defaults](#phase-4--tests-tooling-and-login-defaults) | Go tests, gallery path checker, Locus `next` paths updated | `done` |

---

## Background

The operator UI grew around **`/ui/logs`** (summarized feed + admin cards) and a **`/ui/desktop`** shell that iframe’d `/ui/pwa` and toggled `/ui/logs?embed=1`. That naming no longer matches product intent: the page is **settings/admin**, not a logs-first app. Legacy routes (`/ui/panel`, `/ui/metrics`, `/ui/indexer`) and query deep links (`?focus=admin`, `?focus=metrics`, etc.) exist only for migration—there are **no previous users**, so redirects and deep links can be deleted outright.

**Related docs:** [`version-v0.3.md`](../version-v0.3.md), [`installation.md`](../installation.md), [`plans/embedui-component-gallery.md`](embedui-component-gallery.md), [`plans/chimera-gateway-package-boundaries.md`](chimera-gateway-package-boundaries.md). Operator-facing doc updates (README, plans mentioning `/ui/logs`) are tracked separately by the doc owner.

**API naming (unchanged):** Operator JSON/SSE remains under **`/api/ui/*`** (e.g. `/api/ui/logs`, `/api/ui/state`)—not `/ui/api/*`. Pages live under `/ui/*`; APIs under `/api/ui/*` keeps machine endpoints separate from HTML and matches gateway logging/middleware conventions.

---

## Phase 1 — Routes and aligned page files

**Goal.** HTTP routes and on-disk embed filenames match: no surprise where `GET /ui/settings` is implemented.

**Deliverables**

- **`routes.go`:** `GET /ui` → app shell (today `shell.html` → rename `index.html`). `GET /ui/pwa` unchanged. `GET /ui/settings` → settings page (today `logs.html` → `settings.html`). `GET /ui/settings/gallery` → single gallery HTML (today `gallery/gallery-unified-operator.html` → e.g. `settings/gallery.html`).
- **Remove handlers** (no redirects): `/ui/desktop`, `/ui/logs`, `/ui/panel`, `/ui/metrics`, `/ui/indexer`, `/ui/gallery`, `/ui/gallery/operator`, `/ui/gallery/tokens`.
- **Static assets:** Rename served paths to match settings naming, e.g. `/ui/assets/settings.css`, `/ui/assets/settings.js`, `/ui/assets/settings/main.js`, `/ui/assets/settings/**` (from `logs.css`, `logs_entry.js`, `logs_app.js`, `embedui/logs/`). Update `assets.go` `//go:embed` and all `<script>` / `<link>` in HTML.
- **Remove** `reload.svg` from embed and `GET /ui/assets/reload.svg`.

**Acceptance**

- Authenticated operator can load `/ui`, `/ui/pwa`, `/ui/settings`, and `/ui/settings/gallery` only (plus login/setup and shared `/ui/assets/*`).
- Grep for removed route strings in `chimera-gateway` returns no live handlers (docs may lag until updated separately).

**Status:** `done`

---

## Phase 2 — Shell, settings UX, and deep-link removal

**Goal.** The app shell is the default post-login surface; settings opens in-place via the gear control; URL query params no longer drive card focus.

**Deliverables**

- **`index.html` (shell):** Default iframe `/ui/pwa`. Settings button toggles `/ui/settings?embed=1` (rename `LOGS_ROUTE` / `isLogsRoute` → settings naming). Optional: rename `postMessage` type `chimera-logs-activate` → `chimera-settings-activate` in shell + `streaming.js`.
- **Load Material Symbols** on the shell page for the reload control (see Phase 3).
- **`settings.html`:** Remove chrome nav links to Shell / Metrics / Admin (`?focus=*`). Add link to **Component gallery** → `/ui/settings/gallery`.
- **`logs_app.js` / `summarizedFeed.js` / `sumEvlog.js`:** Remove parsing and behavior for `focus`, `card`, `principal`, `conversation`, `conv`, `seq` query params (including scroll-to-card and evlog row highlight).
- **Login / desktop defaults:** `handler.SanitizeLoginNext` default → `/ui`; `login.html` fallback `next` → `/ui`; `internal/locus/res.go` `DefaultLoginNextPath` → `/ui` (not `/ui/logs` or `/ui/desktop`).
- **Embedded mode:** Keep `?embed=1` and `logs-embedded` styling; hide settings chrome appropriate for iframe (no shell link).

**Acceptance**

- After login, operator sees `/ui` with PWA placeholder; settings opens embedded settings and closes back to prior iframe route.
- Opening `/ui/settings?focus=admin` (or any former deep link) does not change scroll/focus behavior (params ignored or absent).
- No references to `/ui/desktop` or `/ui/logs` in embed HTML/JS or Locus login URL builder.

**Status:** `done`

---

## Phase 3 — Gallery consolidation and refresh icons

**Goal.** One component gallery under settings; no token swatch page; reload affordances use Material Symbols.

**Deliverables**

- **Delete** `gallery/sample.html` and any gallery “tokens” route/documentation for `/ui/gallery/tokens`.
- **Gallery assets:** Serve gallery CSS/JS at **`/ui/assets/gallery/`** → `embedui/gallery/`; HTML entry at `GET /ui/settings/gallery` → `embedui/settings/gallery.html`.
- **Replace `reload.svg`:** Shell reload button and admin YAML revert buttons (`.sg-op-reload-icon` in `adminRouterModels.js`, `adminRouting.js`, `adminFallback.js`) use `<span class="material-symbols-outlined" aria-hidden="true">refresh</span>`; remove mask CSS tied to `/ui/assets/reload.svg`.

**Acceptance**

- Only one gallery URL is linked from settings; token swatch page 404s or is gone.
- Reload/revert controls render without requesting `reload.svg`.

**Status:** `done`

---

## Phase 4 — Tests, tooling, and login defaults

**Goal.** CI and path-check scripts enforce the new route and asset layout.

**Deliverables**

- Update **`ui_logs_test.go`**, **`ui_metrics_test.go`**, **`ui_login_env_test.go`**: assert `/ui`, `/ui/settings`, `/ui/pwa`; remove tests for legacy redirects and `?focus=` locations.
- Update **`scripts/check-component-gallery-paths.ps1`** and **`.sh`**: gallery directory/path allowlists for `/ui/settings` and new asset prefixes.
- **`gatewayCardModel.js`:** Replace `/ui/logs` path case with `/ui/settings` if still used for log classification.
- **LogStore gate:** Shell, settings, gallery, and static assets register **without** `LogStore != nil` (log stream APIs remain gated in `api/logs/register.go`).

**Acceptance**

- `go test ./chimera/chimera-gateway/...` passes for UI route tests.
- Gallery path checker passes on embed HTML under the new tree.

**Status:** `done`

---

## Decisions (resolved)

1. **LogStore nil:** All operator HTML routes (`/ui`, `/ui/pwa`, `/ui/settings`, `/ui/settings/gallery`) and embed assets register unconditionally; only `/api/ui/logs*` stays behind `LogStore`.
2. **Internal JS namespace:** `globalThis.ChimeraSettings`; modules under `embedui/settings/` (renamed from `logs/`).
3. **Gallery asset prefix:** **`/ui/assets/gallery/`** → `embedui/gallery/`.

---

## References

- Code: [`routes.go`](../../chimera/chimera-gateway/internal/server/adminui/embed/routes.go), [`embedui/index.html`](../../chimera/chimera-gateway/internal/server/adminui/embed/embedui/index.html), [`embedui/settings.html`](../../chimera/chimera-gateway/internal/server/adminui/embed/embedui/settings.html), [`adminui/register.go`](../../chimera/chimera-gateway/internal/server/adminui/register.go)
- Docs: [`version-v0.3.md`](../version-v0.3.md), [`plans/embedui-component-gallery.md`](embedui-component-gallery.md)
- Tests: [`ui_logs_test.go`](../../chimera/chimera-gateway/internal/server/ui_logs_test.go), [`scripts/check-component-gallery-paths.ps1`](../../scripts/check-component-gallery-paths.ps1)
