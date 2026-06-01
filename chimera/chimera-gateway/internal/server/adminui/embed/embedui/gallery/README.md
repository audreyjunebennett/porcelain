# Component gallery assets (`embedui/gallery/`)

Static CSS/JS for the operator component gallery. The gallery **page** is `embedui/settings/gallery.html` at **`GET /ui/settings/gallery`**.

Do not add duplicate assets under `settings/gallery/` — this directory is the only source for `/ui/assets/gallery/*`. YAML revert buttons use Material Symbols `refresh` (see `settings/render/cards/admin*.js`).

## Served paths

| URL | Source |
|-----|--------|
| `/ui/assets/gallery/gallery-shell.css` | `embedui/gallery/gallery-shell.css` |
| `/ui/assets/gallery/gallery-evlog-markup.js` | `embedui/gallery/gallery-evlog-markup.js` |
| `/ui/assets/gallery/gallery-*.js` | `embedui/gallery/gallery-*.js` |
| `/ui/assets/gallery/gallery-card-fixtures.js` | Renders production card HTML into `#gallery-fixture-*` mount points |
| `/ui/assets/gallery/gallery-parts.js` | “Show part names” toggle + `?parts=1` (sets `body.gallery-show-parts`) |

**Card parity:** `gallery-card-fixtures.js` loads the same `settings/render/cards/*` and `shared/*` modules as `/ui/settings`, then fills fixture containers for gateway overview, providers, workspace draft, managed workspace, virtual model (per-VM routing), and indexer-stale cards.

**Part overlay:** Enable **Show part names** on the gallery toolbar (or open with `?parts=1`). Labels come from production `data-ui-part` attributes documented in [`../settings/card-parts-registry.md`](../settings/card-parts-registry.md). Overlay styles live in `gallery-shell.css` only.

Edit under `embedui/gallery/`, then refresh `/ui/settings/gallery`. With **`CHIMERA_ADMINUI_ROOT`** set to `adminui/embed`, changes apply without rebuilding the gateway.

**Mobile layout review:** use Chrome DevTools device preset **iPhone 12 (390×844)** on `/ui/settings/gallery` — scoped logs should match production two-column markup (`data-sum-evlog-cols="2"`, inline source/status in `.sum-evlog__msg-meta`). Shared row builders live in `gallery-evlog-markup.js`.

## Related

- Settings module map: [`embedui/settings/README.md`](../settings/README.md)
- CI path check: `scripts/check-component-gallery-paths.ps1`
