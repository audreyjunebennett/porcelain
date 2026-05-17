# Security notes — chimera-gateway (Go)

Operator-focused summary of how the `chimera` binary handles secrets, exposure, and subprocesses. **Not** a formal audit or penetration test.

## Secrets and logging

- **Client gateway tokens** (`Authorization: Bearer …` on `/v1/*`) are **not** logged in full. Access logs use a **redacted** header value (`Bearer` + first four characters + `…` when the token is long, `Bearer ***` when short). See `internal/server/server.go` (`redactAuth`, `loggingMiddleware`).
- **Upstream API keys** (env named by `upstream.api_key_env`, usually `CHIMERA_UPSTREAM_API_KEY`, or inline `upstream.api_key` in YAML) are **not** written to logs when used for outbound calls. Debug logs may include **upstream base URL** and paths, not key material.
- `tokens.yaml` reload logs **path** and **token count**, not token strings (`internal/tokens/tokens.go`).
- **Chat completion** logs (info level) include `clientModel`, `stream`, and `tenant` (from the validated gateway token), **not** full message bodies.

Treat logs as sensitive metadata (tenants, models, URLs) and protect retention and access like any service log.

## Network exposure

- **Default shipped config** (`config/gateway.example.yaml`) listens on `0.0.0.0:3000` (`gateway.listen_host` / `listen_port`). Anything that can reach that address can call `/v1/*` with a valid gateway token and can read `/health` without auth. Prefer `127.0.0.1` (or host firewall / reverse proxy + TLS) when the gateway must not be reachable from other machines.
- `GET /status` is **unauthenticated** (same idea as `/health`) and returns JSON: effective listen address, upstream base URL, upstream probe result, and optional supervisor hints (BiFrost / Qdrant) when `chimera serve` is in use. Do not expose the gateway port to untrusted networks if that metadata matters.
- **TLS termination** is **not** implemented inside `chimera`. Use a reverse proxy or sidecar for HTTPS in production-style deployments.

## First-run bootstrap (no `tokens.yaml` yet)

When **no valid gateway tokens** are configured, the process serves a **narrow bootstrap** HTTP surface (redirect to `/ui/setup`, `POST /api/ui/setup/token`, `/health`, `/status`) on **loopback-only** listeners (`127.0.0.1` and `[::1]` on the chosen port). That limits who can create the first `tokens.yaml` from the network. After at least one token exists and the gateway is restarted, the **normal** listener and full router apply (including your configured `listen_host`).

## Operator UI (`/ui/*`, `/api/ui/*`)

- **Login** validates a **gateway token** from `tokens.yaml` and sets an **HttpOnly** session cookie (`SameSite=Lax`, 24h). Session state is **in-memory** on the server.
- **Authenticated UI JSON APIs** change provider keys, tokens, routing files, etc. There is **no separate CSRF token**; `SameSite=Lax` reduces some cross-site risks but is **not** a substitute for binding the admin UI to **localhost** or **TLS + strict same-site** policies when exposed beyond a trusted operator machine.
- **Embedded static UI** is served from the binary; follow usual browser hygiene for XSS (the app does not inject untrusted HTML from users into the panel).

## Config on disk

`gateway.yaml`, `tokens.yaml`, `routing-policy.yaml`, `provider-free-tier.yaml`, and `bifrost.config.json` can hold secrets and policy. Use filesystem permissions (e.g. `chmod 600`) and never commit real `tokens.yaml` or live keys.

## `chimera serve` (supervisor)

- Spawns **BiFrost** and optionally **Qdrant** as children. The effective user can **execute** whatever `-bifrost-bin` / `-qdrant-bin` point to; use trusted binaries and paths.
- Environment is **inherited** from the parent; do not run the supervisor with untrusted env.
- BiFrost **config** is materialized under the data directory; keep `bifrost.config.json` permissions tight on disk.

## Reporting

If you find a security issue in this repository, **report it privately** to the maintainer (e.g. GitHub Security Advisories / maintainer contact on the repo) rather than a public issue, so fixes can ship before wide disclosure.
