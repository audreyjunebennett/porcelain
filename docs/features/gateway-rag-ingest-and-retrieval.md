# Feature: Gateway RAG ingest and retrieval

| Field | Value |
|-------|-------|
| **Doc kind** | `feature-record` |
| **Areas** | Gateway RAG service, vector store, embedding client, chat path, indexer ingest APIs |
| **Status** | `current` |
| **Introduced** | Gateway v0.2 RAG baseline |
| **Originated from** | [`plans/indexer.md`](../plans/indexer.md), gateway RAG design in [`docs/indexer.md`](../indexer.md) |
| **Related features** | [Workspace file indexer](indexer.md), [Indexer ingest pipeline](indexer-ingest-pipeline.md), [Operator chat UI](operator-chat-ui.md), [Context window admission](context-window-admission.md), [Gateway chat routing pipeline](gateway-chat-routing-pipeline.md) |
| **Depends on** | `rag.enabled` in gateway config, vector store, broker embedding catalog |
| **Last updated** | See git history |

## At a glance

When `rag.enabled` is true, the gateway runs a shared **RAG service** that chunks ingested text, embeds via the broker catalog model, upserts vectors into tenant/project/flavor **collections**, and retrieves top‑k chunks at chat time to inject a system message. The **indexer never embeds locally** — it sends file bytes to gateway ingest APIs. Chat clients and the operator UI scope retrieval with `X-Chimera-Project` / `X-Chimera-Flavor-Id` (or workspace-derived coords). Successful chat turns expose snippet metadata on `X-Chimera-RAG-Hits`.

## Operator-visible behavior

- **Settings** — Indexer service card exposes global embedding model selector; after save, an info banner links to the Workspaces section for re-index guidance.
- **Chat** — Optional workspace selector scopes retrieval; assistant messages show expandable snippets with path, score, and highlighted text.
- **Logs** — `rag.ingest.*`, `rag.query`, `conversation.rag.span`, and retrieval lifecycle slugs appear in the settings event log when RAG runs on a turn.

## System behavior and contracts

**Invariants**

- **Gateway owns chunking and embedding** — chunk size/overlap from gateway config; indexer sends whole-file or session-chunked **text** only.
- **Collection naming** — Derived from `(tenant_id, project_id, flavor_id)` via `vectorstore.CollectionName`.
- **Re-ingest** — Delete-by-source then upsert; server computes authoritative `content_sha256` over UTF-8 bytes.
- **Retrieve** — Embed query → vector search with score floor and top‑k; empty query returns no hits. When collection `vector_dim` ≠ configured `rag.embedding.dim` and the collection has points, retrieval returns **no hits** and logs `rag.retrieve.dim_mismatch` (stale vectors after an embedding model change until re-index).
- **Chat injection** — Non-empty hits formatted as “Retrieved context” system message prepended to proxied chat body (see `rag.FormatRetrievedContext`, `rag.InjectSystemMessage`).
- **RAG disabled** — Ingest and indexer config APIs return **503**; chat skips retrieval injection.
- **Scope** — Bearer token supplies `tenant_id`; project/flavor from headers or gateway defaults.

**Decisions**

| Topic | Decision |
|-------|----------|
| Whole vs session ingest | Below `max_whole_file_bytes` → `POST /v1/ingest`; above → session API with ordered chunks |
| Default chunking | 512 chars / 128 overlap when unset |
| Default top_k | 8 at retrieve unless overridden |
| Per-VM RAG | Not scoped per virtual model in v1 — gateway-global when enabled |

## Interfaces

| Surface | Detail |
|---------|--------|
| `POST /v1/ingest` | Whole-document ingest (Bearer auth) |
| `POST /v1/ingest/session`, chunk PUT, complete | Large-file transport |
| `GET /v1/indexer/config` | Chunk limits, embedding model, corpus paths (RAG gate) |
| `GET /v1/indexer/storage/health` | Vector + embedding readiness |
| `GET /v1/indexer/storage/stats` | Collection stats per scope |
| `GET /v1/indexer/corpus/inventory` | Skip detection for indexer |
| `POST /v1/chat/completions` | Retrieves when RAG enabled + scope present |
| Headers | `X-Chimera-Project`, `X-Chimera-Flavor-Id`; response `X-Chimera-RAG-Hits` (base64 JSON) |
| Config | `gateway.yaml` → `rag.enabled`, `rag.qdrant.*`, thresholds, size limits |

## Code map

| Concern | Location |
|---------|----------|
| RAG service | `chimera/chimera-gateway/internal/rag/service.go` |
| Chunking | `internal/rag/chunk/chunk.go` |
| Prompt injection | `internal/rag/prompt.go` |
| Response headers / hit summaries | `internal/rag/response_meta.go` |
| Embedding client | `internal/rag/ragembed/embed.go` |
| Vector store | `internal/vectorstore/` |
| Ingest HTTP handlers | `internal/server/` (ingest routes in `server.go`) |
| Indexer-facing RAG API | `internal/server/indexerapi/` |
| Chat retrieval wiring | `internal/server/virtualmodel_chat.go` |

## Verification

```bash
go test ./chimera/chimera-gateway/internal/rag/...
go test ./chimera/chimera-gateway/internal/server/ -run RAG
```

Manual: ingest a file via indexer or `POST /v1/ingest`, chat with matching project/flavor headers, confirm snippets in UI and `X-Chimera-RAG-Hits`.

## Revision coherence and expansion (manifest P6–7)

- **Stale sources** — Indexer compares on-disk normalized hash to sync-state `ServerSHA` on workspace poll; `PUT /v1/indexer/corpus/stale` replaces per-scope entries. Operators read `GET /v1/indexer/corpus/stale` or `/api/ui/indexer/corpus/stale`. Chat shows a **Stale** badge when a hit’s `content_sha256` matches an entry’s `indexed_sha256` and mode is not `off` (`rag.coherence.mode`: `off` \| `warn` \| `strict`, default `warn`).
- **Strict mode** — `GET /v1/rag/*` expansion returns 409 `corpus_stale` when the indexed digest is stale.
- **Expansion REST** (when `rag.tooling.enabled`, default true): `GET /v1/rag/segments`, `/context`, `/adjacent`, `/tools`. Segment text comes from Qdrant payloads keyed by `corpus_segments.vector_point_id`; context-around merges overlapping chunks with LRU cache (`rag.tooling.expansion_cache_*`).
- **Code** — `internal/corpusstale`, `internal/rag/expansion.go`, `internal/server/indexerapi/rag_expansion.go`, `internal/operatorstore/corpus_segments_query.go`.

## Out of scope and known gaps

- `X-Chimera-RAG-Hits` and `FormatRetrievedContext` include line ranges; chat UI gutter shipped ([`indexer-manifest-ingest`](../plans/indexer-manifest-ingest.md) Phases 4–5).
- Indexer `POST /v1/indexer/read-segment` (live file bytes) — deferred; expansion uses Qdrant + segment index only.
- Per-virtual-model RAG scope — deferred (see [virtual models](operator-virtual-models.md)).

## References

- Operator guide: [`docs/indexer.md`](../indexer.md)
- Indexer feature: [indexer.md](indexer.md), [indexer-ingest-pipeline.md](indexer-ingest-pipeline.md)
