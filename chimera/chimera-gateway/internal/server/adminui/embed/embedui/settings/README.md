# Operator settings modules (`embedui/settings/`)

JavaScript for **`GET /ui/settings`** (`settings.html`). Namespace: **`globalThis.ChimeraSettings`**.

## Load order

See `settings.html`: `contracts.js` → `ChimeraUI` (`ui/`) → util/parse → derive → components → handlers → `settings_app.js` (as `/ui/assets/settings/main.js`) → `settings_entry.js` (as `/ui/assets/settings.js`).

## Module map

| Directory | Responsibility |
|-----------|----------------|
| `app/` | Feed orchestration (`summarizedFeed.js`), handler wiring, dirty routing |
| `derive/` | Pure metrics/card models from parsed log lines |
| `handlers/` | DOM event handlers (admin saves, chrome, evlog panel) |
| `render/` | HTML builders; `render/cards/` registers card types on `ChimeraSettings.Render.Cards` |
| `summarized/` | Diff/patch model for incremental card updates |
| `transport/` | Log poll/SSE (`/api/ui/logs`, `/api/ui/logs/stream`) |
| `parse/` | Log line text → flat field map |
| `filters/` | Client-side filter state |
| `components/` | Settings-local widgets (also uses `ui/components/`) |
| `util/` | hash, time, escape re-exports |
| `testing/` | Test harness loader (not used in production HTML) |
| `contracts.js` | Generated constants (products, timeline kinds) |
| `operator_copy.js` | Generated operator message registry |

## Related routes

| Route | File |
|-------|------|
| `GET /ui/settings/gallery` | `../settings/gallery.html` (assets in `../gallery/`) |

**APIs (unchanged):** `GET /api/ui/logs`, `GET /api/ui/logs/stream`, `GET /api/ui/state`, `GET /api/ui/metrics`, indexer/routing save endpoints, etc.

## Maintainer scripts

- `../scripts/extract-cards-phase3.py` — card extraction from `app/summarizedFeed.js` (paths under `settings/`).
