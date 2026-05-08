# Plan: Operator-facing Qdrant log classification

| Field | Value |
|-------|-------|
| **Doc kind** | `feature-plan` |
| **Owners / areas** | Gateway supervision (`internal/supervisor`), logs UI (`internal/server/embedui/logs`), parse/derive (`internal/server/embedui/logs/parse`, `derive`), desktop mirror (`internal/servicelogs`) |
| **Status** | `implemented` (core path in gateway + logs UI; legacy unparsed tail lines may lag until buffer refreshes) |
| **Targets** | Gateway + supervised Qdrant with **JSON logger only** (`QDRANT__LOGGER__FORMAT=json`) |
| **Last updated** | 2026-05-07 |

## At a glance

Qdrant subprocess output is becoming **JSON lines** only. Today the operator log view mostly shows raw Qdrant/Rust `target` strings and embedded HTTP access lines. This plan captures:

1. A **stable `msg` taxonomy** on **every** classified line (parallel to **`indexer.*`**).
2. **Operator-facing copy** (subtitle, KV summary fields, counters) derived from those slugs.
3. **UI contract** for the **Qdrant service card** and for **per-workspace indexer cards** when Qdrant activity maps to an indexer collection.

**Related docs:** [`supervisor.md`](../supervisor.md), [`log-presentation-layer.md`](log-presentation-layer.md), [`docs/indexer.md`](../indexer.md).

---

## Background

- Supervised Qdrant uses **`QDRANT__LOGGER__FORMAT=json`** so each tracing event is one JSON object per line (plus optional non-JSON banner/version lines on stdout depending on Qdrant release behavior).
- The **desktop mirror** format remains: `timestamp<TAB>source<TAB>payload` (`internal/servicelogs/store.go`).
- The **logs UI** parses payloads with `ClaudiaLogs.parseLogText` (`internal/server/embedui/logs/parse/parseLogText.js`); nested JSON fields flatten with dot keys; HTTP rollup in the Qdrant card today expects flattened `method` / `path` / `statusCode` for `http.access`-style rows unless we add Qdrant-specific normalization.
- **Implementation requirement:** after normalization, **every** Qdrant-derived row exposed to the UI should carry a **`msg`** field (same pattern as indexer `slog` lines: `"msg", "qdrant.*"`).

---

## Locked decisions (2026-05-07)

| Topic | Decision |
|-------|-----------|
| Collection ↔ indexer card | Derive Qdrant collection name from **tenant / project / flavor** using the same rules as `internal/vectorstore/vectorstore.go` `CollectionName` (browser: `derive/qdrantCollection.js`). |
| Fan-out | Matching Qdrant lines appear on **every** indexer card whose coords resolve to that collection name. |
| Normalization location | **On ingest:** `internal/servicelogs/qdrantline` wraps the qdrant line writer (`cmd/claudia/serve.go`) so in-memory buffer + desktop mirror receive enriched JSON. |
| HTTP success | Summaries show **real status codes**; only **200** counts as success for upsert/delete/search counters; non-200 upserts emit **`qdrant.http.points_upsert_rejected`** (fail counter). |
| Counter window | Aggregates use lines **at or after the last `qdrant.version`** in the buffer (detect Qdrant restart while Claudia stays up). Claudia restart clears the ring buffer. |
| Timeline | **Only** the expanded **Qdrant** service panel omits the request timeline + bar. |

### Code references

- Go: [`internal/servicelogs/qdrantline/`](../../internal/servicelogs/qdrantline/) (`NormalizePayload`, `NewWriter`).
- JS: [`internal/server/embedui/logs/derive/sha1.js`](../../internal/server/embedui/logs/derive/sha1.js) (MIT: emn178/js-sha1), [`qdrantCollection.js`](../../internal/server/embedui/logs/derive/qdrantCollection.js).
- UI: [`internal/server/embedui/logs.js`](../../internal/server/embedui/logs.js) (Qdrant card, indexer fan-out, `suppressQdrantBadge`).

## Reference samples (local dev)

These paths are under repo-root **`temp/`** (gitignored).

| Artifact | Path | Purpose |
|----------|------|---------|
| Small excerpt (mixed banner + JSON) | [`temp/qdrant-logs-1.log`](../../temp/qdrant-logs-1.log) | Startup, collection recovery, HTTP access embedded in `actix_web::middleware::logger` JSON |
| Operator summary prepended | [`temp/qdrant-logs-1.operator-prefixed.log`](../../temp/qdrant-logs-1.operator-prefixed.log) | Format: `operator_summary<TAB>timestamp<TAB>qdrant<TAB>payload` |
| Large Qdrant-only extract | [`temp/claudia-desktop-qdrant-only.log`](../../temp/claudia-desktop-qdrant-only.log) | Volume / target spread |

---

## Canonical `msg` taxonomy

Stable machine slug pattern: **`qdrant.<segment>.<segment>…`** (aligned with **`indexer.*`**). Use **dots** between segments (e.g. `qdrant.listen.http`). Detection continues to use JSON `target`, `fields.message`, and embedded Apache-style HTTP fragments.

| `msg` | Typical detection | Notes |
|-------|-------------------|--------|
| `qdrant.startup.banner` | Non-JSON ASCII logo lines | Card subtitle: operator-facing **“Starting up …”** (tone). |
| `qdrant.version` | Plain line `Version: …` | Populate summary KV **version**. |
| `qdrant.web_ui_hint` | Plain line `Access web UI at …` | Logged for transparency; **no change** to Qdrant card fields (hint only). |
| `qdrant.config.optional_missing` | `qdrant::settings`, config file not found | Summary KV **configuration** = **`supervised`** (env-only supervised setup). |
| `qdrant.consensus.raft_load` | raft / consensus load | Subtitle: **“Loading collections …”**. |
| `qdrant.collection.loading` | `Loading collection:` | Subtitle: **“Loading collection {name}”**; increment **collection total** counter. |
| `qdrant.shard.recover_progress` | `Recovering shard` | Subtitle: **“Loading collection {name}”** + progress text from raw line. |
| `qdrant.shard.recovered` | `Recovered collection` | Subtitle: **“Loaded collection {name}”**; increment **collection loaded** counter. |
| `qdrant.cluster.single_node` | `Distributed mode disabled` | Summary KV **mode** = **`single-node`**. |
| `qdrant.listen.tls_disabled_rest` | REST TLS disabled | Summary KV **TLS** = **`disabled`**. |
| `qdrant.listen.http` | `HTTP listening` | Summary KV **port (REST/gRPC)**: set **REST** port component. |
| `qdrant.listen.grpc` | `gRPC listening` | Summary KV **port (REST/gRPC)**: set **gRPC** port component. |
| `qdrant.http.collection_meta` | `GET /collections/{slug}` (metadata) | Subtitle: **“Reading collection {name}”**; label **200** / **400** (or general success vs fail); update indexer **Collection status** → **Reading**. |
| `qdrant.http.points_upsert_ok` | `PUT …/points`, success path | Subtitle: **“Upsert into collection {name}”**; label **200** / **400**; **upsert** success/fail counters; indexer **Collection status** → **Upserting**. |
| `qdrant.http.points_delete` | `POST …/points/delete` | Subtitle: **“Deleting from collection {name}”**; label **200** / **400**; **delete** counters; indexer **Collection status** → **Deleting**. |
| `qdrant.http.vector_search` | `POST …/points/search` | Subtitle: **“Searching collection {name}”**; label **200** / **400**; **search** counters; indexer **Collection status** → **Searching**. |

Additional rows from the earlier spec (telemetry, inference, UI static, TLS gRPC, access_other) remain valid for completeness and can reuse the same **`msg`** + flattening pipeline; operator copy for those can stay secondary until needed.

**HTTP status labeling:** use **200** (or 2xx) as success and **400** (or 4xx/5xx) as fail for the compact label in subtitles unless we standardize on full status codes later.

---

## Indexer precedent (operator-focused structured logs)

The indexer emits **`slog`** with a stable **`"msg", "<dotted.slug>"`** and structured attributes. Qdrant should match that contract **after** parsing child JSON (either gateway-emitted shadow lines or enrich-only in `/api/ui/logs`).

---

## UI contract: Qdrant service card (summarized logs)

Applies to **Logs → Qdrant** summary card (`renderExpandedService` / `qdrantServicePanelMiniHtml` / `buildServiceCard` in `internal/server/embedui/logs.js`).

### Collapsed card header

- **Remove** metric pills: retrieve · search · lines (today `rollupGatewayRagPipeline` + `qdrantHttpPathRollup` + line count).
- Replace with a **single operator subtitle line** driven by the **latest** high-signal `qdrant.*` event (same pattern as other services’ “last message”), using the **subtitle strings** in the taxonomy table above.

### Expanded card — below the summary heading (KV row)

New **key-value** fields (always show keys; values fill as events arrive):

| Key | Source `msg` / logic |
|-----|---------------------|
| **version** | `qdrant.version` |
| **configuration** | `qdrant.config.optional_missing` → value **`supervised`** |
| **mode** | `qdrant.cluster.single_node` → **`single-node`** |
| **TLS** | `qdrant.listen.tls_disabled_rest` → **`disabled`** |
| **port (REST/gRPC)** | `qdrant.listen.http` + `qdrant.listen.grpc` → display **`{rest}/{grpc}`** (REST first, slash, gRPC). |

### Expanded card — summary section (replace current mini-cards)

**Remove:**

- Pills: retrieve; search; lines (header).
- **Request timeline** section and **timeline bar** for Qdrant (keep for other services as today; **skip** `timelineBlock` when `name === "qdrant"`).
- Mini-cards: **RAG retrieval (gateway)**, **Σ query embed time**, **Vector REST (Qdrant process)**.

**Add** four counter boxes:

| Box | Behavior |
|-----|----------|
| **Collections loaded / total** | From `qdrant.collection.loading` (increment **total**), `qdrant.shard.recovered` (increment **loaded**). Progress lines (`shard.recover_progress`) refine subtitle only unless we later split partial credit. |
| **Upsert success / fail** | From `qdrant.http.points_upsert_ok` (+ success/fail by HTTP status). |
| **Delete success / fail** | From `qdrant.http.points_delete`. |
| **Search success / fail** | From `qdrant.http.vector_search`. |

### Full event log (expanded)

- **Remove** the **qdrant** source pill on each row in this panel only (still show severity / shape badges if applicable). Implementation: when rendering the list for `name === "qdrant"`, suppress the badge that would show **qdrant** (e.g. pass a flag into `logSummaryHtml` / `buildDetailsColumn` analogous to `suppressIndexerBadge` for indexer).

---

## UI contract: Workspace indexer cards + collection mapping

**Goal:** Operations that name a **collection** should map to **indexer workspaces** defined for the system. Those log lines are both **Qdrant messages** (global Qdrant card) and **indexer messages** for the matching workspace indexer.

### Routing

- Parse collection **`{name}`** from HTTP paths (`/collections/{slug}/…`) or from shard/collection log text.
- Join to workspace/indexer cards using the same **collection naming** the gateway indexer uses per root (document exact join in code: config / `collection` field on indexer logs / workspace id). If a line cannot be mapped, it stays **Qdrant-only**.

### Per indexer card

- **Include** matching Qdrant events in that card’s **event list**. When shown there, each line keeps the **qdrant** pill (badge), treating the line as indexer-associated Qdrant activity.
- Add summary field **Collection status** (KV under card header or beside existing backlog UI — match existing indexer KV styling).

| `msg` | Indexer card subtitle | **Collection status** value |
|-------|----------------------|---------------------------|
| `qdrant.collection.loading` | Loading collection {name} | **Loading** |
| `qdrant.shard.recover_progress` | Loading collection {name} (+ progress) | (optional: keep **Loading** or show truncated progress) |
| `qdrant.shard.recovered` | Loaded collection {name} | **Loaded** |
| `qdrant.http.collection_meta` | Reading collection {name} (+ 200/400) | **Reading** |
| `qdrant.http.points_upsert_ok` | Upsert into collection {name} (+ 200/400) | **Upserting** |
| `qdrant.http.points_delete` | Deleting from collection {name} (+ 200/400) | **Deleting** |
| `qdrant.http.vector_search` | Searching collection {name} (+ 200/400) | **Searching** |

**Consistency:** Subtitle strings match the Qdrant card tone but are scoped to that indexer’s latest relevant event.

---

## Tooling

| Tool | Location | Role |
|------|----------|------|
| Operator-prefix prototype | [`temp/prefix_qdrant_operator_lines.py`](../../temp/prefix_qdrant_operator_lines.py) | Executable reference for classification |

```bash
python temp/prefix_qdrant_operator_lines.py temp/qdrant-logs-1.log temp/qdrant-logs-1.operator-prefixed.log
```

---

## Phased implementation

| Phase | Outcome |
|-------|---------|
| **P1 — Spec** | This doc + frozen `qdrant.*` list |
| **P2 — Parse & `msg`** | Go and/or JS: Qdrant JSON line → `target` / `fields.message` / HTTP fragment → **`msg`** + flattened attrs (`collection`, `http_status`, …) on every normalized row |
| **P3 — Dual routing** | Collection → indexer workspace resolution; fan Qdrant rows into indexer buckets when mapped |
| **P4 — Qdrant card UI** | KV fields, counters, remove pills/timeline/RAG mini-cards, full log without qdrant pill |
| **P5 — Indexer card UI** | **Collection status** + merged event list with qdrant badge |

---

## Open questions

- **Volume:** HTTP access lines dominate; consider rollup-only or caps for very long sessions.
- **Locale:** English-only operator strings (matches indexer).

---

## Checklist before marking done

- [x] Every Qdrant-normalized line carries **`msg`** and attributes needed for UI (via `qdrantline` on supervised ingest).
- [x] Qdrant card matches **KV**, **counters**, **subtitle**, **full log** (no qdrant pill in that panel) spec above.
- [x] Indexer cards show **Collection status** + routed Qdrant lines with **qdrant** pill.
- [ ] Fixture-backed tests from `temp/qdrant-logs-1.log` under `testdata/` (optional follow-up).
- [x] Supervisor doc notes normalization boundary ([`supervisor.md`](../supervisor.md)).
