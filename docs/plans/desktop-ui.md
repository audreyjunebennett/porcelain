# Plan: Chimera desktop UI (webview) + gateway admin surface

| Field | Value |
|-------|-------|
| **Doc kind** | `feature-plan` |
| **Owners / areas** | Desktop webview, Gateway admin UI |
| **Status** | `done` |
| **Targets** | Desktop UI phase 1 and phase 2 configuration |
| **Last updated** | See git history |
| **Supersedes / superseded by** | Supersedes the legacy Fyne GUI direction |

## At a glance

Give operators a single desktop window that opens straight to the Chimera control panel: sign in, see provider keys, edit them inline, and copy a ready-to-paste VS Code Continue snippet. The same UI works in a normal browser, and the same binary still runs headless on a server.

| Phase | Outcome | Status |
|-------|---------|--------|
| [Phase 1 — Gateway-served operator UI](#phase-1--webview-wrapper--gateway-admin-ui) | Login, control panel with provider rows, Continue snippet | `done` |
| [Phase 1 — Desktop shell & lifecycle](#desktop-launcher-bundled-release-and-lifecycle) | One binary opens a native window; clean shutdown shared with signals | `done` |
| [Phase 1 — Repo cleanup & checklist](#implementation-checklist-phase-1) | Old Fyne app removed; Makefile, scripts, and README aligned | `done` |
| [Phase 2 — Saved settings & polish](#phase-2) | Saved gateway URL, deep links, richer observability UI, PWA | `done` |

---

This document plans a **cross-platform desktop shell** that wraps a **system webview** and loads **operator UI served by the chimera-gateway**. The goal is **one** web-based control experience shared with the browser (and eventually a PWA). **Version 0.1 removes** the legacy **Fyne** desktop app ([`gui/`](../gui/)) entirely; the webview shell does not use Fyne and is the only desktop UI.

**Implementation direction:** work toward **one primary executable** (`chimera` / `chimera.exe`) that the user launches in **desktop mode**: it starts the **supervised stack** (optional Qdrant, BiFrost, and the **HTTP gateway** in-process, per [`supervisor.md`](../supervisor.md)), then opens the **webview** against the gateway’s `/ui/…` entry. The same binary also supports **headless** operation (no webview) for servers, automation, and future **platform installers** that install a bundle without a desktop shell.

**Related docs:** [`cli-tool.plan.md`](cli-tool.plan.md) (operator CLI, shared BiFrost assumptions), [`supervisor.md`](../supervisor.md), [`bifrost-discovery.md`](../bifrost-discovery.md), [`configuration.md`](../configuration.md), [`vscode-continue/`](../vscode-continue/) (Continue examples).

---

## Versioning

### Phase 2 — Webview wrapper + gateway admin UI

**Desktop shell (webview)**

- Embeds a **native webview** (platform WebView2 / WKWebView / WebKitGTK, or a small helper such as Wails/Tauri if the team standardizes on one). **No Fyne** — CGO is only required if the chosen webview stack requires it (unlike the old Fyne GUI).
- **Build entry (target):** integrate the webview into **[`cmd/chimera`](../cmd/chimera)** so **one binary** runs **desktop mode** (supervisor + gateway + window). A **temporary** separate package (e.g. [`cmd/chimera-gui`](../cmd/chimera-gui)) is acceptable only if it accelerates early integration; the **deliverable** to optimize for is **single `chimera`**. Makefile / script names such as `make gui-build` may continue to produce a `chimera-gui` artifact during transition, or may be retargeted to the desktop-capable `chimera` build once merged — document whichever layout the repo uses after the cutover.
- **Remove in Phase 1:** delete the Fyne [`gui/`](../gui/) module; retarget [`scripts/gui-build.sh`](../scripts/gui-build.sh), [`scripts/gui-install.sh`](../scripts/gui-install.sh), and [`scripts/gui-run.sh`](../scripts/gui-run.sh) at the **webview-capable** build; update [`Makefile`](../Makefile) `vet-gui` / `test-gui` / `fmt` paths and drop `CGO_ENABLED=1` unless the webview stack needs CGO; update [`scripts/clean.sh`](../scripts/clean.sh), [`scripts/print-make-help.sh`](../scripts/print-make-help.sh), [`docs/gui-testing.md`](../docs/gui-testing.md), README, and CI (e.g. `.github/workflows`) so they describe webview deps, not Fyne.
- **Default navigation target:** chimera-gateway **operator entry** served by `chimera` (e.g. `http://127.0.0.1:3000/ui/desktop` with a single iframe; settings opens `/ui/logs?embed=1` — **must** be pages shipped **from the gateway**, not only bundled inside the wrapper). The legacy multi-tab shell (Main / Logs / Admin) is retired; admin and metrics live in `/ui/logs` cards.
- **Static assets bundled with the wrapper** (not the gateway): a **gateway unreachable** page (HTML/CSS) shown when the wrapper cannot connect to the configured base URL (connection refused, timeout, DNS failure). No token or secrets on that page.
- **Phase 1 default base URL:** `http://127.0.0.1:3000` (hard-coded or single compile-time default; configurable persistence is **Phase 8**).

**Gateway-served UI (same origin as gateway)**

1. **Default / landing** — First paint from Chimera: welcome or redirect into the login flow.
2. **Login** — User enters the **gateway token** (same class of secret as `Authorization: Bearer` on `/v1/*`, or a dedicated **admin** token if split in implementation; Phase 1 must document which). Submission **authenticates** the session for admin UI routes only.
3. **Authentication model (Phase 1)** — After successful login, use a **session the browser/webview can reuse** without putting the token in the URL:
   - Preferred: `POST /api/ui/login` (name illustrative) validates token against the existing token store ([`config/tokens.yaml`](../config/tokens.yaml) / gateway auth), then responds with `Set-Cookie`: **httpOnly**, **SameSite=Lax**, path scoped to `/ui` and `/api/ui` (or equivalent).
   - Subsequent `fetch()` from the control panel sends the cookie automatically inside the webview.
   - **401** on any admin call → return to login; clear stale session.
4. **Control panel** — Single page (or small multi-step) that:
   - **Displays current values** for BiFrost **Groq**, **Gemini**, and **Ollama** (as surfaced by the gateway from BiFrost’s management API — key metadata **masked**, Ollama base URL as plain text).
   - **Edits per row** — One row (or card) per concern: Groq API key, Gemini API key, Ollama base URL. User saves **one row at a time** (explicit Save per row); avoids losing half-completed multi-field forms.
   - **Inline errors** — Each row shows validation/API errors **next to that row** (HTTP 4xx/5xx from BiFrost or gateway BFF mapped to readable text). No silent failure.
5. **VS Code Continue snippet** — On the control panel (or a dedicated subsection), show a **copy-ready** configuration block: gateway **base URL**, **Bearer token** placeholder or instructions to paste the user’s token, and **model id** guidance aligned with [`vscode-continue/`](../vscode-continue/) (e.g. virtual `chimera-<semver>` from [`config/gateway.yaml`](../config/gateway.yaml)). User copies into Continue `config.json` / YAML.

**BiFrost prerequisite (phase 1)**

- [`config/bifrost.config.json`](../config/bifrost.config.json) **must** ship with **`config_store` enabled** so management APIs persist and return consistent state for the control panel. Align with [`cli-tool.plan.md`](cli-tool.plan.md) § BiFrost API + config store.

**Gateway backend essentials (phase 1)** — implied by the UI above; all in scope for 0.1:

- **BFF (server-side)** from gateway to BiFrost management HTTP API (`/api/providers/...` per pinned `chimera/deps.lock` / OpenAPI). Browser **never** calls BiFrost directly (avoids CORS, hides BiFrost admin auth if enabled later).
- **Read path:** aggregate **Groq keys**, **Gemini keys**, **Ollama** URL (or key config) for display (masked secrets).
- **Write path:** update or create keys / Ollama URL per row; map errors to JSON the UI can show inline.
- **Session/login** and **authorization middleware** for `/ui/*` HTML and `/api/ui/*` JSON (cookie session tied to validated gateway token).

**Out of scope for phase 1** (see **Version 0.8**):

- User-configurable gateway URL persisted in the wrapper (beyond default).
- PWA manifest / service worker.
- Multi-user RBAC, audit log UI, non-localhost hardening beyond documenting HTTPS for remote deploy.

### Phase 2

Everything **not** required to satisfy phase 1 above, including but not limited to:

- Saved **gateway base URL** (and optional port) in wrapper config; optional **profiles** (dev/prod).
- **Deep links**, **offline** PWA behavior, **installable** manifest served from gateway.
- Richer **observability** UI (logs, metrics), **BiFrost dashboard** parity, additional providers beyond Groq/Gemini/Ollama.
- **Unified** styling system, **i18n**, accessibility audit beyond baseline.
- **Automated** E2E tests for webview + gateway (CI matrix).

---

## Desktop launcher, bundled release, and lifecycle

**Single executable, not a single file for the whole product**

- The **user-facing launcher** is **one** `chimera` binary that, in **desktop mode**, starts everything that belongs in-process and via the existing **supervisor** (optional **Qdrant** and **BiFrost** subprocesses, plus the **Go HTTP gateway**).
- A **release** for end users is still a **bundle**: that executable **plus** the other programs the supervisor runs (`bifrost-http`, optional **Qdrant** binary), **configuration** (`config/gateway.yaml`, tokens, `bifrost.config.json`, etc.), and **data directories** as documented in installation / [`supervisor.md`](../supervisor.md). Installers (future) ship this layout; nothing requires stuffing BiFrost or Qdrant *into* the same PE/ELF file.

**Headless vs desktop (same binary)**

- **Headless:** e.g. `chimera serve` with flags as today (or an explicit `--headless` / build tag that omits webview linkage for smaller CI and server artifacts). No window; shutdown is driven by **OS signals** only (unless extended later).
- **Desktop:** same binary opens the webview after (or while) the gateway is listening; default URL remains `http://127.0.0.1:3000` (or the resolved listen address) for `/ui/…`.

**Unified shutdown**

- Implement **one** internal shutdown path (e.g. cancel a **root context** and/or a dedicated `shutdown()` used everywhere). **Both** of the following must invoke it:
  - **OS signals** (**SIGINT**, **SIGTERM**) — same semantics as today’s `chimera serve`.
  - **Webview window close** (`OnClose` or the framework’s equivalent).
- **Order (conceptual):** graceful **HTTP server shutdown**, then **cancel supervisor child context** so **Qdrant** / **BiFrost** processes stop. Avoid duplicating teardown logic between signal handlers and UI callbacks.

**Backend failures vs. the shell**

- If **Qdrant** or **BiFrost** exits or never becomes healthy, the **desktop process** (and webview) **should keep running** so the operator can see **degraded state** (gateway `GET /status`, failure page, or in-app messaging). **User-driven quit** (close window or signal) still tears down the whole operation. Exact restart policy (auto-restart children vs. report-only) is implementation detail; phase 1 should at minimum **surface** failures without killing the window immediately.

---

## Architecture (phase 1)

**Logical components** (desktop mode): one OS process hosts the **webview** and the **gateway**; **BiFrost** and optional **Qdrant** remain **child processes** started by the supervisor.

```text
┌──────────────────────────────────────────────────────────────────────────────┐
│  Process: chimera (desktop mode)                                             │
│  ┌─────────────────────┐     HTTP (cookie session)      ┌───────────────────┐  │
│  │  Webview            │ ────────────────────────────► │  Gateway (in-proc)│  │
│  │  (bundled failure   │   GET /ui/…, POST /api/ui/…   │  HTML/JS + BFF    │  │
│  │   page only local)  │                               │  token store      │  │
│  └─────────────────────┘                               └─────────┬─────────┘  │
└──────────────────────────────────────────────────────────────────┼───────────┘
                                                                   │ exec / supervise
                                          ┌────────────────────────┴────────────────────────┐
                                          ▼                                                 ▼
                                 ┌─────────────────┐                               ┌─────────────────┐
                                 │  Qdrant (child) │                               │  BiFrost (child)│
                                 │  optional       │                               │  config_store   │
                                 └─────────────────┘                               └─────────────────┘
```

**Headless** omits the webview box; the gateway + supervisor layout matches [`supervisor.md`](../supervisor.md).

---

## Control panel UX (phase 1) — row model

| Row | Display | Action | Error surface |
|-----|---------|--------|----------------|
| **Groq** | Masked key fingerprint or “not set”; optional last-updated | Input + **Save** | Inline under row |
| **Gemini** | Same | Input + **Save** | Inline under row |
| **Ollama** | Current `base_url` | Input + **Save** | Inline under row |

Optional **Refresh** control to re-fetch state from BiFrost without full page reload.

---

## VS Code Continue (phase 1)

- **Static template** in the gateway UI with placeholders: `{gateway_url}`, `{token_hint}`, `{virtual_model_id}` filled from server-known values (semver/virtual model from runtime config).
- Link to repo [`vscode-continue/`](../vscode-continue/) for full examples.
- Warn: token in Continue config is **user-local**; do not commit.

---

## Security notes (phase 1)

- **No token in query strings** for navigation.
- **httpOnly** session cookie for admin UI; **CSRF** consideration for state-changing `POST`: use **SameSite**, or anti-CSRF token in form body / header for phase 1 if using cookie session.
- **localhost-only** by default; document that remote access requires **HTTPS** and tighter binding (`listen_host` in [`gateway.yaml`](../config/gateway.yaml)).

---

## Implementation checklist (phase 1)

**Status (repo as of this edit):** Gateway-served operator UI, session auth, BiFrost BFF, and embedded panel (`internal/server/embedui/`, wired from `internal/server/ui_handlers.go`) are **in place**. Desktop shell uses `github.com/webview/webview_go` behind `-tags desktop` (see `cmd/chimera/webview_desktop.go`, `cmd/chimera/serve.go`). **Still open** vs this plan: **bundled wrapper “gateway unreachable” HTML** (no static failure page in the webview layer yet). **Process model nuance:** shipping uses a **separate artifact name** `locus-desktop` (`make desktop-build`) for the CGO/webview build; default `./chimera` stays gateway-only without desktop tags.

**Gateway**

- [x] `config_store` in [`config/bifrost.config.json`](../config/bifrost.config.json) (enabled, SQLite) — aligns with admin persistence; cross-link remains in [`cli-tool.plan.md`](cli-tool.plan.md) / [`supervisor.md`](../supervisor.md) as needed.
- [x] **Admin session:** `POST /api/ui/login`, `POST /api/ui/logout`, httpOnly cookie, `requireAuth` for `/ui/*` pages and `/api/ui/*` JSON (plus optional `CHIMERA_GATEWAY_TOKEN` env skip on login).
- [x] **BFF:** read/write **Groq**, **Gemini**, **Ollama** via `internal/bifrostadmin` and routes under `/api/ui/provider/...` (and related UI state).
- [x] **Serve operator UI:** embedded `embedui/*.html` (login, panel, logs, metrics, shell, setup) registered on `/ui/...`.
- [x] **Control panel:** per-provider rows with **inline errors**, masked/fingerprint display, **Refresh**; additional sections (**gateway tokens**, **routing / tool-router**) beyond the minimal phase 1 row model.
- [x] **VS Code Continue** snippet block on the panel (gateway URL, token guidance, virtual model id).

**Desktop shell + launcher**

- [x] **Desktop mode:** `chimera desktop` (or desktop-tagged `chimera` with no subcommand) runs **supervisor + gateway** and opens the **webview** to `/ui/login?next=/ui/desktop` (or `/ui/setup` in bootstrap), using the **resolved listen URL** (not hard-coded when bound to a concrete address).
- [x] **Headless / no webview:** `chimera serve`, leading `--headless`, or a build **without** the `desktop` tag — no webview linked or opened.
- [x] **Unified shutdown:** `signal.NotifyContext` cancels the root context; webview `Terminate` runs when the context ends; `w.Run()` return invokes `stopRoot`; HTTP **Shutdown** then **supervisor child cancel** in `runServe`.
- [x] On **initial navigation / gateway unreachable** → bundled **static failure** page with retry/quit (not implemented in `webview_desktop.go` today — webview navigates straight to the entry URL).
- [x] On success → gateway `/ui/...` entry (`/ui/desktop` tabbed shell after login).

**Repo hygiene**

- [x] **Fyne `gui/` removed**; **Makefile** / scripts use `desktop-install`, `desktop-build`, `desktop-run`, `vet-desktop`, `test-desktop` (no `gui-*` targets).
- [x] **README** documents **desktop (webview)** vs **`chimera serve` / headless** installs and `make chimera-install` vs `make install`.

