# Plan: chimera-gateway — Go rewrite, BiFrost, packaging, and GUI

| Field | Value |
|-------|-------|
| **Doc kind** | `working-notes` |
| **Owners / areas** | Gateway, BiFrost, desktop packaging |
| **Status** | `shipped` |
| **Targets** | Gateway v0.1 migration to Go + BiFrost |
| **Last updated** | See git history |
| **Supersedes / superseded by** | Historical migration record; superseded operationally by [`supervisor.md`](../supervisor.md) and [`packaging.md`](../packaging.md) |

## At a glance

Move Chimera onto a Go gateway with BiFrost as the upstream LLM proxy, give it one supervised command that starts everything, and ship a desktop binary so operators install one bundle and click run. This document is the historical record of how that migration landed phase by phase.

| Phase | Outcome | Status |
|-------|---------|--------|
| [Phase 0 — BiFrost discovery](#phase-0--discovery-bifrost-with-the-existing-typescript-gateway) | Validate BiFrost as the upstream and capture parity gaps | `done` |
| [Phase 1 — Go gateway scaffold](#phase-1--go-gateway-project-scaffold-and-http-parity-spike) | Minimal Go binary proxying chat, models, and health | `done` |
| [Phase 2 — v0.1 feature parity](#phase-2--go-gateway-feature-parity-with-v01-chimera-bifrost-upstream) | Virtual model, token auth, routing policy, fallback chain | `done` |
| [Phase 3 — Supervised stack](#phase-3--process-supervision-one-command-runs-bifrost--chimera) | One command runs BiFrost + Chimera with graceful shutdown | `done` |
| [Phase 4 — Cross-platform packaging](#phase-4--cross-platform-packaging-macos-windows-linux) | Releasable artifacts for macOS, Windows, and Linux | `done` |
| [Phase 5 — Desktop GUI](#phase-5--gui-mew-mew-love-chimera) | Desktop window for the basic flow | `done` |
| [Phase 6 — Hardening & TS sunset](#phase-6--hardening-operator-ux-and-typescript-gateway-sunset) | Security pass, migration guide, TypeScript server retired | `done` |

---

This document is a **phased migration plan**. Each phase has a **deliverable**, **verification (tests)**, and **TODO** items. Work proceeds **one phase at a time**: the **user asks the agent to implement the next phase**. When an agent finishes a phase, it **updates this file** (checkboxes, completion notes, and links to PRs/commits as appropriate).

**Product goals (end state)**

- **chimera-gateway** implemented in **Go**, with BiFrost as the component that **manages API keys and provider connections** (replacing LiteLLM in the reference architecture).
- **One distributable** operators can run on **macOS, Windows, and Linux** that bundles or supervises **both** BiFrost and Chimera (exact layout decided in packaging phases).
- A **GUI** that displays the message: `mew mew, Love Chimera` (minimum viable UI; may grow into settings/service management later).

**Non-goals for this document**

- It does not replace [porcelain.plan.md](../porcelain.plan.md) for normative product requirements; it **implements** a technical path toward v0.1+ goals described in [version-v0.1.md](version-v0.1.md).

---

## How agents should update this plan

After completing work for a phase:

1. Mark that phase’s **TODO** checkboxes as done (`[x]`).
2. Under **Phase completion log**, add a dated entry: phase name, summary, PR or commit SHA, and any **follow-ups** or **deferred items**.
3. If scope changed, edit the phase text in place and note **why** in the log.

---

## Phase completion log

| Date | Phase | Summary | Reference |
|------|--------|---------|-----------|
| 2026-04-03 | Phase 0 | BiFrost + TypeScript gateway discovery: `docs/bifrost-discovery.md` added; operator verified VS Code → Chimera → BiFrost; Compose/image and compatibility matrix recorded. Optional: pin BiFrost image digest; add `curl` receipts to discovery doc if desired. | [bifrost-discovery.md](../bifrost-discovery.md) |
| 2026-04-04 | Phase 1 | Go module `github.com/lynn/porcelain`: `cmd/chimera` binary, `internal/gateway` health + `/v1/*` reverse proxy (SSE flush), `httptest` integration tests, `.github/workflows/go.yml` (fmt/vet/test -race), `scripts/precommit-smoke.sh`. README mapping table. No YAML config file yet (flags + env). | `go.mod`, [README.md](../README.md) |
| 2026-04-04 | Phase 2 | v0.1 parity in Go: `config/gateway.yaml` + mtime reload, `tokens` / `routing-policy` stores, virtual model + fallback (429/5xx), BiFrost `/api/models` + `/v1/models`, `checks.upstream` health JSON, slog logging. Packages: `internal/config`, `tokens`, `routing`, `upstream`, `chat`, `server`. Tests: routing policy, 429 fallback integration, models list order. **Dual-ship:** Compose `chimera` stays TypeScript until a later phase; operators may run Go binary with same YAML. Optional: script against live BiFrost in CI — not added (network/secrets). | [configuration.md](../configuration.md), [README.md](../README.md) |
| 2026-04-04 | Phase 3 | `chimera serve` / `supervise`: subprocess BiFrost (`APP_HOST`/`APP_PORT`, data dir, config copy), poll `/health`, gateway with `NewRuntimeWithUpstreamOverride` loopback URL; SIGINT/SIGTERM → HTTP Shutdown then child cancel. `internal/supervisor`, docs [supervisor.md](../supervisor.md). Tests: env merge, config copy, WaitHealthy, sleep killed on cancel (Unix). E2E with real BiFrost binary: optional/deferred. | [supervisor.md](../supervisor.md) |
| 2026-04-04 | Phase 4 | GoReleaser v2: `.goreleaser.yaml`, `chimera -version` / `--version` (ldflags), archives linux/darwin amd64+arm64 + windows amd64, `checksums.txt`. CI: `package` job snapshot + smoke; `release.yml` on `v*` tags. `docs/packaging.md`, `make release-snapshot`. BiFrost not bundled; signing deferred. | [packaging.md](../packaging.md), `.goreleaser.yaml` |
| 2026-04-04 | Phase 5 | Fyne v2 desktop app in nested module `gui/` showing `mew mew, Love Chimera`;  CI `gui` job: linux-amd64 compile with CGO + X11 deps. Manual checklist `docs/gui-testing.md`. GUI **not** in GoReleaser zip (CGO/cross-compile); documented in packaging. Supervisor launch from GUI deferred. | [gui-testing.md](../gui-testing.md), `gui/` |
| 2026-04-04 | Phase 6 | `SECURITY.md`: tokens, log redaction, bind surface, supervisor. `docs/operator-migration-to-go.md`, `scripts/e2e-first-chat-curl.sh` (historical `docs/e2e-operator-path.md` later removed from tree). README + plan: **TypeScript sunset** (Go primary; `src/` legacy for Compose image). `src/README.md`. Config fuzz: `internal/config/fuzz_test.go` (`FuzzLoadGatewayYAML`). Audit: HTTP logs use `redactAuth`; config/tokens reload paths do not log secrets. **RC tag:** maintainers cut with `git tag v0.1.0-rc.1` (or semver) per [packaging.md](../packaging.md); not automated here. | [SECURITY.md](../SECURITY.md), [operator-migration-to-go.md](../operator-migration-to-go.md) |
| 2026-04-04 | Follow-up | Removed **TypeScript** `src/`, **Dockerfile** / `docker-compose.yml`, and **LiteLLM** `config/litellm_config.yaml`. Repo is **Go-only**, local **BiFrost**. | [README.md](../README.md), [operator-migration-to-go.md](../operator-migration-to-go.md) |

---

## Phase 0 — Discovery: BiFrost with the **existing** TypeScript gateway

**Intent.** De-risk BiFrost **before** committing to Go. The agent and user share a **discovery** pass: install BiFrost, configure keys and models in BiFrost, point the **current** Node/Fastify gateway at BiFrost’s OpenAI-compatible base URL, and record gaps.

**Deliverable**

- `docs/bifrost-discovery.md` (or equivalent) containing:
  - Exact BiFrost version(s) tried and install method(s) (e.g. Docker image tag, released binary, `npx`).
  - Minimal **BiFrost configuration** needed for at least one real completion (chat) and `GET /v1/models`.
  - `config/gateway.yaml` (or env) settings: `upstream.base_url` points at BiFrost (or any OpenAI-compatible proxy). Legacy `litellm` YAML keys remain supported as fallbacks.
  - **Compatibility matrix**: streaming (SSE) vs non-streaming, auth header shape, error codes, timeouts, anything that differs from LiteLLM behavior.
  - **Go migration implications**: what must be abstracted in a future Go gateway (endpoints, headers, fallback triggers).
- Optional but valuable: a **Compose override** or **documented commands** to run BiFrost + Chimera together for local reproduction (without removing LiteLLM docs until a later phase).

**Tests / acceptance criteria**

- [x] **Manual smoke**: With BiFrost up and keys configured in BiFrost (not duplicated in Chimera for provider secrets), **`curl` or equivalent** succeeds against Chimera for **non-streaming** chat using the virtual model path that exercises routing/fallback **or** a documented limitation if parity is impossible in TS without code changes.
- [x] **Manual smoke**: **Streaming** completion works through Chimera → BiFrost **or** discovery doc states the gap and reproduction steps.
- [x] `GET /v1/models` through Chimera returns expected model list including virtual model behavior **or** gap is documented with workaround.
- [x] `GET /health` on Chimera reflects upstream reachability in a way that matches current semantics **or** documented delta.
- [x] Discovery doc includes a **short “definition of done” checklist** the next phase can rely on.

**TODO (Phase 0)**

- [x] Install and run BiFrost per official docs; record version and command lines.
- [x] Configure at least one provider **only in BiFrost**; confirm completions work **directly** against BiFrost.
- [x] Point existing gateway config at BiFrost; fix or document any gateway-side changes needed (minimal diff to TS allowed if required for discovery).
- [x] Write `docs/bifrost-discovery.md` with the sections above.
- [x] Execute and record results of the **acceptance** checks (pass/fail/skip with reason).

**Status:** ☐ Not started · ☐ In progress · ☑ **Complete**

---

## Phase 1 — Go gateway: project scaffold and HTTP parity spike

**Intent.** Create a **new Go module** (suggested layout: `cmd/chimera/`, `internal/...`) that can serve `GET /health` and proxy `POST /v1/chat/completions` and `GET /v1/models` to a configurable upstream (BiFrost URL), without yet full feature parity with TypeScript.

**Deliverable**

- Runnable `chimera` (or `porcelain`) binary from `go build` with configuration via flags and/or a small config file.
- Documented **mapping** from current YAML/env concepts to Go config (can be subset in this phase).

**Tests / acceptance criteria**

- [x] `go test ./...` passes in CI (add Go workflow if missing).
- [x] **Integration or handler tests** (e.g. `httptest` + fake upstream) verify: request forwarding, required headers, streaming pass-through behavior at the HTTP level for a **minimal** SSE fixture.
- [x] **Manual or scripted smoke**: binary against a fake upstream confirms listen address and timeout behavior.

**TODO (Phase 1)**

- [x] Add Go module and `README` section for building the Go binary.
- [x] Implement minimal reverse proxy or typed client for chat + models + health upstream probe.
- [x] Add CI job running `go test ./...` (and `go vet` / formatting as project standard).

**Status:** ☐ Not started · ☐ In progress · ☑ **Complete**

---

## Phase 2 — Go gateway: feature parity with v0.1 Chimera (BiFrost upstream)

**Intent.** Port **virtual model**, **token auth**, **routing policy**, **fallback chain on 429/5xx**, **config reload** (or equivalent), and **logging** to match [version-v0.1.md](version-v0.1.md) behavior as closely as BiFrost allows.

**Deliverable**

- Go gateway passes a **parity checklist** derived from TypeScript behavior and `docs/bifrost-discovery.md`.
- Migration note (**decision**): **Dual-ship** for now — the **Dockerfile / Compose `chimera` service** remains **TypeScript**; the **Go `chimera` binary** is an alternative runtime using the **same YAML**. Removing or archiving the Node server is **out of scope** until packaging/supervisor phases; no hard sunset date.

**Tests / acceptance criteria**

- [x] **Unit tests** for routing policy evaluation and fallback ordering (fixtures from current YAML samples).
- [x] **Integration tests** with **mock upstream** returning 429/5xx to assert fallback chain walk.
- [x] **Golden or snapshot tests** for virtual model id and models list ordering where stable.
- [ ] **Optional**: black-box test script in `scripts/` that runs Go binary against BiFrost in CI (may be `workflow_dispatch` only if secrets/network heavy) — deferred.

**TODO (Phase 2)**

- [x] Port token loading and mtime reload (or chosen alternative).
- [x] Port routing policy and virtual model + fallback logic.
- [x] Align **all** public routes required for v0.1 Continue/client compatibility.
- [x] Update operator docs to describe **BiFrost + Go Chimera** as the reference path.

**Status:** ☐ Not started · ☐ In progress · ☑ **Complete**

---

## Phase 3 — Process supervision: one command runs BiFrost + Chimera

**Intent.** A single **entry binary** (could be the same `chimera` with a `serve` subcommand) that **starts BiFrost** and **Go Chimera**, sets **inter-process URLs** (e.g. localhost ports), handles **signals** for graceful shutdown, and optionally **waits for upstream readiness**.

**Deliverable**

- Documented **runtime architecture** (two processes, ports, config dirs, data dirs for BiFrost).
- Supervisor behavior verified on **Linux** at minimum; macOS/Windows noted or tested in Phase 4.

**Tests / acceptance criteria**

- [x] **Unit tests** for supervisor logic where testable without spawning real BiFrost (mock commands or interface injection).
- [ ] **Integration test** (optional in CI): job that downloads or uses a **pinned BiFrost binary** / Docker to verify end-to-end **one command** startup (may be nightly or manual job—document which) — deferred; see [supervisor.md](../supervisor.md).
- [x] Manual checklist: SIGINT stops both children without zombie processes — documented in [supervisor.md](../supervisor.md) (operator-run).

**TODO (Phase 3)**

- [x] Implement subprocess management (start order, env, working directory).
- [x] Embed or document **how BiFrost binary is obtained** (bundled path, `PATH`, or download helper—align with licensing).
- [x] Expose flags for ports and config paths.

**Status:** ☐ Not started · ☐ In progress · ☑ **Complete**

---

## Phase 4 — Cross-platform packaging (macOS, Windows, Linux)

**Intent.** Produce **releasable artifacts** per OS/arch (e.g. `.tar.gz`, `.zip`, macOS bundle or signed `.app` TBD, Windows `.exe` installer or zip TBD) that include **everything needed to run** Phase 3’s combined stack without a compiler on the target machine.

**Deliverable**

- **Release checklist** and automation (e.g. GoReleaser or custom scripts) producing artifacts attached to a tag.
- **Signing / notarization** called out as **follow-up** if not implemented (especially macOS).

**Tests / acceptance criteria**

- [x] **CI** builds artifacts for **linux-amd64** at minimum; **darwin** and **windows** targets configured (may run on tag only).
- [x] **Smoke test** job: unpack artifact, run `--version` or `--help`, and optionally start with **mock** upstream (fast path).
- [x] **Documentation**: install steps per OS, antivirus/first-run notes for Windows if relevant.

**TODO (Phase 4)**

- [x] Select tooling (GoReleaser, etc.) and pin BiFrost versions per release.
- [x] Define artifact layout (binary names, `LICENSE` files, third-party notices).
- [x] Add `docs/packaging.md` for operators.

**Status:** ☐ Not started · ☐ In progress · ☑ **Complete**

---

## Phase 5 — GUI: “mew mew, Love Chimera”

**Intent.** Ship a **graphical** entry (desktop shell) that displays `mew mew, Love Chimera`. The GUI may be **Wails**, **Fyne**, or another agreed stack; it should **launch or attach** to the supervised stack from Phase 3–4 where feasible, or clearly document “GUI-only demo” until wired.

**Deliverable**

- GUI application (`gui/` Fyne module, `make gui-build`) for **macOS, Windows, Linux**. **Exception:** not yet bundled inside GoReleaser `chimera` archives (CGO / cross-compile); documented in [packaging.md](../packaging.md) and [gui-testing.md](../gui-testing.md).
- On first launch, user sees **exactly** the required message (additional UI optional).

**Tests / acceptance criteria**

- [x] **Automated UI test** where feasible (e.g. Wails/Fyne test hooks or screenshot/E2E in CI for one platform); if not feasible, **manual test script** with signed checklist in `docs/gui-testing.md`.
- [x] **Build verification**: CI builds GUI flavor for at least **linux-amd64** (headless-friendly checks as appropriate).

**TODO (Phase 5)**

- [x] Choose GUI framework; spike hello-world in repo.
- [x] Implement required message string and window sizing/accessibility basics.
- [x] Integrate with supervisor **or** document phased wiring; update packaging.

**Status:** ☐ Not started · ☐ In progress · ☑ **Complete**

---

## Phase 6 — Hardening, operator UX, and TypeScript gateway sunset

**Intent.** Security review pass, logging/redaction, upgrade path between versions, **migration guide** from Docker+LiteLLM stack, and final decision on **removing or archiving** the TypeScript implementation.

**Deliverable**

- **SECURITY.md** or equivalent notes (token handling, local attack surface).
- **Migration guide** for existing operators.
- Explicit **sunset** statement for TS gateway in README/plan.

**Tests / acceptance criteria**

- [x] **Fuzz or static analysis** (optional): `go test -fuzz=FuzzLoadGatewayYAML ./internal/config -fuzztime=30s` (config loader); expand later if desired.
- [x] **End-to-end script** (documented): `scripts/e2e-first-chat-curl.sh` at repo root (CLI; GUI does not host the API). The older `docs/e2e-operator-path.md` walkthrough was removed from the tree—use the script and [`operator-migration-to-go.md`](../operator-migration-to-go.md).
- [x] **Regression suite** from Phase 2 still green (`go test ./...`).

**TODO (Phase 6)**

- [x] Audit secrets in logs and config reload paths (documented in **SECURITY.md**; code review: `redactAuth`, token store logs count-only).
- [x] Finalize docs and archive TS server — **done:** `src/`, Compose, and LiteLLM config removed; Go-only tree.
- [x] Document **v0.x** release candidate tagging for maintainers ([packaging.md](../packaging.md)); actual tag is manual.

**Status:** ☐ Not started · ☐ In progress · ☑ **Complete**

---

## Quick reference

| Topic | Primary location after migration |
|--------|----------------------------------|
| Go entrypoint | `cmd/chimera/` (`go build -o chimera ./cmd/chimera`) |
| Discovery artifacts | `docs/bifrost-discovery.md` |
| Subprocess BiFrost + Go | `docs/supervisor.md`, `chimera serve` |
| Packaging | `docs/packaging.md` |
| GUI testing | `docs/gui-testing.md` |

---

*Last updated: 2026-04-04 — Phases 0–6 complete; see Phase completion log.*
