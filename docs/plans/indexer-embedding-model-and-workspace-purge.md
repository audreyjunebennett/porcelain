# Plan: Indexer embedding model and workspace purge

| Field | Value |
|-------|-------|
| **Doc kind** | `feature-plan` |
| **Owners / areas** | Gateway RAG, operator UI (`/ui/settings`), indexer service card, vector store, operator SQLite |
| **Status** | `draft` |
| **Targets** | Gateway v0.3 setup wizard step 5, gateway v0.5 indexer lifecycle |
| **Last updated** | See git history |
| **Supersedes / superseded by** | Extends [`indexer-workspaces-sqlite-gateway-api.md`](indexer-workspaces-sqlite-gateway-api.md); aligns with v0.4 purge theme — may ship earlier for delete-on-workspace-remove |
| **As-built** | None — link to [`docs/features/`](../features/README.md) when shipped |

## At a glance

Operators need to **choose the embedding model** used for ingest and retrieval from the **live broker catalog**, not only via hand-edited `gateway.yaml`. The **indexer / RAG service card** on `/ui/settings` should expose a catalog combobox, persist the choice through a gateway API, and show a clear **re-embed warning** when the model changes. When an operator **deletes a workspace**, the gateway should **drop the scoped vector collection** so orphaned vectors do not linger in Qdrant.

| Phase | Outcome | Status |
|-------|---------|--------|
| [Phase 1 — Embedding config API and runtime reload](#phase-1--embedding-config-api-and-runtime-reload) | Authenticated API writes `rag.embedding.model` (and `dim` when known); gateway reloads RAG embedder | `todo` |
| [Phase 2 — Indexer card embedding selector](#phase-2--indexer-card-embedding-selector) | Service card or workspace section: catalog combobox, warning copy, health reflects new model | `todo` |
| [Phase 3 — Workspace delete collection purge](#phase-3--workspace-delete-collection-purge) | Deleting a workspace drops its Qdrant collection (tenant + project + flavor scope) | `todo` |
| [Phase 4 — Re-embed operator guidance](#phase-4--re-embed-operator-guidance) | After embedding change, UI explains full re-index requirement; optional force re-index link when that plan ships | `todo` |

---

## Background

**Problem.** Today `rag.embedding.model` defaults to `ollama/nomic-embed-text:latest` in `gateway.yaml`. The gateway exposes the current value read-only via `GET /v1/indexer/config` and storage health flags `embed_model_not_in_catalog`, but operators cannot change it from `/ui/settings`. The v0.3 setup wizard step 5 assumes an embedding combobox that has no write path.

**Workspace delete.** Deleting a workspace row stops the indexer watch after reload but **does not purge** vectors ([`indexer-workspaces.md`](../features/indexer-workspaces.md) known gap). Operators expect “delete workspace” to clear searchable corpus for that scope.

**Related docs:** [`version-v0.3.md`](../version-v0.3.md) (setup wizard step 5), [`version-v0.4.md`](../version-v0.4.md) (indexer workspace lifecycle and purge), [`gateway-rag-ingest-and-retrieval.md`](../features/gateway-rag-ingest-and-retrieval.md), [`indexer-workspaces.md`](../features/indexer-workspaces.md), [`configuration.md`](../configuration.md).

---

## Phase 1 — Embedding config API and runtime reload

**Goal.** Operators (and the setup wizard) can set the embedding model through the same authenticated surface as other UI mutations.

**Deliverables**

- `GET /api/ui/rag/embedding` — current model id, dimension, catalog presence (`ok` / `embed_model_not_in_catalog`), and catalog-derived **candidate list** (all model ids from live chimera-broker snapshot, or full merged catalog per product decision).
- `PUT /api/ui/rag/embedding` — body `{ "model": "provider/model-id" }`; validate non-empty id and presence in live catalog (or allow with warning if catalog poll stale — document behavior).
- Patch `gateway.yaml` `rag.embedding.model` (and `rag.embedding.dim` when catalog or static map supplies dimension); `Runtime.Sync()`; RAG embedder reload.
- Unit tests: invalid model rejected; valid model persists and appears on `GET /v1/indexer/config`.
- Extend `config` package with `PatchGatewayYAMLBytesWithEmbeddingModel` (mirror fallback-chain patch pattern).

**Acceptance**

- Session-authenticated PUT updates embedding model; indexer health transitions from `embed_model_not_in_catalog` to ok when a valid catalog id is chosen.
- No secret material in request/response bodies.

**Status:** `todo`

---

## Phase 2 — Indexer card embedding selector

**Goal.** The operator sees and changes embedding model from the **indexer / RAG service card** (or a dedicated subsection on workspace cards if product prefers one global embed setting).

**Deliverables**

- Combobox populated from `GET /api/ui/rag/embedding` candidates (any catalog model — operator responsible for choosing an embedding-capable id; optional client-side hint filter for ids containing `embed` / known prefixes, not a hard gate).
- **Warning callout** when selection differs from saved value or on save: changing embedding model **invalidates existing vectors** for all workspaces using the prior model; operator should **re-index** (full workspace re-embed). Copy must be explicit and require confirm on save.
- Save wires to `PUT /api/ui/rag/embedding`; card shows current model id and embed health pill (reuse indexer health derive patterns).
- Setup wizard step 5 reuses the same component/module.

**Acceptance**

- Operator changes model in UI without editing YAML; warning appears before persist; `/v1/indexer/config` reflects new id within one sync.
- Gallery fixture or embedui test covers warning visibility.

**Status:** `todo`

---

## Phase 3 — Workspace delete collection purge

**Goal.** `DELETE /api/ui/indexer/workspaces/{id}` also **drops the vector collection** for that workspace’s `(tenant_id, project_id, flavor_id)` scope.

**Deliverables**

- Before or after SQLite row delete: compute `vectorstore.CollectionName(coords)` and call store **delete collection** (or delete-all-points API) with error handling — fail delete if purge fails, or complete SQLite delete with surfaced purge error (pick one; document in feature record).
- Structured log slug e.g. `gateway.operator.workspace.purged` with collection name and point count if available.
- Integration test: ingest into scope → delete workspace → search/retrieve returns empty for that scope; other collections untouched.
- Update [`indexer-workspaces.md`](../features/indexer-workspaces.md) when shipped (remove “no automatic vector purge” gap).

**Acceptance**

- Delete workspace in UI removes DB row and Qdrant collection for that scope only.
- Operator runbook/doc cross-link from v0.4 purge section.

**Status:** `todo`

---

## Phase 4 — Re-embed operator guidance

**Goal.** After embedding model change, operators know **what to do next** without silent broken retrieval.

**Deliverables**

- Post-save banner on indexer card: “Embedding model changed — schedule re-index for affected workspaces.”
- Link to workspace cards or future force re-index control ([`indexer-sync-state-sqlite-and-force-reindex.md`](indexer-sync-state-sqlite-and-force-reindex.md) when available).
- Optional: block retrieval hits with log warning when collection dimension/model metadata mismatches configured embedder (only if cheap to detect — otherwise document manual purge + re-index).

**Acceptance**

- Changing embedding model never implies silent continued use of old vectors without operator-visible warning.

**Status:** `todo`

---

## Open questions

1. **Global vs per-workspace embedding model** — v0.3 wizard and this plan assume **one gateway-global** `rag.embedding.model`. Per-workspace embed models defer to v0.4+ scope unless explicitly needed for wizard MVP.
2. **Catalog candidate list** — All broker models vs heuristic “embedding-capable” subset for combobox UX.
3. **Purge failure on delete** — Transactional (rollback SQLite) vs best-effort purge with retry job.
4. **Dim auto-detection** — Required fields when switching to models with non-768 dimensions (Qdrant collection recreate implications).

---

## References

- Code: `internal/rag/`, `internal/rag/ragembed/`, `internal/server/indexerapi/health.go`, `internal/vectorstore/`, `embed/embedui/settings/render/cards/serviceCard.js`, workspace handlers `internal/server/adminui/api/indexer/`
- Config: [`config/gateway.example.yaml`](../../config/gateway.example.yaml) (`rag.embedding`)
- Plans: [`indexer-workspaces-accurate-reporting.md`](indexer-workspaces-accurate-reporting.md) Phase 4 corpus purge notes
