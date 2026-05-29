# Feature: Operator UI session auth

| Field | Value |
|-------|-------|
| **Doc kind** | `platform-contract` |
| **Areas** | Gateway admin UI, `/ui/*`, `/api/ui/*`, session store |
| **Status** | `current` |
| **Introduced** | Operator settings/chat shell (v0.2+) |
| **Originated from** | [`plans/operator-conversation-history.md`](../plans/operator-conversation-history.md) Phase 1 |
| **Related features** | [Operator conversation history](operator-conversation-history.md), [Operator chat UI](operator-chat-ui.md), [Operator settings UI](operator-settings-ui.md) |
| **Depends on** | `api-keys.yaml` token validation |
| **Last updated** | See git history |

## At a glance

The operator UI is **not** authenticated by the gateway Bearer token on every browser request. After login, the gateway issues an in-memory **UI session** (HTTP-only cookie) bound to a durable **`principal_id`** (today: the `tenant_id` from the validated API key). All `/api/ui/*` JSON routes and protected `/ui/*` pages require that cookie. Chat persistence, conversation history, workspace CRUD, and settings APIs scope data by session principal — not by re-reading `api-keys.yaml` on each request.

## Operator-visible behavior

- **`/ui/login`** — Operator pastes a gateway API token; on success, cookie is set and browser redirects to `next` (default `/ui`).
- **Auto-login** — When `CHIMERA_GATEWAY_TOKEN` (or handler env login token) is set, GET `/ui/login` can establish a session without the form.
- **Unauthorized** — JSON APIs return `401`; HTML routes redirect to login with `?next=`.
- **Logout** — Session revoked; cookie cleared (where implemented in UI flow).

## System behavior and contracts

**Invariants**

- Cookie name default: `chimera_ui_session`.
- Session TTL default: **24 hours**; expired ids pruned on access.
- **`principal_id`** stored at `Issue()` time — rotating the API key does not orphan history if `tenant_id` in YAML is unchanged.
- **UI session ≠ chat Bearer token** — Browser chat uses tokens from `/api/ui/tokens`; persistence aligns when login token and chat token share the same tenant.
- **Bootstrap mode** — Most `/api/ui/*` mutating routes return 503 until first API key exists (see [bootstrap and API tokens](operator-bootstrap-and-api-tokens.md)).
- Sessions are **in-process only** — not persisted to SQLite; restart clears UI sessions (API keys remain in YAML).

**Decisions**

| Topic | Decision |
|-------|----------|
| Identity column | SQLite uses `principal_id`; value equals token `tenant_id` today |
| Auth wrappers | `RequireAuthJSON`, `RequireAuthPage` on handler |
| Session store | `session.Store` map in gateway memory |

## Interfaces

| Surface | Detail |
|---------|--------|
| `GET/POST /ui/login` | Login page + token POST |
| Cookie | `chimera_ui_session=<hex>` |
| Session helpers | `Handler.SessionOK`, `SessionPrincipal`, `SetSessionCookie` |
| Env auto-login | `CHIMERA_GATEWAY_TOKEN` via handler `EnvLoginToken()` |
| Protected routes | All `adminui` API registers except login/setup/bootstrap paths |

## Code map

| Concern | Location |
|---------|----------|
| Session store | `internal/server/adminui/session/session.go` |
| Handler auth | `internal/server/adminui/handler/handler.go` |
| Login routes | `internal/server/adminui/api/auth/` |
| Route registration | `internal/server/adminui/register.go` |

## Verification

```bash
go test ./chimera/chimera-gateway/internal/server/adminui/... -run -count=1
```

Manual: login at `/ui/login`, confirm `/api/ui/conversations` works without `Authorization` header but with cookie.

## Out of scope and known gaps

- OIDC / SSO — not implemented; YAML tokens only.
- Cross-process session sharing — single gateway process assumed.

## References

- Delivery plan: [`operator-conversation-history.md`](../plans/operator-conversation-history.md)
- Token source: [`operator-bootstrap-and-api-tokens.md`](operator-bootstrap-and-api-tokens.md)
