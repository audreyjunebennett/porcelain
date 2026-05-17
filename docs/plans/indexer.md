# Plan: Chimera file indexer (`chimera-indexer`)

| Field | Value |
|-------|-------|
| **Doc kind** | `feature-plan` |
| **Owners / areas** | Indexer, Gateway RAG, vector storage |
| **Status** | `active` |
| **Targets** | Gateway v0.2+, indexer v0.2-v0.5 |
| **Last updated** | See git history |
| **Supersedes / superseded by** | None |

## At a glance

Keep your project files searchable in chat without manual uploads. The indexer watches the folders you configure, respects `.gitignore` and `.chimeraignore`, and quietly hands files to the gateway so retrieval-aware chat can find them. Large files are handled, restarts pick up where they left off, and absolute host paths never leave your machine.

| Phase | Outcome | Status |
|-------|---------|--------|
| [v0.2 — Initial release](#indexer-v02-initial-release) | Watch one or many roots, ignore rules, whole-file ingest | `done` |
| [v0.3 — Project & flavor scopes](#indexer-v03) | Per-root and per-glob `project_id` / `flavor_id` overrides | `done` |
| [v0.4 — Large files & server hash](#indexer-v04--large-files-dual-mode-ingest--authoritative-server-hash) | Session ingest for big files; server-authoritative content hash | `done` |
| [v0.5 — Operator observability](#indexer-v05) | Structured status events; supervised under `chimera serve` and desktop | `done` |
| [v0.8 — Layered configuration](#indexer-v08-configuration-parity-with-chimeractl) | Global + project YAML files merged with flags | `done` |
| [v0.9 — Model-assisted strategy](#indexer-v09) | Optional LLM-recommended ignore / index strategy | `todo` |

---

This document plans a **portable Go binary** that watches configured directories, respects ignore rules, and sends **whole-file** bodies to the **chimera-gateway** for **server-side chunking and embedding** (same strategy as [`porcelain.plan.md`](../porcelain.plan.md): one document per request; gateway owns chunk boundaries and can change them without indexer upgrades). It complements gateway **ingest** and **indexer** APIs (`POST /v1/ingest`, `GET /v1/indexer/config`, etc.).

**Related docs:** [`cli-tool.plan.md`](cli-tool.plan.md) (configuration precedence pattern), [`porcelain.plan.md`](../porcelain.plan.md), [`overview.md`](../overview.md), [`network.md`](../network.md), [`log-presentation-layer.plan.md`](log-presentation-layer.plan.md) (operator log UX for supervised processes).

**Current code (this repo):** `chimera-indexer` is implemented and reports `chimera-indexer v0.5.0` (`--version`). Sources: `cmd/chimera-indexer`, `internal/indexer`, operator guide [`indexer.md`](../indexer.md), example [`config/indexer.example.yaml`](../config/indexer.example.yaml). Makefile targets `indexer-build` / `indexer-run` / `indexer-install` and `scripts/clean.sh` / `scripts/print-make-help.sh` include the binary. **Still missing** relative to this document: durable offline queue and optional **global discovery without cwd** edge cases. **Shipped:** layered YAML, `GET /health` during recovery, **Mode B** per-step retries, `GET /v1/indexer/corpus/inventory`, **v0.5** optional supervision under `chimera serve` / **desktop** with `--log-json`, single supervised `config_path`, BFF `/api/ui/indexer/*`, native folder picker (`chimeraPickFolder`) in the desktop webview, and operator **logs** tee (`Application: indexer`). **Planned beyond v0.5:** richer **health and update-status** reporting (structured progress fields), optional remote log shipping.

---

## Goals

1. **Security-conscious identifiers** — stable document identity and `source` metadata use **paths relative to configured workspace roots**, never absolute host paths, so payloads sent to the gateway do not leak usernames, drive letters, or internal mount layouts.
2. **Portable artifact** — single **Go** binary (`chimera-indexer` / `chimera-indexer.exe`) shipped alongside or independently of `chimera`, same cross-platform story as the gateway.
3. **Incremental indexing** — on startup, compute the watch set, **reconcile with gateway-held state** (when APIs exist), enqueue work, then run incrementally with debouncing and backpressure consistent with common file-watcher tooling.
4. **Layered configuration** — `.chimera/indexer.config.yaml` (and optional global override file) with explicit **precedence**; casual users can run with **one root** and minimal YAML.
5. **Defer complex lifecycle** — **delete/rename/tombstone** semantics follow **prior art** (e.g. OpenClaw-style agents, mature indexers) in later milestones; first indexer release focuses on **add/update** paths and documented gaps.

---

## Non-goals (initial milestones)

- **Continue** as the indexer runtime (Continue remains a **chat client**; headers must **match** indexer scope per gateway plan).
- **Embedding inside the indexer** — embeddings stay on the **gateway** (LiteLLM/BiFrost path per product plan) unless a future version explicitly adds local embed models.
- **Full VS Code UI** in early indexer releases — see [§ Visual Studio Code integration](#visual-studio-code-integration).

---

## Versioning (indexer milestones)

**Indexer and gateway v0.2 align:** the first shippable `chimera-indexer` targets **gateway v0.2** (ingest + indexer config/storage APIs). Later indexer versions may add features without a gateway bump, but **v0.2** is the shared baseline for “RAG indexing works end-to-end.” **As of the current tree**, the shipped indexer binary is **v0.4.0** and implements **v0.2 + v0.3 + most of v0.4** below; **[Indexer v0.5](#indexer-v05)** is planned (supervision + richer status). Milestone headings remain the spec history.

### Indexer v0.2 (initial release)

**Scope**

- **Tenant scope** — `tenant_id` is implied by the **gateway-issued Bearer token** (same token model as chat); no separate tenant field in YAML required.
- **Single or multiple roots** — configurable **watch roots** (directories); each root is a **security boundary** for relative paths (see [§ Stable document identity](#stable-document-identity)).
- **Ignore rules** — skip binary files; honor `.chimeraignore` (shipped template or generated defaults including entries such as `.env`); also honor `.gitignore` and, where feasible, other common `*ignore` patterns documented in config.
- **Symlinks** — default **do not follow** symlinks when walking the tree (more secure); no supported toggle in v0.2 (**current code:** a YAML `follow_symlinks` field exists but **`Resolve` forces off**).
- **Ingest unit** — **one logical file** per ingest; **v0.2** uses `POST /v1/ingest` for bodies under the whole-file cap; **v0.4** adds the **session chunk API** for larger files (see [Indexer v0.4](#indexer-v04--large-files-dual-mode-ingest--authoritative-server-hash)). **Gateway** chunks, embeds, and writes vectors (see [§ Chunking and gateway contract](#chunking-and-gateway-contract)).
- **Auth** — gateway URL and **API token from environment** (`CHIMERA_GATEWAY_URL` / `CHIMERA_GATEWAY_TOKEN`); `chimera-indexer` also loads `env` / `.env` from CWD before reading flags. No token in YAML yet.
- **Operational behavior** — **debouncing**, **coalescing**, and **backpressure** (bounded worker pool, queue depth limits); **failure handling** follows [§ Failure handling (normative)](#failure-handling-normative).

**Not in indexer v0.2** (historical scope; some items shipped in later milestones)

- Per-path **`project_id` / `workspace_id` / `flavor_id`** overrides — **shipped in v0.3** (see checklist).
- Gateway **reconciliation API** (full “list remote files + content hash”) — **still absent**; indexer uses **full local scan** plus optional **v0.4 sync state** skip (see [§ Startup reconciliation](#startup-reconciliation)).

### Indexer v0.3

**Scope**

- **`project_id` / `workspace_id`** and `flavor_id` — support **global defaults** in YAML, plus **per-root** and **per-glob** overrides (merge order documented in [§ Configuration schema](#configuration-schema-evolution)).
- **Alignment with Continue** — same values must be sent as `X-Chimera-Project` / `X-Chimera-Flavor-Id` on chat for RAG to hit the same corpus ([`porcelain.plan.md`](../porcelain.plan.md) § Client integration).

### Indexer v0.4 — large files: dual-mode ingest + authoritative server hash

**Goal:** keep **whole-file** ingest as the default (see **v0.2**), and add an optional **second path** for **large files** that would exceed HTTP body limits or waste bandwidth on retries.

**Indexer + gateway must implement both modes** (negotiated per file or per config):

1. **Mode A — whole-file** (unchanged from v0.2): single `POST /v1/ingest` per file; gateway chunks server-side.
2. **Mode B — session + ordered chunk uploads** for large bodies — **implemented** in gateway + `internal/indexer/client.go`: `POST /v1/ingest/session` (JSON `source`, `content_hash`) → `PUT /v1/ingest/session/{id}/chunk` (raw bytes, header `X-Chimera-Chunk-Index`) → `POST /v1/ingest/session/{id}/complete`. Gateway still **owns embedding and vector writes**; the split is **transport** only. Session limits (`max_chunk_bytes`, `max_total_bytes`) come from the start response.

**Configuration:** file size vs `GET /v1/indexer/config` fields `max_whole_file_bytes` and `ingest_session_path` select Mode A vs B; the indexer may cap whole-file size further with YAML `max_whole_file_bytes`.

**Content hash (this milestone):**

- **v0.2–v0.3:** **client-computed SHA-256** (or agreed algorithm) is the **source of truth** the indexer uses for change detection and sends on ingest; reconciliation compares **local client hash** to **remote stored hash** from inventory when available.
- **v0.4 adds:** gateway **computes hash over the bytes it actually ingested** (after decoding/normalization as defined in the contract) and returns `content_sha256` (name TBD) in the **ingest response** (and persists it for **corpus inventory**). Indexer **updates local bookkeeping** to that value so **server truth** can override client preflight hash when they differ (normalization, transcoding, or bug diagnosis).

**Deliverables:** documented APIs for Mode B, size thresholds, error/retry semantics per chunk/session, and **response body** fields for **server-side SHA**. **Status:** gateway and client implement start/chunk/complete and ingest JSON responses include `content_sha256` / `client_content_hash`; **per-step bounded retries** (session start, each **PUT** chunk, **complete**) reuse the same backoff caps as whole-file ingest **within** one `processJob` attempt. If all attempts fail, the outer job retry may still restart the session from byte zero.

### Indexer v0.5

**Operator observability, supervised `chimera serve`, and desktop.** Operators (and the **log presentation layer**) should see **what the indexer is doing** without tailing a separate terminal: **health**, **update / sync status**, **backoff and recovery timing**, and **high-level progress**. The indexer runs as an **optional child** of `chimera serve` and of the **desktop** stack so stdout/stderr is captured the same way as BiFrost and Qdrant today.

**Motivation:** today `chimera-indexer` is a **standalone** process logging to `stderr`; the gateway `/ui/logs` buffer only receives `gateway`, `bifrost`, and `qdrant` lines when using `chimera serve` (`servicelogs` writers in `cmd/chimera/serve.go`). Indexer traffic appears indirectly as `gateway` HTTP access logs for `/v1/ingest`, not as first-class **indexer** narrative. v0.5 closes that gap for supervised deployments and improves **structured** signals for summarization (see [`log-presentation-layer.plan.md`](log-presentation-layer.plan.md)).

**Scope**

1. **Structured status and health reporting (indexer process)**  
   - Emit **stable, parse-friendly** log events (prefer **`slog` JSON** on stderr for supervised mode, or document equivalent key/value fields) for milestones an operator cares about, for example:
     - **Run lifecycle:** `indexer.run.start` / `indexer.run.ready` (roots, config fingerprint or gateway URL host only—no secrets), optional `index_run_id` (UUID) for UI threading.
     - **Discovery / reconciliation:** counts after initial scan—**candidate files**, **skipped** (unchanged inventory, sync state, ignores), **enqueued for upload** (“proposed updates”); per-root breakdown optional.
     - **Incremental watch:** watcher **attached** roots, `debounce_ms` effective; on **debounced** enqueue, optional **rate-limited** “file changed” summary lines (not one log per event in a storm).
     - **Queue + workers:** periodic or threshold-based `queue_depth`, `workers`; **ingest success/fail** tallies over a window (optional).
     - **Backoff / retry:** when `ingest retry` fires, include **explicit** `next_retry_in` / `delay` and **reason class** (429, 5xx, network); when **paused** for health, log `recovery_poll_interval`, **next_poll_at** (or equivalent), and **which probes** run (`storage/health` vs `/health`).
   - **Security:** unchanged rules—no absolute paths in payloads to the gateway; logs may use **relative** paths consistent with today; no tokens in structured fields.

2. **Supervised `chimera serve`**  
   - **Optional** indexer child process (off by default), controlled from `gateway.yaml` (or dedicated snippet) and/or CLI flags, for example:
     - Path to `chimera-indexer` binary (default: next to `chimera` or on `PATH`).
     - **Working directory** and/or explicit `--config` path for layered YAML.
     - **Environment:** inherit `CHIMERA_GATEWAY_URL` / `CHIMERA_GATEWAY_TOKEN` (or map from gateway’s token store path only if a safe pattern is defined—prefer env inherited from the parent process).
   - **Supervision:** same pattern as Qdrant/BiFrost: `context` cancellation on gateway shutdown; **`Stdout` / `Stderr`** teed to `logStore.Writer("indexer")` so `/api/ui/logs` shows `Application: indexer` lines live.
   - **Bootstrap / RAG off:** do not start the indexer when the gateway is in **bootstrap** mode or **RAG disabled** unless explicitly overridden; document behavior when storage health is **degraded** (indexer may still run and self-pause per existing recovery logic).

3. **Desktop bundle (`chimera` desktop mode)**  
   - When the **desktop** entry (`chimera serve` + webview per [`ui-tool.plan.md`](ui-tool.plan.md)) is used, **reuse the same supervision block**: if indexer supervision is enabled in config, the **desktop** process starts it and tees logs to `servicelogs`—no second packaging story. **Release bundles** that ship `chimera` + `bifrost-http` + `qdrant` should **optionally** ship `chimera-indexer` beside them and document `gateway.yaml` keys to turn it on.

**Non-goals (v0.5)**

- Replacing **gateway** ingest metrics with indexer-reported truth (gateway remains authoritative for stored corpus).
- **Remote** log shipping (Splunk, etc.)—only in-process ring buffer + UI as today.
- **Durable offline queue** (still [open decisions](#open-decisions) / later milestone).

**Deliverables:** config schema snippet in `config/gateway.example.yaml` (or `indexer.supervised` block), operator notes in `docs/indexer.md`, and checklist items below. **Version string:** bump `chimera-indexer --version` to **v0.5.0** when this milestone ships.

### Indexer v0.8+ (configuration parity with `chimeractl`)

- **Layered config files** mirroring [`cli-tool.plan.md`](cli-tool.plan.md): global `~/.chimera/indexer.config.yaml`, optional flags — see [§ Configuration precedence](#configuration-precedence).

### Indexer v0.9

- **Model-assisted indexing strategy** — optional flow: indexer (or a companion tool) sends a **directory tree summary**, **effective ignore sets**, and **config** to a **gateway or LLM** endpoint and receives a **recommended indexing strategy** (patterns, priorities, exclusions). Normative API shape is **TBD**; depends on gateway/tooling roadmap.

### Visual Studio Code integration

**Later releases** (not tied to a single indexer semver in this draft):

1. **Early extension** — surface **progress**, **logs**, and **errors** from the running `chimera-indexer` process (spawned by the extension or attached to an existing process).
2. **Config assistance** — open or generate `.chimera/indexer.config.yaml` with **sensible defaults**, wizards, or **prompt text** the user can paste to an assistant to produce a config.
3. **Richer UX** — status views, queue depth, per-root health, links to gateway storage stats.
4. **Multi-project RAG** — help users run or attach indexers for **other workspaces** / corpora (organization-dependent).

---

## Stable document identity

- **Canonical id** — derived from `(tenant_id, root_id, path_relative_to_that_root, content_hash)` where:
  - `tenant_id` comes from the token (server-side); indexer does not send raw tenant in path ids unless the gateway contract requires it in payload.
  - `root_id` is a stable slug for each configured watch root (**implementation:** basename-derived slug in logs only via `internal/indexer/config.go`; **not** sent on the wire). Config-defined or hashed root ids remain a possible future refinement.
  - `path_relative_to_that_root` is the **only** path form stored in `source` and used for human-readable citations.
  - `content_hash` — **cryptographic hash** of file bytes (e.g. **SHA-256**).
    - **Indexer v0.2–v0.3:** computed **on the client**; treated as **truth** for **local change detection** and sent with ingest so the gateway can **store** it for inventory (exact header or JSON field name is part of the ingest contract). Reconciliation uses **client hash vs remote stored hash** when the inventory API exists.
    - **Indexer v0.4+:** gateway **also** computes hash over **canonical ingested bytes** and returns it in the **ingest response**; indexer **prefers server-reported SHA** for persisted sync state when present (see [Indexer v0.4](#indexer-v04--large-files-dual-mode-ingest--authoritative-server-hash)).
- **Absolute paths** must not appear in **HTTP bodies** or logs in production modes; debug logging may redact or hash paths.

This keeps **multi-root** setups correct while avoiding **cross-machine path leakage**.

---

## Deletes, renames, and corpus lifecycle

**Indexer v0.2 — Explicitly deferred.** Behavior is **undefined** beyond “best effort”: renamed file may appear as **delete + add** once lifecycle APIs exist.

**Future** — adopt patterns from **mature indexers** and **agent platforms** (e.g. OpenClaw-style tooling) for:

- Tombstones vs hard deletes in the vector store.
- **Rename detection** (inode / content hash heuristics).
- Gateway support for **delete-by-source** or **replace-collection** operations.

Track as **open design** once gateway exposes stable operations beyond ingest.

---

## Configuration file

### Primary path

- **Repository / workspace local:** `.chimera/indexer.config.yaml`

(If a repo already uses `.chimera/` for other Chimera artifacts, keep a single subdirectory layout documented in the main README when implemented.)

### Configuration precedence

Aligned with [`cli-tool.plan.md`](cli-tool.plan.md) where applicable.

**Indexer v0.8** — **YAML file merge implemented** in `LoadLayeredConfig` / `MergeFileConfig`:

1. **Built-in defaults** (compiled into the binary; applied in `Resolve`).
2. **Global user config:** `~/.chimera/indexer.config.yaml` (`os.UserHomeDir()`; Windows `%USERPROFILE%\.chimera\…`).
3. **Local project config:** `<cwd>/.chimera/indexer.config.yaml`.
4. `--config path` — merged **last** when provided (file must exist).

**Merge rule:** later YAML files override earlier for the same key; see `MergeFileConfig` for field-wise rules.

**After merged YAML:** `CHIMERA_GATEWAY_URL` overrides `gateway_url`; `--gateway-url` / `--root` override merged YAML for URL and roots. `CHIMERA_GATEWAY_TOKEN` is **only** from the environment (never YAML). On startup, `cmd/chimera-indexer` loads `env` then `.env` from the **current working directory** (missing files ignored), matching the main `chimera` binary. Operator summary: [`indexer.md`](../indexer.md).

### Configuration schema (evolution)

**v0.2 — minimal** (**implemented** in `FileConfig` plus v0.4 fields below)

- `gateway_url` (or env `CHIMERA_GATEWAY_URL`; CLI `--gateway-url` wins)
- `roots`: list of directory paths to watch (or CLI `--root` replaces the list)
- Optional `ignore_extra`: list of glob patterns added to `.chimeraignore` semantics
- **Backoff / recovery** — optional overrides for [§ Failure handling (normative)](#failure-handling-normative): `retry_max_attempts`, `retry_base_delay_ms`, `retry_max_delay_ms`, `recovery_poll_interval_ms`, optional `recovery_include_root_health` (default **true**: recovery also waits on `GET /health`)
- **Operational:** `debounce_ms`, `workers`, `queue_depth`, `max_file_bytes`, `request_timeout_ms`, optional `binary_null_byte_sample_bytes` / `binary_null_byte_ratio`
- **v0.4 additions:** `sync_state_path` (default: next to `--config` when set, else `.chimera/indexer.sync-state.json`), `max_whole_file_bytes`

**v0.3 — scoped overrides**

```yaml
# Implemented shape (see config/indexer.example.yaml).
defaults:
  project_id: "my-app"
  flavor_id: "default"

roots:
  - path: "./apps/web"
    project_id: "web"
  - path: "./legacy"
    flavor_id: "legacy-corpus"

overrides:
  - glob: "**/*.md"
    flavor_id: "docs"
```

**Later** — per-glob **lifecycle** and **priority** rules (queue ordering, inclusion/exclusion strategies).

---

## Ignore rules

1. **Binary detection** — skip non-text files via extension allowlist/denylist + content sniff where appropriate.
2. `.chimeraignore` — first-class file with **sensible defaults** (e.g. `.env`, secrets, large artifacts); `chimera-indexer init` (future) may generate it.
3. `.gitignore` — honored when present (reuse a well-tested Go library or stdlib + gitignore parser).
4. **Other `*ignore` files** — optional phased support; document which names (e.g. `.dockerignore`) are honored per release.

---

## Chunking and gateway contract

**Product decision (this plan):** the **indexer sends the whole file** (multipart `file` and/or JSON fields per gateway schema); the **gateway** applies `chunk_size`, `chunk_overlap`, **embedding**, and **Qdrant** writes. That matches [`porcelain.plan.md`](../porcelain.plan.md) (**one document per request**; gateway chunking defaults configurable and surfaced via `GET /v1/indexer/config`).

**Rationale:** the service works at **file** (document) granularity for ingest APIs and storage evolution; chunking strategy can improve **without** shipping a new indexer.

**Indexer responsibilities (v0.2):** read file bytes, compute **client `content_hash`**, set `source` to the **relative path**, call `POST /v1/ingest`; obey **max request size** limits — files over the limit are **skipped or errored** until [Indexer v0.4](#indexer-v04--large-files-dual-mode-ingest--authoritative-server-hash) **dual-mode** ingest exists.

**Future (v0.4):** same logical **file** may use **whole-file** or **streaming/chunked** transport per threshold; gateway remains responsible for **chunking for embedding** after assembly.

Embedding model and vector dimensions remain **gateway-owned**; indexer **must** refresh config when `GET /v1/indexer/config` reports changes (see [§ Version skew and embedding settings](#version-skew-and-embedding-settings)).

---

## Failure handling (normative)

For **ingest failures** (transient HTTP errors, **503**, **429**, network errors) where the response does **not** explicitly require the client to **stop permanently** (contrast: **401** / **403** — treat as **fatal / operator action**, do not infinite-retry):

1. **Retry with exponential backoff** — **configurable** `retry_max_attempts` (small integer, e.g. default **5**), `retry_base_delay`, `retry_max_delay` (cap per wait). Jitter optional. Apply per failing operation or per batch per implementation, but **must** bound total attempts.
2. **After the last backoff attempt fails** — **pause** the ingest **queue** (do **not** discard queued work; continue **collecting** filesystem events if desired, subject to backpressure limits).
3. **Recovery polling** — while paused, periodically call gateway **status** endpoints to determine whether **ingest / RAG storage** is available again:
   - `GET /v1/indexer/storage/health` (Bearer token; scoped per [`porcelain.plan.md`](../porcelain.plan.md) **indexer REST**) — **implemented**; client parses `ok`, `status`, `detail`, and the structured `{"error":{...}}` shape when RAG is disabled.
   - `GET /health` for overall gateway / upstream readiness — **implemented** when `recovery_include_root_health` is **true** (default); set `false` to use only storage health.
4. **Resume** when responses indicate **healthy / not degraded** for the paths relevant to ingest (exact JSON fields documented with gateway implementation). **Reset** backoff state for subsequent failures.

**Configurable** `recovery_poll_interval` (e.g. default **30s**) governs how often to poll while paused.

Document defaults and env overrides in the indexer **README** when implemented.

---

## Authentication

- **v0.2:** Bearer token from **environment** (`CHIMERA_GATEWAY_TOKEN`). `chimera-indexer` auto-loads `env` and `.env` from the process CWD (see [§ Configuration precedence](#configuration-precedence)), so tokens can live in `.env` without a separate shell export step.
- **Later:** read token (or path to token file) from **YAML** per [§ Configuration precedence](#configuration-precedence); never commit secrets; recommend `.gitignore` for `.chimera/indexer.config.yaml` when it holds tokens.

---

## Path allowlist and symlinks

- **v0.2:** Only index under configured `roots`; **do not follow symlinks** by default when enumerating files.
- **Later:** configuration toggle to **follow symlinks** with explicit warning in docs (security + duplicate path risk).

---

## Startup reconciliation

**Desired behavior**

1. On start, compute the **candidate file set** from all roots (after ignores).
2. Call the gateway (or indexer API) to obtain **remote inventory** for the authenticated **tenant** (and, from **v0.3**, **project** / **flavor** scope): e.g. **paths + `content_hash`** the gateway stores or aggregates from Qdrant payload.
3. Compute **diff**: enqueue **uploads** for missing files or paths whose **local hash ≠ remote hash**.
4. Run workers with **backpressure**; transient failures follow [§ Failure handling (normative)](#failure-handling-normative).

**Startup reconciliation:** `GET /v1/indexer/corpus/inventory` (paginated; see [`indexer.md`](../indexer.md)) lists `source` + `content_sha256` + optional `client_content_hash` for the authenticated tenant/project/flavor. `chimera-indexer` merges all pages after `GET /v1/indexer/config` and uses the map to **skip** unchanged files when hashes match (see implementation in `internal/indexer`). **Multi-root** setups that share one corpus and reuse the same relative `source` string can collide — use disjoint paths or separate projects.

---

## Version skew and embedding settings

On **every startup** (and periodically during long runs), the indexer **SHOULD** call `GET /v1/indexer/config` with the same **Bearer token** and (from **v0.3**) **`X-Chimera-Project` / `X-Chimera-Flavor-Id`** as appropriate.

**Use returned fields for:**

- `embedding_model`, `chunk_size`, `chunk_overlap` (inform logging / version skew only; **indexer does not chunk**), `ingest_path`, required headers.
- `gateway_version` — log and optionally trigger **full reindex** if major embedding/collection rules change.

**Optional future:** same response (or `GET /v1/indexer/storage/stats`) includes **point counts** or **per-corpus checksums** to inform reconciliation (depends on gateway implementation).

**v0.4+:** `GET /v1/indexer/config` (or ingest response) may advertise `max_whole_file_bytes` and **dual-mode** capability flags so the indexer selects Mode A vs B without hardcoding.

---

## Binary and module layout

| Item | Proposal |
|------|----------|
| **Go package** | `cmd/chimera-indexer` |
| **Artifact name** | `chimera-indexer` (Unix), `chimera-indexer.exe` (Windows) |
| **Shared logic** | `internal/indexer/*` — config load/merge, ignore engine, hashing, queue, gateway client |
| **Import path** | Same module as gateway (`go.mod` at repo root) unless packaging later splits modules |

---

## Makefile (**implemented** in repo root `Makefile`)

| Target | Behavior |
|--------|----------|
| `make indexer-build` | `go build -o chimera-indexer[.exe] ./cmd/chimera-indexer` |
| `make indexer-run` | `go run ./cmd/chimera-indexer` (pass flags via `ARGS=...`) |
| `make indexer-install` | `go install ./cmd/chimera-indexer` |

`scripts/print-make-help.sh` and `scripts/clean.sh` list / remove `chimera-indexer[.exe]` alongside `chimera` / `chimera-desktop`.

---

## Testing

- **Unit:** present under `internal/indexer/*_test.go` — ignore matching, walker, hashing, queue, scope merge, debounce, config resolve, gateway client (`httptest` for config / ingest / health / session flows).
- **Integration:** expand with testcontainers or a live gateway fixture when useful; not required for current CI coverage of the indexer package.

---

## Documentation deliverables

- `docs/indexer.md` — operator quick start: install via `make`, env vars, example config path, **v0.3** headers, **v0.4** sync state, failure defaults, `--one-shot`.
- **Root `README.md`** — still optional; indexer is discoverable via `docs/indexer.md` and `make help`.
- **Security** — no absolute paths in payloads; symlink default; secret handling (see [`indexer.md`](../indexer.md)).
- **Gateway API** — whole-file ingest + session chunk paths live in `internal/server`; a single consolidated “indexer HTTP contract” doc for operators (inventory API still open) would still add value.

---

## Open decisions

1. **Large files** — **Mode B** addresses over-limit bodies; remaining product choice: behavior when a file exceeds `max_ingest_bytes` / session `max_total_bytes` (**skip** vs **fail loud** vs user-visible metric).
2. **v0.4+ — Mode B resilience** — idempotency keys, **resume after partial chunk failure** without full-file redo, session TTL semantics; must stay aligned with gateway.
3. **Corpus inventory endpoint** — schema (**path key**, `content_hash`, pagination); **authz** per tenant/project/flavor; **v0.4** stores **server-computed** hash for truth after ingest.
4. **Delete/rename** — first gateway primitive (tombstone, delete-by-filter, or reindex-only).
5. **Durable queue format** — SQLite vs JSONL vs embedded store for offline resilience while **paused**.
6. **Binary name** — settled on `chimera-indexer` for `make` and docs unless packaging introduces an alias.

---

## Implementation checklist (summary)

**Indexer v0.2**

- [x] `cmd/chimera-indexer`: `--config` YAML, env-based token, watch roots (`roots` / `--root`), ignores (built-ins + `ignore_extra` + `.chimeraignore` + `.gitignore`), **no symlink follow** (YAML `follow_symlinks` exists but **`Resolve` forces false**).
- [x] **Whole-file** ingest; `content_hash` as `sha256:<hex>` for local change detection and gateway echo.
- [x] Gateway HTTP client: `GET /v1/indexer/config`, `POST /v1/ingest`, `GET /v1/indexer/storage/health` — [§ Failure handling (normative)](#failure-handling-normative) backoff + pause + recovery poll **implemented** in `internal/indexer/indexer.go`.
- [x] Optional `GET /health` during recovery — `CheckGatewayRootHealth`; gated by `recovery_include_root_health` (default **true**).
- [x] Debounced change handling (`fsnotify` + `debounce_ms`), worker pool, bounded queue / backpressure.
- [x] Stable **relative** `source` (no absolute paths on wire).
- [x] Makefile targets + `scripts/print-make-help.sh` + `scripts/clean.sh`.
- [x] Operator docs: `docs/indexer.md` + `config/indexer.example.yaml` (root `README.md` does not yet summarize the indexer).

**Indexer v0.3**

- [x] `project_id` / `flavor_id` / `workspace_id` in YAML: defaults, per-root, per-glob (`internal/indexer/scope.go`).
- [x] Send `X-Chimera-Project` / `X-Chimera-Flavor-Id` on ingest and on default `GET /v1/indexer/config` fetch; documented in `docs/indexer.md`.

**Indexer v0.4**

- [x] **Dual-mode ingest:** whole-file (Mode A) + session chunk path (Mode B); threshold from YAML and/or `GET /v1/indexer/config`.
- [x] Parse **ingest response** `content_sha256`; persist client + server digests in `sync_state_path` (default next to `--config` when set, else `.chimera/indexer.sync-state.json`) for skip-if-unchanged.
- [x] `--one-shot` scan mode and `--version`.
- [x] **Mid-session HTTP retries:** each Mode B step (**POST** session start, **PUT** chunk, **POST** complete) uses bounded exponential backoff (`retry_*` fields) before the worker exhausts attempts and pauses.

**Indexer v0.5**

- [x] **Structured operator events:** discovery/reconciliation **summaries** (candidate / skipped / enqueued counts), **queue/worker** snapshots, **retry/backoff** and **recovery poll** timing fields suitable for [`log-presentation-layer.plan.md`](log-presentation-layer.plan.md); `index_run_id` on every line via slog `With`. See **[`indexer.md`](../indexer.md) § Structured operator logs** and `internal/indexer/ops_events.go` + milestone `msg` values in `internal/indexer/indexer.go` / `cmd/chimera-indexer/main.go`.
- [x] **`chimera serve`:** optional supervised `chimera-indexer` subprocess with **stderr/stdout** teed to `servicelogs` source `indexer`; shutdown with gateway; gated when **bootstrap** or **RAG off** unless `start_when_rag_disabled`.
- [x] **Gateway config + docs:** `gateway.yaml` / `gateway.example.yaml` documents supervision flags; operator UI `/ui/indexer` + `/api/ui/indexer/*` for the single supervised `config_path` file.
- [x] **Desktop:** desktop webview `chimeraPickFolder` (native directory dialog via `dlgs`) for the Indexer tab; same supervision path as `chimera serve` when enabled.
- [x] **Version:** `chimera-indexer --version` reports **v0.5.0**.

**Indexer v0.8**

- [x] Layered YAML merge: `~/.chimera/indexer.config.yaml` → `<cwd>/.chimera/indexer.config.yaml` → `--config` (`LoadLayeredConfig`). CLI/env overrides unchanged.

**Indexer v0.9**

- [ ] Optional LLM-assisted strategy generation (API TBD).

**Gateway coordination (not indexer-only)**

- [x] `POST /v1/ingest` — **whole-file** document schema (v0.2); **server-side** chunking per existing gateway plan; accept **client** `content_hash` (echoed as `client_content_hash`); authoritative `content_sha256` over ingested UTF-8 bytes.
- [x] `GET /v1/indexer/corpus/inventory` — paginated corpus rows (`source`, `content_sha256`, optional `client_content_hash`); Qdrant scroll + dedupe per page; indexer client + startup skip logic.
- [x] `GET /v1/indexer/storage/health` — implemented and used for **resume**; **operator-facing field reference** still thin vs “document defaults” checklist item.
- [x] `GET /health` — consumed when `recovery_include_root_health` is true (default).
- [x] **v0.4:** **Mode B** `POST /v1/ingest/session` + `PUT .../chunk` + `POST .../complete`; **compute and return** canonical **server SHA** on success; advertise limits in `GET /v1/indexer/config` (`max_whole_file_bytes`, `ingest_session_path`, path templates).

---

*Plan status: **implemented through v0.5 supervision** (optional child under **`chimera serve` / desktop**, `servicelogs` `indexer` source, `--log-json`, single supervised `config_path`, UI + native folder picker). **Structured operator events (v0.5 checklist):** discovery / reconcile / queue / retry / recovery / run-done counters are logged as JSON **slog** lines (`msg` slugs). **Outstanding:** durable paused queue, root README blurb, smarter **session resume** after abandoning a partially uploaded session mid-flight.*
