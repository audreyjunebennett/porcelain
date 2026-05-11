# Plan: Operator-facing Qdrant log classification

| Field | Value |
|-------|-------|
| **Doc kind** | `feature-plan` |
| **Owners / areas** | Gateway supervision (`internal/supervisor`), logs UI (`internal/server/embedui/logs`), parse/derive (`internal/server/embedui/logs/parse`, `derive`), desktop mirror (`internal/servicelogs`) |
| **Status** | `shipped` |
| **Targets** | Gateway plus supervised Qdrant with JSON logging only (`QDRANT__LOGGER__FORMAT=json`) |
| **Last updated** | 2026-05-08 |
| **Supersedes / superseded by** | None |

## At a glance

Operators see **classified Qdrant output** in the logs UI: every supervised line carries a stable **`qdrant.*`** slug and structured fields, so the **Qdrant** service card and **indexer** workspace cards can show plain-language subtitles, summary key-values, and HTTP counters instead of raw Rust targets or unparsed access lines.

**Related docs:** [`supervisor.md`](../supervisor.md), [`log-presentation-layer.md`](log-presentation-layer.md), [`docs/indexer.md`](../indexer.md).

| Phase | Outcome | Status |
|-------|---------|--------|
| [Phase 1 — Spec and frozen taxonomy](#phase-1--spec-and-frozen-taxonomy) | Frozen `qdrant.*` list, ingest boundary, and UI contract documented | `done` |
| [Phase 2 — Parse and normalization](#phase-2--parse-and-normalization) | `internal/servicelogs/qdrantline` emits `msg` plus flattened attrs on ingest | `done` |
| [Phase 3 — Dual routing](#phase-3--dual-routing) | Collection name maps Qdrant lines onto matching indexer workspace cards | `done` |
| [Phase 4 — Qdrant service card UI](#phase-4--qdrant-service-card-ui) | KV row, counters, operator subtitle; no legacy RAG mini-cards or timeline in Qdrant panel | `done` |
| [Phase 5 — Indexer card UI](#phase-5--indexer-card-ui) | Collection status plus merged events with qdrant badge where mapped | `done` |

---

## Background

Supervised Qdrant uses **`QDRANT__LOGGER__FORMAT=json`** so each tracing event is one JSON object per line (optional non-JSON banner or version lines may appear first). The desktop mirror keeps **`timestamp<TAB>source<TAB>payload`** (`internal/servicelogs/store.go`). The logs UI parses payloads with `ClaudiaLogs.parseLogText` (`internal/server/embedui/logs/parse/parseLogText.js`); nested JSON flattens to dot keys. After normalization, **every** Qdrant-derived row exposed to the UI carries **`msg`** with pattern **`qdrant.*`**, matching indexer **`slog`** lines.

---

## Phase 1 — Spec and frozen taxonomy

**Goal.** Lock slug names, classification rules, and operator-visible behavior so gateway, mirror, and UI share one contract.

**Deliverables**

- Canonical **`qdrant.*`** taxonomy (table below) with detection notes and operator subtitles or KV mapping.
- Locked decisions for collection naming, fan-out, ingest location, HTTP counter semantics, restart window, and Qdrant-only timeline suppression (see table).
- Reference log captures under repo-root **`temp/`** (gitignored): [`temp/qdrant-logs-1.log`](../../temp/qdrant-logs-1.log), [`temp/qdrant-logs-1.operator-prefixed.log`](../../temp/qdrant-logs-1.operator-prefixed.log), [`temp/claudia-desktop-qdrant-only.log`](../../temp/claudia-desktop-qdrant-only.log).

**Acceptance**

- Taxonomy covers startup, TLS, telemetry, cluster hints, HTTP verbs used by the indexer, and fallback trace or unparsed lines.
- [`supervisor.md`](../supervisor.md) documents that normalization runs at supervised ingest (`qdrantline`).

**Status:** `done`

### Locked decisions (2026-05-07)

| Topic | Decision |
|-------|-----------|
| Collection ↔ indexer card | Derive Qdrant collection name from tenant / project / flavor using the same rules as `internal/vectorstore/vectorstore.go` `CollectionName` (browser: `derive/qdrantCollection.js`). |
| Fan-out | Matching Qdrant lines appear on **every** indexer card whose coords resolve to that collection name. |
| Normalization location | **On ingest:** `internal/servicelogs/qdrantline` wraps the qdrant line writer (`cmd/claudia/serve.go`) so in-memory buffer and desktop mirror receive enriched JSON. |
| HTTP success | Summaries show real status codes; only **200** counts as success for upsert, delete, and search counters; non-200 upserts emit **`qdrant.http.points_upsert_rejected`**. |
| Counter window | Aggregates use lines **at or after** the last **`qdrant.version`** in the buffer (Qdrant restart while Claudia stays up). Claudia restart clears the ring buffer. |
| Timeline | Only the expanded **Qdrant** service panel omits the request timeline and bar. |

### Canonical `msg` taxonomy

Stable machine slug pattern: **`qdrant.<segment>.<segment>…`** (aligned with **`indexer.*`**). Detection uses JSON **`target`**, **`fields.message`**, and embedded Apache-style HTTP fragments inside access logs.

| `msg` | Typical detection | Notes |
|-------|-------------------|--------|
| `qdrant.startup.banner` | Non-JSON ASCII logo lines | Card subtitle: **Starting up …**. |
| `qdrant.version` | Plain line `Version: …` | Summary KV **version**. |
| `qdrant.web_ui_hint` | Plain line `Access web UI at …` | Hint only; no KV change. |
| `qdrant.config.optional_missing` | `qdrant::settings`, config file not found | KV **configuration** = **`supervised`**. |
| `qdrant.consensus.raft_load` | raft / consensus load | Subtitle: **Loading collections …**. |
| `qdrant.collection.loading` | `Loading collection:` | Subtitle + **collection total** counter. |
| `qdrant.shard.recover_progress` | `Recovering shard` | Subtitle + progress detail. |
| `qdrant.shard.recovered` | `Recovered collection` | Subtitle + **collection loaded** counter. |
| `qdrant.cluster.single_node` | `Distributed mode disabled` | KV **mode** = **`single-node`**. |
| `qdrant.listen.tls_disabled_rest` | `TLS disabled for REST API` (`qdrant::actix`) | KV **TLS (REST)** = **`disabled`**. |
| `qdrant.listen.tls_enabled_rest` | `TLS enabled for REST API` (`qdrant::actix`) | KV **TLS (REST)** = **`enabled`**. |
| `qdrant.listen.http` | `HTTP listening` / `Qdrant HTTP listening` | KV **port (REST/gRPC)** REST component. |
| `qdrant.listen.tls_disabled_grpc` | `TLS disabled for gRPC API` (`qdrant::tonic`) | KV **TLS (gRPC)** = **`disabled`**. |
| `qdrant.listen.tls_enabled_grpc` | `TLS enabled for gRPC API` (`qdrant::tonic`) | KV **TLS (gRPC)** = **`enabled`**. |
| `qdrant.listen.grpc` | `gRPC listening` (public API) | KV **port** gRPC component. |
| `qdrant.listen.internal_grpc` | `Qdrant internal gRPC listening on …` | Internal cluster port in **`internal_grpc_port`**; debug-level in typical setups. |
| `qdrant.cluster.internal_tls_disabled` | `TLS disabled for internal gRPC API` | KV **TLS (internal)** = **`disabled`** (`qdrant_internal_tls`). |
| `qdrant.cluster.internal_tls_enabled` | `TLS enabled for internal gRPC API` | KV **TLS (internal)** = **`enabled`**. |
| `qdrant.grpc.endpoint_disabled` | `gRPC endpoint disabled` | Public gRPC off by config; not an error. |
| `qdrant.telemetry.enabled` | `Telemetry reporting enabled` | KV **telemetry** = **`enabled`**. |
| `qdrant.telemetry.disabled` | `Telemetry reporting disabled` | KV **telemetry** = **`disabled`**. |
| `qdrant.hardware_reporting.enabled` | `Hardware reporting enabled` | High-signal opt-in feature line. |
| `qdrant.inference.disabled` | `Inference service is not configured` | Informational. |
| `qdrant.inference.configured` | `Inference service is configured` | Informational. |
| `qdrant.storage.recovery_mode` | `Qdrant is loaded in recovery mode` | KV **recovery** = **`active`**; degraded startup. |
| `qdrant.cluster.bootstrap_uri_duplicate` | Bootstrap URI vs peer mismatch warnings | Classifier matches lines containing **`bootstrap uri`** plus **same**, **equal**, **peer**, or **duplicate** (exact wording varies by release). |
| `qdrant.process.server_start_failed` | `Error while starting … server:` | Offline or failing signal; detail in **`progress_detail`**. |
| `qdrant.runtime.panic` | `Panic occurred` / `Panic backtrace` | Failing signal. |
| `qdrant.gpu.init_failed` | `Can't initialize GPU` | Optional GPU path failed. |
| `qdrant.runtime.init_file_warning` | init file indicator create/remove failed | Startup filesystem warning. |
| `qdrant.security.jwt_rbac_warning` | `JWT RBAC` or JWT plus API key configuration warnings | Misconfiguration warning. |
| `qdrant.process.shutdown_signal` | `Stopping … on SIGINT` / `SIGTERM` | Shutdown path (often debug). |
| `qdrant.debug.feature_flags` | `Feature flags:` | Debug. |
| `qdrant.debug.collection_loaded` | `Loaded collection:` (debug) | Distinct from **`Loading collection:`** toc line. |
| `qdrant.ui.static_missing` | `qdrant::actix::web_ui` | Web UI static folder missing or disabled. |
| `qdrant.actix.workers` | `actix_server::builder` | HTTP worker pool. |
| `qdrant.actix.bind` | `actix_server::server` | HTTP bind. |
| `qdrant.http.collection_meta` | `GET /collections/{slug}` (no points) | Subtitle + indexer **Reading** (counter box uses upsert, delete, search only). |
| `qdrant.http.points_upsert_ok` | `PUT …/points`, HTTP 200 | Upsert success counter. |
| `qdrant.http.points_upsert_rejected` | `PUT …/points`, non-200 | Upsert fail counter. |
| `qdrant.http.points_delete` | `POST …/points/delete` | Delete counters; indexer **Deleting**. |
| `qdrant.http.vector_search` | `POST …/points/search` | Search counters; indexer **Searching**. |
| `qdrant.http.access_other` | Access log line, unmatched route | Traffic still visible; no indexer routing. |
| `qdrant.trace.other` | JSON line with no specific rule | Raw message preserved in **`progress_detail`** for operators. |
| `qdrant.unparsed` | Non-JSON after trim, or JSON decode error | Raw payload in **`progress_detail`**. |

**HTTP status labeling:** use **200** as success and non-**200** as fail for compact subtitles unless standardized otherwise.

The indexer emits **`slog`** with **`"msg", "<dotted.slug>"`**; Qdrant matches that contract **after** child JSON is parsed.

---

## Phase 2 — Parse and normalization

**Goal.** Turn each raw Qdrant stdout line into one JSON object with **`msg`**, **`service":"qdrant"`**, and structured fields the UI and tests can rely on.

**Deliverables**

- [`internal/servicelogs/qdrantline/normalize.go`](../../internal/servicelogs/qdrantline/normalize.go) — `NormalizePayload`, HTTP access classification, `classifyOperatorSignals`, idempotent **`_claudia_norm`** tagging.
- Wire-up via [`cmd/claudia/serve.go`](../../cmd/claudia/serve.go) writer wrapper (`NewWriter`).
- Derive helpers in [`internal/server/embedui/logs/derive/qdrantCollection.js`](../../internal/server/embedui/logs/derive/qdrantCollection.js): `qdrantOperatorLine`, `qdrantCardModel`, `qdrantIndexerCollectionStatusLabel`, slice-after-version window.
- SHA1 helper [`internal/server/embedui/logs/derive/sha1.js`](../../internal/server/embedui/logs/derive/sha1.js) for collection naming parity.

**Acceptance**

- Unit tests in [`internal/servicelogs/qdrantline/normalize_test.go`](../../internal/servicelogs/qdrantline/normalize_test.go) cover representative slugs (config, HTTP upsert, telemetry, TLS fallbacks, bootstrap variants, JWT or API key warnings).
- Forward-compatible unknown **`qdrant.*`** slugs still produce readable operator text in the UI (friendly fallback from slug segments).

**Status:** `done`

---

## Phase 3 — Dual routing

**Goal.** Operations that name a **collection** appear on the global Qdrant card and on every indexer workspace whose resolved collection name matches.

**Deliverables**

- Parse **`collection`** from HTTP paths (`/collections/{slug}/…`) or shard recovery text in **`qdrantline`**.
- Join indexer cards using the same naming as the gateway indexer (`qdrantCollectionName` / workspace meta).

**Acceptance**

- If a line cannot be mapped to a workspace collection, it remains **Qdrant-only**.

**Status:** `done`

---

## Phase 4 — Qdrant service card UI

**Goal.** Replace generic pills and RAG mini-cards with operator KV fields, counters, and a single subtitle line driven by the latest classified event.

**Deliverables**

- [`internal/server/embedui/logs.js`](../../internal/server/embedui/logs.js) — `qdrantServicePanelMiniHtml`, `buildServiceCard` integration with `qdrantCardModel`, suppress **qdrant** source pill in the expanded Qdrant panel only, omit **`timelineBlock`** when **`name === "qdrant"`**.
- Operator subtitles from **`qdrantOperatorLine`** for collapsed header and primary line display.

**Acceptance**

- **Collapsed header:** one operator subtitle (latest high-signal **`qdrant.*`** line in the current-process window); no retrieve · search · lines pills.
- **KV row** (keys always visible; values fill as events arrive):

| Key | Source `msg` / logic |
|-----|---------------------|
| **version** | `qdrant.version` |
| **configuration** | `qdrant.config.optional_missing` → **`supervised`** |
| **mode** | `qdrant.cluster.single_node` → **`single-node`** |
| **TLS (REST)** | `qdrant.listen.tls_disabled_rest` → **`disabled`**; `qdrant.listen.tls_enabled_rest` → **`enabled`** |
| **TLS (gRPC)** | `qdrant.listen.tls_disabled_grpc` → **`disabled`**; `qdrant.listen.tls_enabled_grpc` → **`enabled`** |
| **TLS (internal)** | `qdrant.cluster.internal_tls_disabled` → **`disabled`**; `qdrant.cluster.internal_tls_enabled` → **`enabled`** (`qdrant_internal_tls`) |
| **telemetry** | `qdrant.telemetry.disabled` / `qdrant.telemetry.enabled` |
| **recovery** | `qdrant.storage.recovery_mode` → **`active`** |
| **port (REST/gRPC)** | `qdrant.listen.http` + `qdrant.listen.grpc` → **`{rest}/{grpc}`** |

- **Counters:** collections loaded or total; upsert success or fail; delete success or fail; search success or fail (per taxonomy rules).
- **Full log:** rows in this panel do not repeat the **qdrant** source pill.

**Status:** `done`

---

## Phase 5 — Indexer card UI

**Goal.** Indexer workspace cards show **Collection status** and include routed Qdrant lines with the **qdrant** badge.

**Deliverables**

- Indexer card subtitle prefers the latest matching **`qdrantOperatorLine`** for that workspace when the last Qdrant-tagged line applies.
- **Collection status** KV from **`qdrantIndexerCollectionStatusLabel`** for loading, loaded, reading, upserting, deleting, searching.

**Acceptance**

| `msg` | Indexer card subtitle (scoped) | **Collection status** |
|-------|-------------------------------|------------------------|
| `qdrant.collection.loading` | Loading collection {name} | **Loading** |
| `qdrant.shard.recover_progress` | Loading collection {name} (+ progress) | **Loading** (optional truncated progress) |
| `qdrant.shard.recovered` | Loaded collection {name} | **Loaded** |
| `qdrant.http.collection_meta` | Reading collection {name} (+ status) | **Reading** |
| `qdrant.http.points_upsert_ok` / `qdrant.http.points_upsert_rejected` | Upsert into collection {name} (+ status) | **Upserting** |
| `qdrant.http.points_delete` | Deleting from collection {name} (+ status) | **Deleting** |
| `qdrant.http.vector_search` | Searching collection {name} (+ status) | **Searching** |

**Status:** `done`

---

## References

### Code

- Go: [`internal/servicelogs/qdrantline/`](../../internal/servicelogs/qdrantline/) (`NormalizePayload`, `NewWriter`).
- JS: [`internal/server/embedui/logs/derive/sha1.js`](../../internal/server/embedui/logs/derive/sha1.js), [`qdrantCollection.js`](../../internal/server/embedui/logs/derive/qdrantCollection.js).
- UI: [`internal/server/embedui/logs.js`](../../internal/server/embedui/logs.js).

### Follow-up (optional)

- Fixture-backed golden tests from `temp/qdrant-logs-1.log` under `testdata/` for regression parsing.
- If HTTP access volume becomes an operator problem, consider rollup or caps in the UI without changing **`msg`** classification.
