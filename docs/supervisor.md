# BiFrost (+ optional Qdrant) subprocesses + Claudia (`claudia serve`)

Phase 3 of [go-bifrost-migration.plan.md](plans/go-bifrost-migration.plan.md): one command can start **Qdrant** (optional), **BiFrost**, and the **Go Claudia** HTTP server in the **same** parent process. **SIGINT** / **SIGTERM** triggers graceful HTTP **shutdown**, then **all** supervised children are **stopped** (reverse order is not guaranteed; context cancel ends them together).

## Runtime layout

| Piece | Role |
|-------|------|
| **Parent** | Go `claudia serve` — HTTP gateway (`config/gateway.yaml`, tokens, routing). |
| **Child (optional)** | **Qdrant** native binary — `QDRANT__STORAGE__STORAGE_PATH`, `QDRANT__SERVICE__HOST`, `QDRANT__SERVICE__HTTP_PORT`, `QDRANT__SERVICE__GRPC_PORT` (defaults **6333** / **6334**), plus `QDRANT__LOGGER__FORMAT=json` so subprocess logs are **one JSON object per line** (easier to pipe into the operator UI buffer). Before lines hit `servicelogs`, **`internal/servicelogs/qdrantline`** rewrites each complete line into gateway-style JSON with stable **`msg`** slugs (`qdrant.*`), **`service":"qdrant"`**, and structured fields (`collection`, `http_status`, …) — see [`docs/plans/log-qdrant.md`](plans/log-qdrant.md). Readiness: `GET /readyz`. Omit by leaving `-qdrant-bin` empty. When `rag.enabled` is **true**, the gateway uses Qdrant for RAG; supervise Qdrant for a full local stack. |
| **Child** | BiFrost HTTP binary (`bifrost-http`) — started with `-app-dir`, `-host`, `-port`, `-log-level`, `-log-style` (empty → **`json`**, one zerolog JSON object per line). `APP_HOST` / `APP_PORT` are also set for compatibility. Working directory = **data dir**. Before lines hit `servicelogs`, **`internal/servicelogs/bifrostline`** rewrites each complete stdout/stderr line into gateway-style JSON with stable **`msg`** slugs (`bifrost.*`), **`service":"bifrost"`**, and structured fields for the operator UI — see [`docs/plans/log-bifrost.md`](plans/log-bifrost.md). |
| **Child (optional)** | `claudia-index` — started when `indexer.supervised` is configured in `gateway.yaml` (see [indexer.md](indexer.md) supervised mode). Receives `CLAUDIA_GATEWAY_URL` and a merged `--config` file; stderr can be structured JSON (`--log-json`). |
| **Config copy** | Your `bifrost.config.json` is copied to `<bifrost-data-dir>/config.json` on each start. |

Claudia’s upstream URL is **overridden** to `http://<upstream-host>:<bifrost-port>` (default `http://127.0.0.1:8080`) so the running gateway always targets the supervised BiFrost instance.

The gateway exposes `GET /status` (JSON, no auth — same sensitivity as `/health`) with `supervisor.active: true`, BiFrost/Qdrant listen hints, and upstream probe results. The desktop shell and operators can poll this endpoint; see [gui-testing.md](gui-testing.md).

## Obtaining the BiFrost binary

The repository does **not** vendor BiFrost. Install per [BiFrost documentation](https://docs.getbifrost.ai/) (release binary, package, or build from [source](https://github.com/maximhq/bifrost)). You need the **HTTP server** artifact (`bifrost-http` from a source build’s `tmp/`), not only the CLI `bifrost`.

### Build from pinned sources (recommended)

**Install from `deps.lock`** (clone BiFrost + Qdrant source under `.deps/`, build BiFrost, fetch Qdrant release binary → `./bin/`):

```bash
make claudia-install
```

Use `make install` when you also want `make desktop-install` (desktop WebView/CGO OS deps). **Full onboarding** (install, build `claudia` + desktop, run desktop UI with supervisor):

```bash
make up
```

Foreground stack: `make claudia-serve` (gateway + BiFrost + Qdrant). Background: `make claudia-start` (after `make claudia-build`); logs in `logs/claudia.log`, PID in `run/claudia.pid`; stop with `make claudia-stop`, status with `make claudia-status`. For BiFrost only in the foreground, run `./claudia serve -bifrost-bin ./bin/bifrost-http` (no Qdrant).

Upstream BiFrost `make build` includes the UI (`build-ui`) and needs **Node.js 20+** and a matching **npm** (not only Go). `make claudia-install` checks Node before building.

Otherwise put `bifrost-http` (or a compatible binary) on `PATH` as `bifrost`, or pass `-bifrost-bin /full/path`.

### `fork/exec ./bin/bifrost-http: no such file or directory` (binary exists)

The kernel resolves a **relative** `-bifrost-bin` path against the **process current working directory**, not the repo root. If `claudia serve` starts with a different cwd (some IDE tasks, `go run` from another directory), `./bin/bifrost-http` misses. Claudia resolves `./…` and `bin/…` to an **absolute** path before exec; use `-bifrost-bin /home/you/src/claudia-gateway/bin/bifrost-http` if you still see issues, or run from the repo root.

### Troubleshooting `npm ci` / `Cannot read property '@base-ui/react' of undefined`

That error usually means `npm` is too old (e.g. **npm 6** with **Node 10**). On Ubuntu, **snap**’s `node` package is often **v10**; BiFrost’s UI expects a current **Node** (see BiFrost `ui/package.json` / Next 15). Fix by installing **Node 20+** (nvm, fnm, [nodejs.org](https://nodejs.org/), or your distro’s `nodejs` package) and ensuring `which node` points at it **before** snap’s `/snap/bin/node`. Then run `make claudia-install` again.

Provider keys (`GROQ_API_KEY`, `GEMINI_API_KEY`, etc.) are read from the **environment** of the `claudia serve` process and inherited by the BiFrost child. Qdrant inherits the same environment (optional `QDRANT__*` overrides).

## Qdrant binary

Supervision is for a full local stack; the gateway **calls Qdrant** when `rag.enabled` is **true** (**v0.2+**). Without RAG, Qdrant may still be supervised but the gateway does not require it.

- **Pinned version:** `QDRANT_RELEASE` in repo-root `deps.lock` (used by `scripts/qdrant-from-release.sh`, `scripts/release-snapshot-qdrant.sh`, and GoReleaser).
- **Local install:** `make claudia-install` (includes Qdrant) or `bash scripts/qdrant-from-release.sh` alone → `./bin/qdrant` or `qdrant.exe` (see `scripts/qdrant-from-release.sh`).
- **Full local stack (Qdrant + BiFrost + gateway):** `make up` or `make claudia-serve` (foreground).

### Qdrant startup log warnings (optional YAML and `./static`)

You do **not** need `config/config` or `config/development` (or other optional YAML) for the stack Claudia starts. Claudia configures Qdrant with environment variables (`QDRANT__STORAGE__STORAGE_PATH`, `QDRANT__SERVICE__HOST`, HTTP/gRPC ports, etc.; see `internal/supervisor/qdrant.go`), and the Qdrant process **working directory** is set to the **storage directory** (default `data/qdrant`). Qdrant may still probe for optional config files relative to that directory. If they are missing, it logs a **WARN** and continues using **environment variables and defaults**.

Add YAML only when you want settings that are easier to express in a file (for example clustering or extra services). Follow [Qdrant’s configuration guide](https://qdrant.tech/documentation/guides/configuration/) and place files where Qdrant expects them for your chosen layout (often under the storage or run directory).

A message that `./static` does not exist refers to Qdrant’s built-in **dashboard** static files. A release binary started with **cwd** in the storage directory usually has no `./static` there, so the web UI is not served. That is common for minimal or repackaged builds.

For this repository, Qdrant is optional when **RAG is off**. With **RAG on**, the gateway depends on Qdrant for ingest and retrieval. Most operational checks use the HTTP API on the configured port (default **6333**), for example `GET /readyz`, not the dashboard.

**Summary:** These warnings are **safe to ignore** for local development unless you want a custom Qdrant file-based configuration or the full web UI—in which case follow Qdrant’s docs and use a build or layout that includes the `static` assets.

## Usage

From the repo root (with `config/gateway.yaml`, `config/tokens.yaml`, `config/bifrost.config.json`):

```bash
export CLAUDIA_UPSTREAM_API_KEY=bifrost-local-dummy
export GROQ_API_KEY=...   # as needed
go run ./cmd/claudia serve
# or: ./claudia serve
```

Common flags:

| Flag | Default | Meaning |
|------|---------|---------|
| `-bifrost-bin` | `bifrost` | `bifrost-http` (or name on PATH); use `./bin/bifrost-http` (or `bifrost-http.exe`) after `make claudia-install` |
| `-bifrost-config` | `config/bifrost.config.json` | Source JSON copied into data dir |
| `-bifrost-data-dir` | `data/bifrost` | Writable BiFrost state directory |
| `-bifrost-bind` | `127.0.0.1` | `-host` (and `APP_HOST`) |
| `-bifrost-port` | `8080` | `-port` (and `APP_PORT`) |
| `-bifrost-log-level` | `info` | `-log-level` |
| `-bifrost-log-style` | `json` | `-log-style` (`json` or `pretty`) |
| `-upstream-host` | `127.0.0.1` | Host segment for Claudia → BiFrost URL (use when BiFrost binds `0.0.0.0`) |
| `-wait-bifrost` | `60s` | Max time to poll `/health` before exiting |
| `-no-wait-bifrost` | off | Skip readiness poll (debug only) |
| `-qdrant-bin` | *(empty)* | Qdrant executable; set e.g. `./bin/qdrant` to supervise Qdrant |
| `-qdrant-storage` | `data/qdrant` | On-disk vector storage (created) |
| `-qdrant-bind` | `127.0.0.1` | `QDRANT__SERVICE__HOST` |
| `-qdrant-http-port` | `6333` | HTTP API port |
| `-qdrant-grpc-port` | `6334` | gRPC port |
| `-qdrant-health-host` | `127.0.0.1` | Host for `/readyz` probe when `qdrant-bind` is `0.0.0.0` |
| `-wait-qdrant` | `60s` | Max time to poll `/readyz` |
| `-no-wait-qdrant` | off | Skip Qdrant readiness poll |

Gateway flags `‑config` and `‑listen` apply as in gateway-only mode. See `claudia serve -h`.

## Make targets

- `make claudia-install` → toolchain check + BiFrost + Qdrant per `deps.lock`
- `make install` → `claudia-install` then `desktop-install`
- `make up` → `install`, `claudia-build`, `desktop-build`, `desktop-run`
- `make claudia-serve` → foreground `go run … serve` with `./bin/qdrant` and `./bin/bifrost-http`
- `make claudia-start` / `make claudia-stop` / `make claudia-status` / `make logs` → background supervisor lifecycle (`scripts/claudia-start.sh`, etc.)
- `scripts/qdrant-from-release.sh` → Qdrant binary only → `./bin/` (invoked by `make claudia-install`; run by hand to refresh Qdrant without full install)

## Manual checklist (Linux)

1. Run `make claudia-install` (or ensure `./bin/bifrost-http` exists) or pass `-bifrost-bin`.
2. Run `claudia serve`; confirm `GET http://127.0.0.1:3000/health` (or your listen port) returns `ok` when BiFrost is up.
3. Send **SIGINT** to the parent; confirm child processes exit (no orphan `bifrost` / `qdrant` in `ps`).

## CI

End-to-end tests with real BiFrost/Qdrant binaries are **optional** (network, secrets). Unit tests cover config copy, env merge, `WaitHealthy`, and context-cancel kills `sleep` children on Unix.
