# Plan: Chimera operator CLI (`chimeractl`)

| Field                          | Value                          |
|--------------------------------|--------------------------------|
| **Doc kind**                   | `feature-plan`                 |
| **Owners / areas**             | Gateway, operator CLI, BiFrost |
| **Status**                     | `draft`                        |
| **Targets**                    | `chimeractl` v0.1/v0.8         |
| **Last updated**               | See git history                |
| **Supersedes / superseded by** | None                           |

## At a glance

Give operators a small command-line tool, `chimeractl`, that confirms the gateway is healthy and configures provider keys (Groq, Gemini, Ollama) without hand-editing files or pasting secrets into shell environment variables. Start with sensible defaults; add per-user and per-project config files later.

| Phase                                         | Outcome                                                                                              | Status |
|-----------------------------------------------|------------------------------------------------------------------------------------------------------|--------|
| [v0.1 — Health & provider setup](#version-01) | `chimeractl health`, `provider set-key`, `ollama set-url`; BiFrost `config_store` enabled by default | `todo` |
| [v0.8 — Layered configuration](#version-08)   | `~/.chimera/cli.config.yaml`, project-local config, `--host` / `--token` overrides                   | `todo` |

---

This document plans a **separate operator-facing CLI** for health checks and BiFrost-oriented setup (Groq, Gemini, Ollama). It is distinct from the `chimera` gateway/supervisor binary built by `make chimera-build` ([`cmd/chimera`](../cmd/chimera)), to keep a small **read-only / config** tool separate from the long-running server.

**Related docs:** [`supervisor.md`](../supervisor.md), [`reference/bifrost-upstream.md`](../reference/bifrost-upstream.md), [`configuration.md`](../configuration.md).

---

## Versioning

Releases below are **plan milestones** for `chimeractl` and the **gateway/BiFrost bootstrap** it depends on. Numbers are intentional (0.8 before a hypothetical 1.0) to leave room for intermediate work.

### Version 0.1

**Operator CLI**

- `chimeractl health` — probe the chimera-gateway (`GET /health`).
- `chimeractl provider set-key` — **groq** and **gemini** only; persists credentials through BiFrost’s management API.
- `chimeractl ollama set-url` — set the Ollama base URL via the same API surface.

**Configuration**

- **Hard-coded defaults only** in the binary (e.g. `http://127.0.0.1:3000` for the gateway, `http://127.0.0.1:8080` for BiFrost management). No `~/.chimera/cli.config.yaml`, no `./chimera/cli.config.yaml`, no `--host` / `--token` overrides yet.

**Gateway and BiFrost**

- Ship [`config/bifrost.config.json`](../config/bifrost.config.json) with **`config_store` enabled** so BiFrost persists provider config and exposes the management APIs this CLI uses.
- **Stop using environment variables** as the primary way to inject Groq/Gemini (and related) secrets into BiFrost for the default stack: operators use `chimeractl` (or the BiFrost UI/API) instead of `GROQ_API_KEY` / `GEMINI_API_KEY` in `.env` / the shell. Update [`env.example`](../env.example), [`supervisor.md`](../supervisor.md), and [`reference/bifrost-upstream.md`](../reference/bifrost-upstream.md) accordingly when implementing.

### Version 0.8

**Configuration layering** (see [§ Configuration](#configuration)):

- Load overrides from `~/.chimera/cli.config.yaml` (user home).
- Load overrides from `./chimera/cli.config.yaml` (current working directory).
- Apply **command-line parameters** (`--host`, `--token`, and any later flags).

**Precedence** (lowest → highest): built-in defaults → home file → local file → flags.

All **v0.1** commands remain; they gain configurable endpoints and tokens without recompiling.

---

## Goals

1. **Makefile integration:** `make cli-install`, `make cli-build`, `make cli-run` (see [§ Makefile](#makefile)) — target **v0.1**; unchanged in v0.8 unless new install hooks are needed.
2. **Layered configuration (v0.8)** with explicit precedence: built-in defaults → global user file → repo-local file → flags (see [§ Configuration](#configuration)).
3. **`health` (v0.1)** — verify the **chimera-gateway** is reachable and report readiness (reuse `GET /health` semantics).
4. **`provider set-key` (v0.1)** — configure **Groq** and **Gemini** provider credentials in BiFrost via HTTP API (see [§ BiFrost API + config store](#bifrost-api--config-store)).
5. **`ollama` (v0.1)** — configure the **Ollama** endpoint BiFrost uses via the management API (base URL / key schema per pinned BiFrost OpenAPI).

---

## Binary and module layout

| Item              | Proposal                                                                                                                                                                                                    |
|-------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| **Go package**    | `cmd/chimeractl`                                                                                                                                                                                            |
| **Artifact name** | `chimeractl` (Unix), `chimeractl.exe` (Windows) — avoids clashing with `chimera`                                                                                                                            |
| **Import path**   | `github.com/lynn/porcelain/cmd/chimeractl` (thin `main` only)                                                                                                                                               |
| **Shared logic**  | `internal/clicfg` — v0.8: YAML merge + flags; v0.1: optional thin defaults package or constants in `main`. `internal/cliclient` — gateway HTTP. `internal/bifrostapi` — BiFrost management `/api/*` (v0.1). |

Rationale: one repo, one `go.mod`, no second module unless release packaging later demands it.

---

## Makefile

Add targets alongside existing gateway/GUI rules ([`Makefile`](../Makefile)):

| Target             | Behavior                                                                                                                                                         |
|--------------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `make cli-build`   | `go build -o chimeractl[.exe] ./cmd/chimeractl` (mirror `GUI_BIN` / `OS` pattern for `.exe` on Windows).                                                         |
| `make cli-run`     | `go run ./cmd/chimeractl` — pass through extra args, e.g. `make cli-run ARGS='health'`.                                                                          |
| `make cli-install` | `go install ./cmd/chimeractl` so the binary lands in `$GOBIN` or `$GOPATH/bin` (document that this must be on `PATH`). Optional later: `PREFIX=` install script. |

**Also update** [`scripts/print-make-help.sh`](../scripts/print-make-help.sh) and `.PHONY` list so `make help` documents the new targets.

**`make clean`:** Decide whether to remove `chimeractl[.exe]` (align with removing `chimera[.exe]` in [`scripts/clean.sh`](../scripts/clean.sh)); recommended **yes** for consistency.

---

## Configuration

**Scope:** Full behavior in this section applies from **v0.8**. **v0.1** uses **compiled-in defaults only** ([§ Version 0.1](#version-01)).

### Precedence (lowest → highest)

1. **Built-in defaults** (compiled into the binary).
2. **Global user config:** `~/.chimera/cli.config.yaml`  
   - Resolve home via `os.UserHomeDir()` (on Windows, `%USERPROFILE%\.chimera\cli.config.yaml`).
3. **Local project config:** `./chimera/cli.config.yaml` relative to the **current working directory** (as specified; do not rename to `config/` without an explicit follow-up).
4. **CLI flags:** `--host`, `--token` (and any future flags) **override** merged YAML for those fields only.

Merge rule: **later layers override earlier** for the same key. Missing keys fall through to the previous layer.

### Suggested `cli.config.yaml` shape (v0.8)

Documented fields (all optional except as noted by commands):

```yaml
# Base URL of the chimera-gateway (no trailing slash).
gateway_url: "http://127.0.0.1:3000"

# Bearer token for gateway routes that require it (e.g. if /health ever requires auth in a custom deployment).
# Prefer env CHIMERACTL_TOKEN in docs for CI; file is optional.
gateway_token: ""

# BiFrost management API base (OpenAI-style upstream root), used for provider/ollama setup.
# Default: "http://127.0.0.1:8080" to match config/gateway.yaml upstream.
bifrost_url: "http://127.0.0.1:8080"

# When BiFrost dashboard auth is enabled: Basic auth or session flow — document in v0.8+
# env vars BIFROST_USER / BIFROST_PASSWORD or bifrost_bearer in YAML (optional).
```

**Flags:**

- `--host` — overrides `gateway_url` only (string URL), **not** `bifrost_url` (add `--bifrost` if a single `--host` is ambiguous; see [§ Open decisions](#open-decisions)).
- `--token` — overrides `gateway_token` (Bearer for gateway).

**Security:** Warn in README that global and local YAML may contain secrets; recommend `chmod 600` on Unix and `.gitignore` for `./chimera/cli.config.yaml` if operators store tokens there.

---

## Commands

Shipped in **v0.1**; URLs and optional Bearer token stay at defaults until **v0.8** ([§ Versioning](#versioning)).

### `chimeractl health`

- **Purpose:** Operator check that the **chimera-gateway** is up.
- **Behavior:** `GET {gateway_url}/health` with optional `Authorization: Bearer {token}` if token non-empty.
- **Exit codes:** `0` if HTTP 2xx and body parses as expected JSON (same shape as gateway today); non-zero on network error, timeout, or bad status.
- **Output:** Human-readable one-liner + optional `--json` for scripts (optional flag in plan; implement if low cost).

### `chimeractl provider set-key <groq|gemini>`

- **Purpose:** Register or update the provider API key BiFrost uses for that provider.
- **Arguments:** Provider name: `groq` or `gemini` only (v0.1 scope; more providers later if needed).
- **Secret input:** Prefer `--from-env VAR` (read value from environment, never log) or **stdin** (`--stdin`) over raw argv to avoid shell history leaks.
- **Backend:** BiFrost management HTTP API ([§ BiFrost API + config store](#bifrost-api--config-store)).

### `chimeractl ollama set-url <url>`

- **Purpose:** Point BiFrost’s Ollama integration at a base URL (e.g. `http://127.0.0.1:11434`).
- **Backend:** Same as keys — BiFrost management HTTP API ([§ BiFrost API + config store](#bifrost-api--config-store)).

### Help and version

- `chimeractl help` / `-h` / subcommand help consistent with `cmd/chimera` style.
- Reuse embedded version variables if shared via `internal/version` or ldflags (optional follow-up).

---

## BiFrost API + config store

Operator commands that mutate provider keys or Ollama **must** use BiFrost’s **management HTTP APIs** (e.g. `POST /api/providers/{provider}/keys`, updates for existing keys, and Ollama URL via the provider/key schema in BiFrost’s OpenAPI for the `chimera/deps.lock` pin). See [BiFrost “Setting Up”](https://docs.getbifrost.ai/quickstart/gateway/setting-up).

**Prerequisite (v0.1):** [`config/bifrost.config.json`](../config/bifrost.config.json) (copied into the BiFrost app dir on `chimera serve` startup) **must** include `config_store` enabled (e.g. SQLite under the BiFrost data directory) so configuration persists and admin routes behave as documented. Without `config_store`, file-only mode does not support this workflow. **v0.1** lands the config-store bootstrap together with `chimeractl` and removes reliance on provider env vars for the default stack ([§ Version 0.1](#version-01)).

**CLI config:** `bifrost_url` plus optional credentials for dashboard/admin auth ([`reference/bifrost-upstream.md`](../reference/bifrost-upstream.md) remains the reference for env-vs-file nuances in **static** JSON; runtime changes go through the API once `config_store` is on).

**Implementation order:** `health` first (gateway only, no BiFrost). `provider set-key` and `ollama set-url` require a **running** BiFrost with `config_store` and appropriate auth if enabled.

---

## Dependencies

- `gopkg.in/yaml.v3` — already in [`go.mod`](../go.mod); required for **v0.8** YAML config loading. **v0.1** may omit YAML paths entirely until v0.8.
- Stdlib `flag` (or small helper later) for **v0.8** CLI overrides; **v0.1** subcommands need only enough parsing for `health`, `provider set-key`, `ollama set-url`.
- No extra runtime dependency for HTTP (`net/http`).

---

## Testing

- **v0.8 — Unit:** Config merge order (table-driven): defaults + global + local + flags.
- **v0.1 — Integration (optional):** `httptest` mocking `GET /health` for `chimeractl health`.
- **v0.1 — BiFrost API:** Mock handlers or manual verification against local `bifrost-http` with `config_store` enabled.

---

## Documentation deliverables (when implemented)

- **v0.1:** [`README.md`](../README.md) — `make cli-*`, `chimeractl health`, BiFrost `config_store`, no provider env vars for default stack.
- **v0.8:** Document `~/.chimera/cli.config.yaml`, `./chimera/cli.config.yaml`, flags, and precedence; optional `docs/cli.md` or `--help` only.

---

## Open decisions

1. **`--host` scope:** If operators want one flag for both gateway and BiFrost, consider `--gateway` / `--bifrost` explicitly instead of overloading `--host`.
2. **Local config path:** `./chimera/cli.config.yaml` requires a `chimera/` directory in each project; provide `chimeractl init` later to create it from a template.
3. **`config_store` in repo `bifrost.config.json`:** **v0.1** requirement — enable for the default stack (SQLite under `data/bifrost/` when supervised); document UI/DB on disk and first-boot bootstrap behavior per BiFrost docs.
4. **CI:** Add `vet`/`test` for `./cmd/chimeractl/...` and new `internal/clicfg` packages to `.github/workflows` if not already covered by `go test ./...`.

---

## Implementation checklist (summary)

**v0.1**

- [ ] Add `cmd/chimeractl`: `health`, `provider set-key` (groq, gemini), `ollama set-url`; **hard-coded** `gateway_url` / `bifrost_url` (and token default empty).
- [ ] Add Makefile targets + `clean.sh` + `print-make-help.sh` updates.
- [ ] Enable `config_store` in [`config/bifrost.config.json`](../config/bifrost.config.json); remove default-stack reliance on provider **environment variables**; refresh `env.example` and docs.
- [ ] Implement BiFrost management API client (`internal/bifrostapi` or equivalent) against pinned OpenAPI.
- [ ] Tests for health command; manual or mocked BiFrost API tests.

**v0.8**

- [ ] Add `internal/clicfg`: load `~/.chimera/cli.config.yaml` and `./chimera/cli.config.yaml`, merge with defaults, apply `--host` / `--token` (and `--bifrost` if adopted).
- [ ] Document precedence, secrets, `.gitignore` for `./chimera/`.
- [ ] Table-driven tests for config merge order.

---

*Plan status: **draft for implementation** — v0.1 first; v0.8 adds layered config. Gateway defaults today: [`config/gateway.yaml`](../config/gateway.yaml).*
