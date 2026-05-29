# Feature: Operator UI filesystem dev mode

| Field | Value |
|-------|-------|
| **Doc kind** | `platform-contract` |
| **Areas** | Gateway embed UI assets, local developer workflow |
| **Status** | `current` |
| **Introduced** | Gateway / operator UI v0.3 |
| **Originated from** | [`plans/adminui-filesystem-dev-mode.md`](../plans/adminui-filesystem-dev-mode.md) |
| **Related features** | [Operator settings UI](operator-settings-ui.md), [Locus desktop ↔ supervisor](locus-desktop-supervisor.md) |
| **Depends on** | Loopback gateway listen addr |
| **Last updated** | See git history |

## At a glance

Developers can serve operator UI static assets from disk instead of the compiled `//go:embed` bundle by setting **`CHIMERA_ADMINUI_ROOT`** to the directory containing `embedui/` (typically `chimera-gateway/internal/server/adminui/embed`). Edit HTML, CSS, or JS under `embedui/`, refresh the browser — no `make chimera-gateway-build` for static changes. Go handlers, generated contracts, and API behavior still require a gateway rebuild. Disk mode is allowed only when the gateway listens on **loopback**.

## Operator-visible behavior

- With env unset — production behavior: embedded assets only.
- With env set + loopback listen — same `/ui/*` and `/ui/assets/*` URLs serve on-disk bytes; structured log `gateway.startup.adminui_filesystem` records the root path.
- With env set but non-loopback listen — env is **ignored**; embedded FS used (security guard).

## System behavior and contracts

**Invariants**

- Env key: `CHIMERA_ADMINUI_ROOT` (`internal/naming.EnvAdminUIRoot`).
- Valid root must contain `embedui/settings.html` (or equivalent layout check in resolver).
- `embed.ReadFile`, `AssetsFromDisk`, `DiskRoot` reflect active source; cached until env or listen addr changes.
- Supervisor/desktop pass parent env to gateway child — no extra supervisor flags required.
- Path traversal rules unchanged in `ServePathPrefix`.

**Still requires rebuild**

- Go changes in `internal/server/adminui/` handlers
- Generated JS (`operator_copy.js`, `contracts.js`, etc.)
- New embed patterns in `//go:embed` directive

## Interfaces

| Surface | Detail |
|---------|--------|
| Env | `CHIMERA_ADMINUI_ROOT=/path/to/adminui/embed` |
| Listen gate | `SetGatewayListenAddr` — disk mode requires loopback |
| Make helper | See `Makefile` `chimera-supervisor-dev-ui` / `locus-desktop-dev-ui` targets |

## Code map

| Concern | Location |
|---------|----------|
| Asset resolution | `internal/server/adminui/embed/assets.go` |
| Static serving | `internal/server/adminui/embed/` routes |
| Env constant | `internal/naming/contracts.go` |
| Dev README | `embed/embedui/settings/README.md` |

## Verification

```bash
go test ./chimera/chimera-gateway/internal/server/adminui/embed/...
```

Manual: set env, edit a CSS file, refresh `/ui/settings` without rebuilding gateway.

## Out of scope and known gaps

- Hot reload / file watcher — browser refresh only.
- Component gallery — still static HTML under `docs/component-gallery/` for CSS-only iteration.

## References

- Delivery plan: [`adminui-filesystem-dev-mode.md`](../plans/adminui-filesystem-dev-mode.md)
