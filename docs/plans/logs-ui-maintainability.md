# Plan: Logs UI — Maintainability, Clarity, and Testability

| Field | Value |
|-------|-------|
| **Doc kind** | `refactor-plan` |
| **Owners / areas** | Gateway embed UI, operator logs |
| **Status** | `superseded` |
| **Targets** | Logs UI maintainability, clarity, and testability |
| **Last updated** | 2026-05-18 |
| **Supersedes / superseded by** | Follows [`log-view-refactor.plan.md`](log-view-refactor.plan.md); **superseded by** [`chimera-gateway-refactor.md`](chimera-gateway-refactor.md) Phases 5–6 |

## At a glance

Keep the operator log page easy to evolve. Make file names match what they do, shrink the big "do everything" script into focused modules, and grow tests that catch regressions without spinning up a browser. The goal is faster, safer changes for whoever (human or agent) edits the page next.

| Phase | Outcome | Status |
|-------|---------|--------|
| [Workstream A — Documentation & naming](#workstream-a--documentation-and-naming-low-risk-high-leverage) | Developer map; honest file names; `models.js` actually loaded | `done` (see `embedui/logs/README.md`) |
| [Workstream B — HTML clarity](#workstream-b--html-clarity) | Semantic regions and stable test hooks | `done` (`data-testid`, `<section>`) |
| [Workstream C — CSS clarity](#workstream-c--css-clarity) | Sectioned `logs.css` paired with a DOM map | `partial` (broker/vectorstore class renames; full CSS sectioning optional) |
| [Workstream D — JavaScript architecture](#workstream-d--javascript-architecture-main-effort) | Split `logs_main.js` into focused modules with injectable deps | `done` (`logs_app.js`, `app/*`, `render/sumEvlog.js`) |
| [Workstream E — Testing strategy](#workstream-e--testing-strategy) | Goja coverage for new pure modules and view-mode logic | `done` (`logs_components_test.go`) |
| [Workstream F — Server alignment](#workstream-f--server-alignment) | Document `servicelogs.Entry` for UI parsing assumptions | `todo` (optional follow-up) |

---

**Route:** `/ui/logs`  
**Code:** `internal/server/adminui/embedui/logs.html`, `logs.css`, `logs_app.js`, `logs_entry.js`, `embedui/logs/**`  
**Server:** `internal/server/adminui/ui_handlers.go` (embed + asset routes); see [`embedui/logs/README.md`](../../chimera/chimera-gateway/internal/server/adminui/embedui/logs/README.md)

This document is a **follow-on** to [`log-view-refactor.plan.md`](./log-view-refactor.plan.md). That plan drove extraction of CSS, a module folder, `derive/*`, and goja-based tests. **Most checklist items there are marked done**, but the UI is still hard to change safely because the **application shell** (`embedui/logs.js`, served as `/ui/assets/logs/main.js`) remains a large closed-over state machine. This plan focuses on **clarity** (where to edit what), **testability** (what to test without a browser), and **safe incremental refactors**.

---

## 1. Goals

| Goal | Outcome |
|------|--------|
| **Clarity** | A new contributor (human or agent) can find the right file in one step; URL ↔ disk mapping is obvious; view-mode and data-flow are documented. |
| **Testability** | Pure logic has unit tests; integration boundaries (fetch/SSE/DOM) are injectable or mockable; regressions are caught in `go test`. |
| **Change safety** | Small PRs; behavior preserved unless explicitly changed; manual checklist + automated tests per PR. |

### Non-goals (for this plan)

- Rewriting the UI in React/Vue/Svelte.
- Mandating Node/npm for the default developer loop (Go + optional tooling only unless we explicitly opt in).
- Changing public API contracts for `/api/ui/logs`, `/api/ui/logs/stream`, `/api/ui/metrics`, `/api/ui/tokens` without a dedicated “API change” phase and tests.

---

## 2. Current state (honest baseline)

**What works well**

- `embedui/logs/` holds **pure-ish** utilities: `util/*`, `parse/*`, `filters/*`, `derive/*`, `components/*`, parts of `render/*`, `transport/streaming.js`.
- **Goja tests** in `internal/server/logs_components_test.go` cover components and many `derive/*` functions.
- **HTML** is a thin shell; **CSS** is external `logs.css`.

**What still hurts**

1. **`embedui/logs.js` (~2.7k+ lines)** — Owns view-mode switching, summarized panel rebuild, structured table rows, metrics card HTML, focus/deep-link behavior, and wiring into `transport/streaming.js` via a large implicit `ctx`. Hard to test as a unit; easy to break edge cases.
2. **Confusing asset names** — On disk: `logs_bootstrap.js` → URL `/ui/assets/logs.js`; on disk: `logs.js` → URL `/ui/assets/logs/main.js`. Easy to edit the wrong file or search wrong symbols.
3. **`models.js` exists but is not loaded** by `logs.html` — Typedefs drift from reality; agents do not discover them.
4. **CSS** — Likely single flat file without section banners; selectors may not mirror DOM regions → hard to refactor layout safely.
5. **Transport vs. orchestration** — SSE/poll live in `transport/streaming.js` but lifecycle and “what to do with each entry” mostly live in the monolith; tests rarely span that boundary.

---

## 3. Design principles (for the refactor)

1. **Pure core, impure edges** — JSON parse, derivation, formatting, filtering = pure functions tested with goja. `fetch`, `EventSource`, `document`, timers = thin adapters behind a small facade.
2. **Explicit app context** — Replace the implicit mega-`ctx` with a documented object (JSDoc in a loaded file): refs, caches, sequence state, injected `deps: { fetch, EventSource, now }`.
3. **One feature → one folder** — e.g. `summarized/`, `structuredTable/`, `rawLogs/`, `metricsCards/` under `embedui/logs/`, each with `*.js` + optional `*_test`-via-goja fixtures.
4. **Stable DOM contract** — `logs.html` uses clear `id` / `data-testid` (where useful) regions documented in README; breaking selectors requires intentional change.
5. **PR-sized steps** — No big-bang; each PR moves code or adds tests without mixing large behavior changes.

---

## 4. Workstreams and phases

### Workstream A — Documentation and naming (low risk, high leverage)

**A1 — Developer map (required)**  
Add `internal/server/embedui/logs/README.md` containing:

- Table: **URL path** → **embed path** (`/ui/assets/logs/main.js` → `embedui/logs.js`, etc.).
- List of **HTTP APIs** + one **example JSON** line per endpoint (reuse shapes from `servicelogs.Entry`: `seq`, `source`, `text`, `ts`).
- **View modes** ↔ panels ↔ primary owner file (as known today; update when splits land).
- “**If you change X, also check Y**” (e.g. SSE frame shape ↔ `transport/streaming.js` ↔ `appendLine` path).

**A2 — Rename for honesty (recommended)**  
In one small PR:

- Rename `embedui/logs.js` → `embedui/logs_main.js` (or `embedui/logs/app.js`).
- Rename `embedui/logs_bootstrap.js` → `embedui/logs_entry.js` (or keep bootstrap name but align comment in `ui_handlers.go`).
- Update `//go:embed` and mux routes so **file basename matches role**.

Optional: symlink or duplicate URL for one release **only if** external tools hard-coded paths (unlikely for embedded admin UI).

**A3 — Load or import `models.js`**  
Either:

- Add `<script src=".../models.js" defer></script>` before `main`, **or**
- Move typedefs into a file that is always evaluated first and referenced from README.

---

### Workstream B — HTML clarity

**B1 — Semantic regions**  
Wrap major sections in `<section>` with stable `id` / `aria-labelledby` matching the view-mode doc:

- Status line, classic table panel, summarized panel, raw logs panel, filters, toolbar.

**B2 — Test hooks (optional but useful)**  
Add `data-testid` on: `#view-mode`, `#log-body`, `#panel-summarized`, `#raw-logs-textarea`, status container. Keeps future browser tests stable; no visual change.

**B3 — Script loading strategy (decision point)**  
**Short term:** keep ordered `defer` list; add HTML comment block mirroring the exact order and “do not reorder without …”.  
**Medium term:** switch to **native ES modules** (`type="module"`) *or* one **esbuild** bundle for `logs` only — eliminates order bugs and clarifies imports (see Workstream D).

---

### Workstream C — CSS clarity

**C1 — Section the file**  
Split `logs.css` into logical sections with banner comments:

- Base / tokens, layout, classic table, summarized cards, raw logs / toolbar, embedded overrides, motion/print optional.

**(Optional later)** Split into `logs/base.css`, `logs/summarized.css` — only if we add a trivial concat step or bundler; avoid breaking embed path count without tooling.

**C2 — Pair with DOM map**  
In README, list “DOM region → CSS section anchor” so agents do not grep-blind.

---

### Workstream D — JavaScript architecture (main effort)

**D1 — Catalog and freeze public surface of modules**  
For each file under `embedui/logs/**`, document:

- Exports on `globalThis.ChimeraLogs.*` vs internal IIFE-only.

Goal: reduce duplicate “helper” functions in `logs_main.js`.

**D2 — Extract orchestration slices from `logs_main.js`** (multiple PRs)

Suggested order (dependencies first):

1. **View mode + layout** — `commitViewMode`, `applyViewLayout`, URL sync → `settings/viewMode.js`.
2. **Entry pipeline** — single function `handleLogEntry(deps, state, rawEntry)` that: parse → filter → dedupe → route to summarized vs table vs textarea. Tests: fixture entries in/out.
3. **Summarized feed** — rebuild scheduling, card assembly → `render/summarizedFeed.js` + `summarized/cards/*.js` as needed.
4. **Structured table** — row builders → `render/structuredTable.js`.
5. **Metrics polling** — `fetchGatewayMetrics`, polling timer → `transport/metricsPoll.js` with injectable `fetch`.

After each extraction: **no behavior change**; move-only + wire-up.

**D3 — Formal `deps` injection**  
`streaming.js` and future poll modules accept `deps.fetch`, `deps.EventSource`, `deps.setInterval`/`clearInterval` so goja tests can stub network without Browser.

**D4 — Optional toolchain (explicit decision)**  

| Option | Pros | Cons |
|--------|------|------|
| **Native ES modules** | No bundle step; explicit imports | Must update embed MIME + script tags; goja may need path resolution |
| **esbuild (one binary / `make` target)** | Single file to embed; clean imports | Adds dev dependency invocation in CI/Makefile |

Recommendation: finish **D2** with current global pattern if cost is lower; introduce **esbuild** only when `defer` ordering becomes painful again.

---

### Workstream E — Testing strategy

**E1 — Expand goja coverage (near-term)**

- Any new pure module from D2 gets **fixture-based tests**: input JSON arrays → snapshot or structural assertions on HTML string or intermediate model.
- Add tests for `normalizeViewMode` / URL persistence if extracted.
- Add tests for **dedupe / seq ordering** logic if lifted out of the monolith.

**E2 — “Golden” derivation tests**  
For `derive/*`, keep asserting stable numeric/string outputs; avoid full HTML snapshots for giant cards unless chunked.

**E3 — Go-level UI shell tests (existing pattern)**  
Continue/extend tests that assert `logs.html` contains required script URLs and ids (already noted in refactor plan).

**E4 — Optional browser smoke (later)**  
If/when CI has Chromium: a single Playwright script that opens authenticated `/ui/logs`, switches views, asserts panel visibility. Out of scope until D/B stabilize DOM hooks.

---

### Workstream F — Server alignment

**F1 — Document `servicelogs.Entry` and UI expectations**  
Short comment in `ui_logs.go` or README linking `Entry` fields to client parsing assumptions.

**F2 — Cors / auth / cache headers** — Leave as-is unless a test requires `Cache-Control: no-store` assertions (already used for JS).

---

## 5. Suggested PR sequence (agent-sized)

| # | Title | Contents |
|---|--------|----------|
| 1 | `ui/logs: agent README + script order comment` | README, HTML comment, link from main docs index if desired |
| 2 | `ui/logs: rename embed files to match URLs` | Rename + go:embed + mux only |
| 3 | `ui/logs: load models.js + JSDoc app state` | `models.js` + typedef for app state/deps |
| 4 | `ui/logs: section logs.css + HTML regions` | Comments + `<section>` wrappers |
| 5 | `ui/logs: extract viewMode module` | Move-only + goja tests for normalization |
| 6 | `ui/logs: extract entry pipeline` | Pure function + fixtures + wire-up |
| 7+ | `ui/logs: extract summarized / table / metrics poll` | One concern per PR |

---

## 6. Definition of done (for this maintainability push)

- [ ] README in `embedui/logs/` is the **first** link for logs UI work.
- [ ] On-disk **main bundle** name matches its **role** (no `logs.js` vs `main.js` confusion).
- [ ] **`logs_main.js` (or equivalent) &lt; ~800 lines** OR explicitly split into named modules with no single file &gt; 1k lines.
- [ ] `models.js` (or successor) loaded; **app state + deps** typedef documented.
- [ ] Every `derive/*` and **new pure module** has goja coverage or a documented exception.
- [ ] Manual checklist from [`log-view-refactor.plan.md`](./log-view-refactor.plan.md) run on the last merging PR before closing the epic.

---

## 7. References

- Historical phases + manual checklist: [`log-view-refactor.plan.md`](./log-view-refactor.plan.md)
- Component/derive tests: `internal/server/logs_components_test.go`
- API tests: `internal/server/ui_logs_test.go`, `internal/server/ui_metrics_test.go`
- Asset registration: `internal/server/ui_handlers.go` (`serveLogsModuleAsset`, `/ui/assets/logs*`)
