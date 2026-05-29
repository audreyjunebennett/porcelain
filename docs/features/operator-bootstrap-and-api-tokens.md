# Feature: Operator bootstrap and API tokens

| Field | Value |
|-------|-------|
| **Doc kind** | `feature-record` |
| **Areas** | Gateway bootstrap mode, `api-keys.yaml`, `/ui/setup`, token API |
| **Status** | `current` |
| **Introduced** | Gateway operator UI baseline |
| **Originated from** | Operator UI shell plans; token handlers in admin UI |
| **Related features** | [Operator UI session auth](operator-ui-session-auth.md), [Operator settings UI](operator-settings-ui.md), [Locus desktop ↔ supervisor](locus-desktop-supervisor.md) |
| **Depends on** | Writable `config/api-keys.yaml` |
| **Last updated** | See git history |

## At a glance

On first run when **no API keys exist**, the gateway enters **bootstrap mode**: a limited surface centered on **`/ui/setup`** for creating the first gateway token. Desktop redirects to setup when supervisor `/status` reports bootstrap. After keys exist, operators manage tokens from the settings **Users / tokens** card via `/api/ui/tokens` (list, create, delete). Tokens are stored in `api-keys.yaml`; each record includes `tenant_id` (used as UI `principal_id`) and a display label.

## Operator-visible behavior

- **Bootstrap** — `/ui/setup` wizard; mutating `/api/ui/*` routes return 503 with “use /ui/setup” until unlocked.
- **Settings tokens card** — Lists token metadata (not secrets); create shows one-time plaintext secret; delete removes a row.
- **Login** — Any valid token from the store establishes a UI session (see [session auth](operator-ui-session-auth.md)).
- **Chat / ingest** — Clients use Bearer tokens from the same store (`Authorization: Bearer …`).

## System behavior and contracts

**Invariants**

- Bootstrap detection: `tokens.IsBootstrapMode(api-keys path)` — empty or missing keys file.
- **`tenant_id`** on each token is the durable principal for SQLite scoping and conversation history.
- Token validation for UI login uses in-memory token store synced from YAML on `Runtime.Sync()`.
- Bootstrap listen may use dedicated loopback port helpers (`BootstrapListenPort`) for first-run UX.
- Provider model availability bootstrap runs on store open for known tenants (Groq/Gemini free-tier assist seed).

**Decisions**

| Topic | Decision |
|-------|----------|
| Storage | YAML file (`config/api-keys.yaml`), not SQLite |
| API shape | `operatorapi.TokensListResponse`, create/delete handlers |
| Desktop entry | `EntryURL` → `/ui/setup` when bootstrap flag in supervisor status |

## Interfaces

| Surface | Detail |
|---------|--------|
| `GET /api/ui/tokens` | List token metadata (session auth) |
| `POST /api/ui/tokens` | Create token; returns plaintext once |
| `DELETE /api/ui/tokens/{id}` | Remove token |
| HTML | `/ui/setup`, `/ui/login` |
| Config file | `config/api-keys.yaml` |
| Env | `CHIMERA_GATEWAY_TOKEN` for dev auto-login |

## Code map

| Concern | Location |
|---------|----------|
| Bootstrap mode | `internal/server/runtime/bootstrap.go` |
| Token handlers | `internal/server/adminui/api/tokens/` |
| YAML token I/O | `internal/tokens/` |
| Setup UI | `embed/embedui/setup.html` |
| Provider bootstrap | `internal/operatorstore/provider_models_bootstrap.go` |
| Desktop routing | `locus/locus-desktop/internal/supervisor/client.go` (`EntryURL`) |

## Verification

```bash
go test ./chimera/chimera-gateway/internal/server/adminui/api/tokens/...
go test ./internal/tokens/...
```

Manual: empty `api-keys.yaml`, open `/ui/setup`, create token, confirm settings APIs unlock.

## Out of scope and known gaps

- External identity providers — not supported.
- Token rotation preserving label without tenant change — manual YAML edit.

## References

- Configuration: [`configuration.md`](../configuration.md)
- Session binding: [`operator-ui-session-auth.md`](operator-ui-session-auth.md)
