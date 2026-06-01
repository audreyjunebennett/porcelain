# Operator embed UI (`embedui/`)

Static HTML/CSS/JS served by the gateway admin UI (`adminui/embed`). Session auth applies to all routes except login/setup and shared primitive assets.

## Pages

| HTTP route | File | Role |
|------------|------|------|
| `GET /ui` | `index.html` | App shell (iframe: PWA default, settings via gear) |
| `GET /ui/pwa` | `pwa.html` | PWA placeholder |
| `GET /ui/settings` | `settings.html` | Operator settings + summarized log feed |
| `GET /ui/settings/gallery` | `settings/gallery.html` | Component gallery (styleguide) |
| `GET /ui/login` | `login.html` | Login (registered in `api/auth`) |
| `GET /ui/setup` | `setup.html` | First-run setup (bootstrap mux) |

Legacy routes (`/ui/logs`, `/ui/desktop`, `/ui/gallery`, …) are **not** registered.

## Asset URL map

| URL prefix | Directory | Notes |
|------------|-----------|--------|
| `/ui/assets/settings.css` | `settings.css` | Style composition entry |
| `/ui/assets/settings.js` | `settings_entry.js` | Boots `ChimeraSettings.Main()` |
| `/ui/assets/settings/main.js` | `settings_app.js` | Main app IIFE |
| `/ui/assets/settings/**` | `settings/**` | Modules, generated `contracts.js` / `operator_copy.js` |
| `/ui/assets/gallery/**` | `gallery/**` | Gallery-only CSS/JS (not under `settings/`) |
| `/ui/assets/styles/**` | `styles/**` | Shared layout tokens used by settings + gallery |
| `/ui/assets/ui/**` | `ui/**` | `ChimeraUI` primitives |
| `/ui/assets/shared/**` | `shared/**` | `ChimeraShared` admin primitives (settings + wizard) |
| `/ui/assets/theme-tokens.css` | `theme-tokens.css` | Design tokens |
| `/ui/assets/ui.css` | `ui.css` | Shared primitives (login/setup too) |

## JavaScript layout

- **`globalThis.ChimeraSettings`** — settings app (`settings/` modules). Log stream uses **`/api/ui/logs`** (API name unchanged).
- **`globalThis.ChimeraUI`** — shared presentation components (`ui/components/`).
- **`globalThis.ChimeraShared`** — operator admin primitives (`shared/`; credentials, status, configure, scoped evlog).
- **Codegen:** `settings/contracts.js` ← `go run ./internal/naming/cmd/gencontracts`; `settings/operator_copy.js` ← `go run ./internal/operatorcopy/cmd/genjs`.

## Directory guide

```
embedui/
  index.html, pwa.html, settings.html    # top-level pages
  settings_entry.js, settings_app.js     # served as settings.js + settings/main.js
  settings.css                           # @imports styles/*
  settings/                              # ChimeraSettings modules (see settings/README.md)
  gallery/                               # gallery static assets only
  settings/gallery.html                  # gallery page HTML
  styles/                                # CSS building blocks
  shared/                                # ChimeraShared (see shared/README.md)
  ui/                                    # ChimeraUI components
  scripts/                               # maintainer tools (not served)
```

## Local iteration

Set `CHIMERA_ADMINUI_ROOT` to `chimera/chimera-gateway/internal/server/adminui/embed` (loopback listen only) to serve files from disk without rebuilding.
