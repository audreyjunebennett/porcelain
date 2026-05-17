# Chimera Gateway Runtime

This repo is **Chimera**, the gateway runtime layer inside **Porcelain**. It is a **Go** OpenAI-compatible HTTP service that fronts **BiFrost** (provider keys and upstream models). Locus/workspace clients use a single virtual model id (`Chimera-<semver>`), gateway-issued bearer credentials, YAML routing policy with mtime reload, and sequential upstream fallback on 429/5xx; `GET /health` reports upstream (and optional Qdrant when RAG is on). With `rag.enabled`, Chimera adds **Qdrant** retrieval, `POST /v1/ingest`, indexer REST, and optional supervised `chimera-indexer`. Operator docs are in `docs/`; milestones and release notes in `docs/plans/`. Current ship line is **v0.2.x** — see [docs/version-v0.2.md](docs/version-v0.2.md#shipped-releases-v020-through-v022) for v0.2.0–v0.2.2 deliverables.

## Quick start

You need **GNU Make**.

On Windows:

```powershell
pwsh -ExecutionPolicy Bypass -File scripts/install-make.ps1
```

On Ubuntu and macOS:

```bash
bash scripts/install-make.sh
```

### Install and Start (all-in-one)

One-shot onboarding: get dependencies, seed config if needed, build the gateway, and start the supervisor in the background.

```bash
make up
```

## Installing Dependencies and Building the Service

### Install

Install all the dependencies and build the dependent projects.

```bash
make install
```

`make install` runs `make chimera-install` (toolchain check, BiFrost, Qdrant) and then `make locus-desktop-install` (native WebView/CGO deps). For a **headless** machine or CI-style setup where you only need `./bin/bifrost-http` and `./bin/qdrant`, use `make chimera-install` only.

**Dependencies**

- **Go (1.22+)** — builds the gateway and BiFrost’s Go code.
- **Node.js (20+)** — BiFrost’s UI is built with npm during install; it is not shipped inside the `chimera` binary.
- **Git** — BiFrost is vendored from a tracked revision, not embedded in the clone.
- **GNU Make** — single entrypoint for install and build targets from the repo root.
- **gcc or clang** — BiFrost’s HTTP server is built with **CGO**; the Go toolchain must invoke a C compiler or the build fails early.
- **bash, curl, tar** (and **unzip** on Windows) — reliable way to download and unpack release artifacts the same way on every platform.

Full reference: [docs/installation.md](docs/installation.md).

### Configuration

Create local config files from the shipped examples when they are missing.

```bash
make configure
```

**Creates (only if absent):** `config/gateway.yaml` from [config/gateway.example.yaml](config/gateway.example.yaml). Copy [env.example](env.example) to `.env` yourself when you want local env vars. It does **not** create `config/api-keys.yaml`: on first run, `chimera serve` or `chimera gateway` enters **bootstrap** on localhost and `/ui/setup` creates `api-keys.yaml`; then **restart** for the full stack. You can also copy [config/api-keys.example.yaml](config/api-keys.example.yaml) to `config/api-keys.yaml` manually.

| Purpose | File | Role |
| ------- | ---- | ---- |
| Process environment | `.env`<br/>(copied from [env.example](env.example)) | Optional env file for the gateway→BiFrost upstream key and provider API keys. Not committed. |
| Gateway client auth | `config/api-keys.yaml`<br/>(from [config/api-keys.example.yaml](config/api-keys.example.yaml) or **setup UI**) | API secrets for clients (`Authorization: Bearer …`) and admin UI login. Not committed. |
| Gateway listen + upstream | `config/gateway.yaml` | Listen address, BiFrost upstream URL, routing/RAG/indexer paths, metrics — primary gateway config. Not committed. |
| BiFrost bootstrap | `config/bifrost.config.json` | BiFrost HTTP config; provider secrets pulled from environment variables set in `.env` or the shell. |
| Virtual model mapping | `config/routing-policy.yaml` | Rules that define how the virtual `chimera-<semver>` model routes prompts/turns to underlying models |
| Free-tier allowlist (optional) | `config/provider-free-tier.yaml` | When `routing.filter_free_tier_models` is true, restricts merged model listing and UI routing generation to ids in this file |

**Manual follow-up**

1. `.env`: set `CHIMERA_UPSTREAM_API_KEY` to match `upstream.api_key_env` in `config/gateway.yaml`. Set `GROQ_API_KEY`, `GEMINI_API_KEY`, or other keys that `config/bifrost.config.json` references.
2. `config/api-keys.yaml`: create via first-run `/ui/setup` (see [docs/plans/version-v0.1.md](docs/plans/version-v0.1.md)) or copy from `api-keys.example.yaml`; clients use `Authorization: Bearer <secret>`.
3. `config/gateway.yaml` — run `make configure` once (or copy `gateway.example.yaml`) if missing; adjust `listen_host` / `listen_port`, `upstream.base_url`, `routing.fallback_chain`, paths if needed. Not committed.
4. `config/bifrost.config.json` — align provider blocks and `env.*` with your `.env`.
5. `config/routing-policy.yaml` — committed default; edit or point `gateway.yaml` at another file.
6. `config/provider-free-tier.yaml` — optional operator allowlist; shipped default; tune `models` / `patterns` and set `routing.filter_free_tier_models` in `gateway.yaml` if you want catalog filtering. To refresh a **reference** list from [Groq](https://console.groq.com/docs/rate-limits) + [Gemini pricing](https://ai.google.dev/gemini-api/docs/pricing) docs (network required), run `make catalog-free` (optional `INTERSECT=` path to JSON or YAML `data[].id`, e.g. `config/catalog-available.snapshot.yaml`). To snapshot models from a running BiFrost, run `make catalog-available`. See [docs/configuration.md](docs/configuration.md).

Full reference: [docs/configuration.md](docs/configuration.md).

### Build the Gateway

The install process builds and downloads **BiFrost** and **Qdrant**. This builds the gateway.

```bash
make chimera-build
```

## Managing the Service

With BiFrost and Qdrant binaries installed and the gateway built, you can run the supervised stack.

### Start the Service

`make chimera-supervisor-run` starts the Chimera supervisor runtime with **BiFrost** and **Qdrant** (foreground).

```bash
make chimera-supervisor-run
```

Background supervisor + PID file:

```bash
make chimera-start
```

Further reference: [docs/supervisor.md](docs/supervisor.md).

### View the logs of the background service

Follow the supervisor log in the terminal.

```bash
make logs
```

Follows `logs/chimera-supervisor.log` (typically `tail -f`). Useful after `make up` or `make chimera-start`.

### Check Service

`make chimera-status` reads the PID file and checks gateway, BiFrost, and Qdrant health.

```bash
make chimera-status
```

### Stop the Service

```bash
make chimera-stop
```

## Desktop (native webview shell)

Operator UI is served by the gateway at `/ui/login` and `/ui/panel` (browser or embedded webview). A separate `locus-desktop` binary (`-tags desktop`, **CGO**) runs `chimera desktop` — same flags as `chimera serve` — and opens a native window to the panel.

| Target | What it does |
| ------ | ------------ |
| `make locus-desktop-install` | Installs native deps for **WebView** + **CGO** (Debian/Ubuntu **WebKitGTK**, macOS **CLT**, Windows hints + MSYS2 `gcc`). |
| `make locus-desktop-build` | `CGO_ENABLED=1 go build -tags desktop` → `./locus-desktop` (or `.exe` on Windows). |
| `make locus-desktop-run` | `locus-desktop-build` if missing, then `locus-desktop desktop` with the same **Qdrant/BiFrost** flags as `make chimera-supervisor-run`. |
| `make vet-desktop` | `go vet -tags desktop ./cmd/chimera` with **CGO** (same toolchain as `desktop-build`). |

`make vet` includes `vet-desktop` unless `SKIP_DESKTOP=1`; see **Testing and Linting** below.

## Testing and Linting

| Target | What it does |
| ------ | ------------ |
| `make fmt` | `gofmt -w` on `cmd` and `internal`. |
| `make fmt-check` | Fails if `gofmt` would change any file (same check as CI). Used by `precommit`. |
| `make vet` | `vet-module` (`go vet ./...`) plus `vet-desktop` unless `SKIP_DESKTOP=1`. Used by `precommit`. |
| `make vet-module` | `go vet ./...` on the main module. |
| `make vet-desktop` | `go vet -tags desktop ./cmd/chimera` with **CGO**. |
| `make test` | Runs `test-internal`, `test-catalog-free`, `test-catalog-available`, `chimera-gateway-test`, and `test-desktop` unless `SKIP_DESKTOP=1`. `-race` on Unix (same as before). |
| `make test-internal` / `chimera-gateway-test` / `test-desktop` / `test-catalog-free` / `test-catalog-available` | `go test` on that subtree; use `test-desktop` for the desktop-tagged gateway binary (CGO). |
| `make precommit` | Runs `fmt-check`, `vet`, and `test`. On Windows/Git Bash, `./scripts/precommit-smoke.sh` uses `SKIP_DESKTOP=1` by default; set `FULL_DESKTOP=1` to include desktop vet/test. |

## Repo Management and Packaging

### Clean up built binaries

Remove **built gateway/GUI binaries** and release scratch output.

```bash
make clean
```

**Deletes** `./chimera`, `./chimera-supervisor`, `./locus-desktop`, Windows `.exe` variants, and `dist/`. Does **not** remove `bin/bifrost-http`, `bin/qdrant`, `.deps/`, `run/`, or `logs/`.

### Clean up everything

Deep clean: everything `make clean` removes **plus** third-party binaries and working directories.

```bash
make clean-all CONFIRM=1
```

**Requires** `CONFIRM=1`. **Also removes** `bin/bifrost-http`, `bin/qdrant`, `.deps/`, `run/`, `logs/` (after running `make clean`). Use when you want a fresh `make install`.

### `make release-snapshot`

```bash
make release-snapshot
```

Build snapshot artifacts with **GoReleaser**.

**Requires** `goreleaser` on `PATH`. **Writes** snapshot outputs under `dist/` (see [docs/packaging.md](docs/packaging.md)).


## Documentation

- **Index:** [docs/README.md](docs/README.md)
- **Overview / ports:** [docs/overview.md](docs/overview.md), [docs/network.md](docs/network.md)
- **Installation:** [docs/installation.md](docs/installation.md)
- **Configuration:** [docs/configuration.md](docs/configuration.md)
- **Supervisor:** [docs/supervisor.md](docs/supervisor.md)
- **Packaging / releases:** [docs/packaging.md](docs/packaging.md)
- **Makefile plan:** [docs/plans/makefile.plan.md](docs/plans/makefile.plan.md)
- **Plans & versions index:** [docs/plans/README.md](docs/plans/README.md)
- **Admin UI / desktop shell:** [docs/plans/ui-tool.plan.md](docs/plans/ui-tool.plan.md); legacy Fyne checklist removed (use `/ui/*` + `make locus-desktop-build`).
- **Continue samples:** [vscode-continue/README.md](vscode-continue/README.md)
- **Security:** [SECURITY.md](SECURITY.md)
- **Product / requirements (normative):** [docs/porcelain.plan.md](docs/porcelain.plan.md)

## Development roadmap

| Version | Where to read |
|---------|---------------|
| **v0.1** | [Working notes](docs/plans/version-v0.1.md); [Go + BiFrost migration plan](docs/plans/go-bifrost-migration.plan.md) |
| **v0.1.1** | [Tool router, metrics, quotas](docs/plans/version-v0.1.1.md) |
| **v0.2.0 – v0.2.2** | [Shipped releases + capability plan](docs/version-v0.2.md) |
| **Planned (desktop/onboarding)** | [Working plan — gateway v0.3](docs/plans/version-v0.3.md) |
| **Later** | [Release roadmap](docs/porcelain.plan.md#release-roadmap) in [docs/porcelain.plan.md](docs/porcelain.plan.md) |

`docs/porcelain.plan.md` still anchors historical LiteLLM + Compose requirements text where relevant; the **shipping** stack here is **Go + BiFrost** (+ optional Qdrant / indexer) as documented in `docs/` and `docs/plans/`.

## License

Private / unspecified — add a `LICENSE` if you publish.
