# Plan: Indexer manifest ingest with line metadata

| Field                          | Value                                                                                                                                                                     |
|--------------------------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| **Doc kind**                   | `feature-plan`                                                                                                                                                            |
| **Owners / areas**             | Gateway, `chimera-indexer`, operator SQLite, embed UI (`/ui/chat`), Qdrant payload                                                                                        |
| **Status**                     | `draft`                                                                                                                                                                   |
| **Targets**                    | Gateway v0.4 ([`version-v0.4.md`](../version-v0.4.md))                                                                                                                    |
| **Last updated**               | See git history                                                                                                                                                           |
| **Supersedes / superseded by** | Replaces gateway-only chunking at ingest (v0.2 baseline). Complements [`indexer-sync-state-sqlite-and-force-reindex.md`](indexer-sync-state-sqlite-and-force-reindex.md). |
| **As-built**                   | None — link to [`docs/features/`](../features/README.md) when shipped                                                                                                     |

## At a glance

Operators see **workspace snippets in chat with file paths and line ranges** (e.g. `src/foo.go · L42–58`), a **gutter-style line-number column** beside snippet text, and **mid-line chunk starts** marked with `…` before the visible code. The indexer **pre-chunks every file**, builds a **manifest** with line and byte spans, and sends it to the gateway; the gateway **embeds and stores** rich metadata in Qdrant and a **segment index** in operator SQLite. Retrieval, injected system context, conversation history, **revision coherence**, and **tooling/expansion** APIs all use the same manifest contract. **No legacy ingest path** — manifest is required. A one-time **full re-index of all workspaces** ships the feature.

| Phase                                                                                                      | Outcome                                                                                   | Status |
|------------------------------------------------------------------------------------------------------------|-------------------------------------------------------------------------------------------|--------|
| [Phase 1 — Shared chunk library](#phase-1--shared-chunk-library)                                           | `chimera/internal/chunk` splits text and annotates line/byte spans; golden tests          | `todo` |
| [Phase 2 — Indexer manifest build and ingest](#phase-2--indexer-manifest-build-and-ingest)                 | Indexer builds manifest per file and uploads via manifest-first ingest API                | `todo` |
| [Phase 3 — Gateway manifest ingest and segment index](#phase-3--gateway-manifest-ingest-and-segment-index) | Gateway accepts manifest, embeds, upserts Qdrant + operator SQLite segment rows           | `todo` |
| [Phase 4 — Retrieval, history, and injected context](#phase-4--retrieval-history-and-injected-context)     | Virtual-model RAG returns metadata; history persists lines; system block includes ranges  | `todo` |
| [Phase 5 — Chat snippet gutter UI](#phase-5--chat-snippet-gutter-ui)                                       | `/ui/chat` renders L42–58 header and left gutter with mid-line `…`                        | `todo` |
| [Phase 6 — Revision coherence](#phase-6--revision-coherence)                                               | Staleness detection vs live files; operator-visible warn/strict behavior                  | `todo` |
| [Phase 7 — Tooling and expansion](#phase-7--tooling-and-expansion)                                         | Segment read / context-around tools; indexer or gateway file-serving (see open questions) | `todo` |
| [Phase 8 — Re-index and ship](#phase-8--re-index-and-ship)                                                 | Full workspace re-index; sync-state integration; docs and feature record                  | `todo` |

---

## Background

Today the indexer uploads **whole files**; the gateway runs `chunk.Split` and stores only `source`, `text`, and file-level `content_sha256` in Qdrant. Operators see file paths in chat snippets but **no line numbers**, and tooling cannot navigate by line or byte offset. Chunk character offsets exist transiently in `rag/chunk` but are discarded before upsert.

This plan moves chunking and span annotation to the **indexer**, makes **manifest ingest the only path**, and stores **per-chunk metadata** in Qdrant and operator SQLite for UI, coherence, and tools. It targets **gateway v0.4** and assumes **two operators only** — no legacy ingest or conversation migration.

**Related docs:** [`features/indexer-ingest-pipeline.md`](../features/indexer-ingest-pipeline.md), [`features/indexer-workspaces.md`](../features/indexer-workspaces.md), [`features/operator-conversation-history.md`](../features/operator-conversation-history.md), [`plans/indexer-sync-state-sqlite-and-force-reindex.md`](indexer-sync-state-sqlite-and-force-reindex.md), [`version-v0.2.md`](../version-v0.2.md) (prior payload minimum).

**Non-goals (v1 of this plan)**

- AST / symbol names in chunk metadata.
- Git commit or blame in headers.
- Legacy gateway-only chunking or pre-manifest conversation rows.
- Changing retrieval `top_k` / score threshold behavior.

---

## Contracts (normative)

### Manifest object (indexer → gateway)

Primary transport: **`POST /v1/ingest`** with `Content-Type: application/json` body:

```json
{
  "object": "ingest.manifest",
  "source": "src/main.go",
  "content_sha256": "sha256:…",
  "client_content_hash": "sha256:…",
  "chunk_size": 512,
  "chunk_overlap": 128,
  "chunk_schema": 2,
  "line_count": 842,
  "file_bytes": 28401,
  "chunks": [
    {
      "chunk_index": 0,
      "text": "…",
      "start_line": 42,
      "end_line": 58,
      "start_byte": 1204,
      "end_byte": 3891,
      "start_ch": 1180,
      "end_ch": 1520,
      "starts_mid_line": true,
      "language": "go"
    }
  ]
}
```

**Rules**

- **`source`** — root-relative path (existing invariant).
- **`content_sha256`** — SHA-256 over **normalized UTF-8** file bytes (see newline rule below).
- **`chunks`** — ordered by `chunk_index`; each `text` is the exact bytes embedded and stored in Qdrant.
- **Lines** — **1-based**, human-oriented. **`start_line`** is the line where the chunk **starts** (even when mid-line). **`end_line`** is the line containing the **last character** of the chunk.
- **`starts_mid_line`** — `true` when the chunk begins after column 1 on `start_line`; UI and injected context prefix that line's visible text with `…`.
- **Empty / whitespace-only files** — omit ingest call entirely (same as today). Do **not** upload a zero-chunk manifest unless a future tool requires it.
- **Binary / ignore** — unchanged indexer ignore and NUL sniff rules.

**Newline normalization (platform-independent)**

Before chunking and hashing, normalize line endings to `\n` (`\r\n` and lone `\r` → `\n`). Line and byte spans are computed on the **normalized** text. Document in `docs/indexer.md`.

**Compatibility ingest (external tools)**

Optional second shape for tools that upload a whole file plus manifest:

- Multipart: `source`, `content_hash`, `file`, `manifest` (JSON string or part).
- Gateway **trusts** the manifest (no re-chunk verification). Mismatch between file hash and manifest is rejected.

**Chunked session API (large manifests)**

When manifest JSON exceeds `max_whole_file_bytes`, use the existing session flow:

1. `POST /v1/ingest/session` — `source`, `content_hash` (same as today).
2. `PUT …/chunk` — transport byte slices (unchanged).
3. `POST …/complete` — body includes the **full manifest JSON** (one manifest per session).

Gateway reassembles transport bytes only if the compatibility whole-file path is used; **manifest-first** sessions send manifest on complete without requiring reassembly for embedding (indexer already chunked locally).

### Qdrant payload extension

Add to `vectorstore.Payload` (all required for new points):

| Field             | Type   | Notes                                |
|-------------------|--------|--------------------------------------|
| `chunk_index`     | int    | 0-based within file                  |
| `chunk_count`     | int    | Total chunks at ingest               |
| `start_line`      | int    | 1-based display start                |
| `end_line`        | int    | 1-based display end                  |
| `start_byte`      | int    | UTF-8 byte offset in normalized file |
| `end_byte`        | int    | Exclusive                            |
| `start_ch`        | int    | Rune offset inclusive                |
| `end_ch`          | int    | Rune offset exclusive                |
| `starts_mid_line` | bool   | Mid-line start marker                |
| `line_count`      | int    | File line count at ingest            |
| `file_bytes`      | int    | Normalized file size                 |
| `chunk_schema`    | int    | `2` for this plan                    |
| `language`        | string | From extension when known            |

Existing fields (`tenant_id`, `project_id`, `flavor_id`, `text`, `source`, `content_sha256`, `client_content_hash`, `created_at`) unchanged.

### Operator SQLite — `corpus_segments`

New table in operator migrations (gateway-owned; indexer never opens this file):

```sql
CREATE TABLE corpus_segments (
  segment_id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  project_id TEXT NOT NULL,
  flavor_id TEXT NOT NULL,
  source TEXT NOT NULL,
  content_sha256 TEXT NOT NULL,
  chunk_index INTEGER NOT NULL,
  chunk_count INTEGER NOT NULL,
  start_line INTEGER NOT NULL,
  end_line INTEGER NOT NULL,
  start_byte INTEGER NOT NULL,
  end_byte INTEGER NOT NULL,
  start_ch INTEGER NOT NULL,
  end_ch INTEGER NOT NULL,
  starts_mid_line INTEGER NOT NULL DEFAULT 0,
  vector_point_id TEXT NOT NULL,
  language TEXT,
  created_at TEXT NOT NULL,
  UNIQUE (tenant_id, project_id, flavor_id, source, content_sha256, chunk_index)
);
CREATE INDEX idx_corpus_segments_lookup
  ON corpus_segments (tenant_id, project_id, flavor_id, source, content_sha256);
CREATE INDEX idx_corpus_segments_line
  ON corpus_segments (tenant_id, project_id, flavor_id, source, start_line);
```

**Delete semantics**

- **Re-ingest one file** — `DeleteBySource` in Qdrant (existing) + `DELETE FROM corpus_segments WHERE … AND source = ?` (all hashes for that source, or only stale hash after upsert — implement as single transaction: delete old segments for `source`, insert new).
- **Workspace purge** — delete all segment rows for `(tenant_id, project_id, flavor_id)` and Qdrant collection points (align with [`version-v0.4.md`](../version-v0.4.md) workspace lifecycle when that API ships).
- Prefer **fewest round-trips**: one SQL delete by scope + existing Qdrant batch delete.

**Overlap with sync-state plan**

| Store                               | Owner   | Purpose                                                                                                                                                                                                                     |
|-------------------------------------|---------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `sync-state.sqlite` (indexer)       | Indexer | Per-file **skip/re-ingest** checkpoints (`client_sha256`, `server_sha256`, optional `chunk_count`, `chunk_schema`) — see [`indexer-sync-state-sqlite-and-force-reindex.md`](indexer-sync-state-sqlite-and-force-reindex.md) |
| `corpus_segments` (operator.sqlite) | Gateway | **Tooling / line lookup** and neighbor queries by `(source, content_sha256, chunk_index)`                                                                                                                                   |
| Qdrant                              | Gateway | Vector search + payload copy for retrieval                                                                                                                                                                                  |

Implement **Phase 1 of the sync-state SQLite plan** before or in parallel with Phase 8 so re-index and skip logic scale; bump `chunk_schema` in sync entry when manifest format changes to force re-upload.

### Retrieval and UI surfaces

**`X-Chimera-RAG-Hits` header** — extend each hit:

```json
{
  "source": "src/main.go",
  "text": "…",
  "score": 0.91,
  "language": "go",
  "start_line": 42,
  "end_line": 58,
  "starts_mid_line": true,
  "chunk_index": 3,
  "content_sha256": "sha256:…"
}
```

**Injected system block** (`FormatRetrievedContext`):

```markdown
### Retrieved context

1. `src/main.go` L42–58 (score=0.910)

…visible chunk text when starts_mid_line…

```

**Conversation history** — extend `conversation_retrievals` with `start_line`, `end_line`, `starts_mid_line` (migration `00000N_manifest_retrievals.sql`). No backfill for old rows (greenfield).

### Config

- Chunk knobs remain **`GET /v1/indexer/config`** → `chunk_size`, `chunk_overlap` only (from gateway `rag` config).
- Indexer and gateway both import **`chimera/internal/chunk`** — single source of truth.

### Logging discipline

- **Manifest reject** — one **WARN** per file with `source`, `reason` code, no chunk text; aggregate repeated failures in ingest summary if needed.
- **Manifest built** — **DEBUG** slug `indexer.job.manifest_built` with `rel`, `chunks`, `line_count`, `file_bytes`, scope fields (for large-file troubleshooting).
- Do **not** log full manifest bodies at INFO.

---

## Phase 1 — Shared chunk library

**Goal.** One rune-based chunker with line and byte annotation, shared by indexer and gateway tests.

**Deliverables**

- New package `chimera/internal/chunk` (move/adapt from `chimera-gateway/internal/rag/chunk`):
  - `Split(normalized string, size, overlap int) []Segment`
  - `Segment`: `Index`, `Text`, `StartCh`, `EndCh`, `StartLine`, `EndLine`, `StartByte`, `EndByte`, `StartsMidLine`, `Language` (optional helper from path)
  - `NormalizeNewlines([]byte or string) string` for platform-independent ingest
- Golden-file tests: CRLF file, Unicode runes, mid-line chunk start, overlap producing duplicate line ranges across chunks.
- Gateway re-exports or aliases old import path during migration (single PR or follow-up).

**Acceptance**

- `go test ./chimera/internal/chunk/...` passes with fixtures covering L42–58-style spans.
- Same input + knobs produces identical segments on Linux and Windows-normalized inputs.

**Status:** `todo`

---

## Phase 2 — Indexer manifest build and ingest

**Goal.** Every ingest job builds a manifest locally and uploads it manifest-first; unchanged files still skip before manifest build.

**Deliverables**

- `internal/indexer/manifest.go` — build manifest from file bytes using `chimera/internal/chunk` + gateway config from `lastGW` (`ChunkSize`, `ChunkOverlap`).
- Extend `GatewayClient`:
  - `IngestManifest(ctx, ManifestRequest)` → JSON `POST /v1/ingest`
  - Session complete accepts manifest JSON body (one manifest per session).
  - Optional multipart **compatibility** helper for external tools (file + manifest).
- Replace whole-file-only `Ingest` / `IngestChunked` call sites in `ingest.go` with manifest path.
- DEBUG log `indexer.job.manifest_built` after build, before upload.
- Tests: mock gateway records manifest shape; mid-line fixture; skip path does not call manifest build.

**Acceptance**

- Indexer e2e / client tests show manifest JSON with all contract fields.
- Large file uses session complete with single manifest payload.
- Skip logic unchanged for corpus/sync/empty.

**Status:** `todo`

---

## Phase 3 — Gateway manifest ingest and segment index

**Goal.** Gateway requires manifest ingest, embeds chunk texts, writes Qdrant + `corpus_segments`; rejects invalid manifests without log spam.

**Deliverables**

- `rag.Service.IngestManifest` — validate schema, trust indexer spans, embed batch, upsert points, replace segment rows atomically per `source`.
- HTTP handlers: JSON manifest on `POST /v1/ingest`; session `complete` manifest; **400** on missing/invalid manifest (remove gateway-only split path).
- Operator migration for `corpus_segments` table + repository in `operatorstore`.
- Update `GET /v1/indexer/config` docs payload minimum field list.
- Tests: ingest_test with manifest fixture; segment row count matches chunk count; re-ingest replaces segments.

**Acceptance**

- Ingest without manifest returns 400 with stable error type.
- Qdrant point payload includes line/byte fields; scroll/search returns them.
- Re-ingest same source deletes prior segments for that source efficiently.

**Status:** `todo`

---

## Phase 4 — Retrieval, history, and injected context

**Goal.** Virtual-model chat retrieval surfaces full metadata end-to-end.

**Deliverables**

- `rag.SummarizeHits` / `WriteResponseHeaders` — include line fields.
- `FormatRetrievedContext` — `L{start}–{end}` in system block; mid-line prefix `…` on first line of chunk text when `starts_mid_line`.
- `conversationhistory.recorder` + migration — persist `start_line`, `end_line`, `starts_mid_line` on `conversation_retrievals`.
- `operatorapi.ConversationRAGHit` + handlers updated.
- Tests: `prompt_test`, `response_meta_test`, `conversations_test`, chat header parse test.

**Acceptance**

- Chat completion response headers carry line range JSON.
- Saved conversation reload includes line fields on `ragHits`.
- System message visible in proxy logs / witness at DEBUG only (no full text at INFO).

**Status:** `todo`

---

## Phase 5 — Chat snippet gutter UI

**Goal.** Operators see `path · L42–58` and a line-number gutter beside snippet code; mid-line starts show `…`.

**Deliverables**

- `chat/render/messages.js` + `Snippet` renderer:
  - Summary line: `{source} · L{start}–{end}` (collapse to `L{n}` when start === end).
  - Gutter column with line numbers aligned to rendered lines (reuse or extend markdown/code path).
  - When `starts_mid_line`, first content line prefixed with `…` in the gutter row for `start_line`.
- `chat/state.js` — copy/markdown export includes line ranges.
- CSS in `chat.css` for gutter layout.
- Goja tests: `chat_markdown_test.go` / component tests for L42–58 and mid-line fixture.

**Acceptance**

- Manual: RAG hit on a known file shows gutter numbers matching manifest.
- Overlapping chunks from same file may appear twice with overlapping ranges — acceptable.

**Status:** `todo`

---

## Phase 6 — Revision coherence

**Goal.** Operators know when indexed snippets may be stale relative to files on disk; tools can enforce strict hash match.

**Deliverables**

- **Detection** — On indexer workspace poll or pre-upload stat, compare local file hash to last known `content_sha256` in sync state; expose optional `GET /v1/indexer/corpus/stale` or per-source hint in manifest response echo.
- **UI** — Chat snippet badge when `content_sha256` on hit differs from indexer-reported current hash for that `source` (WARN copy, not ERROR).
- **Modes** (resolution in [Open questions](#open-questions) if not decided in implementation):
  - `off` — no badge.
  - `warn` — badge on snippet + history.
  - `strict` — expansion tools refuse when stale.
- Log slug `indexer.coherence.stale` at DEBUG with `source`, indexed hash, live hash — rate-limited per source.

**Acceptance**

- Edit file on disk, do not re-ingest: next chat hit shows stale warn when mode ≥ warn.
- After re-ingest, badge clears.

**Status:** `todo`

---

## Phase 7 — Tooling and expansion

**Goal.** Gateway (and optionally model tool router) can fetch line-aligned context beyond the retrieved chunk.

**Deliverables**

- **Gateway REST** (authenticated like ingest):
  - `GET /v1/rag/segments?source=&content_sha256=` — list segment rows for a file version.
  - `GET /v1/rag/context?source=&line=&before=&after=` — merge segment texts covering line window (from `corpus_segments` + Qdrant or indexer read — see open questions).
  - `GET /v1/rag/adjacent?point_id=&radius=` — neighbor chunks by `chunk_index`.
- **Indexer REST** (candidate — see open questions):
  - `POST /v1/indexer/read-segment` — `{ source, start_byte, end_byte, content_sha256? }` → bytes + line map slice from watched file.
- **Tool definitions** (gateway tool-router or Continue MCP doc):
  - `workspace_context_around` — `{ source, line, before_lines, after_lines }`
  - `workspace_adjacent_chunks` — `{ source, chunk_index, radius, content_sha256? }`
  - `workspace_read_lines` — `{ source, start_line, end_line }`
- LRU cache in gateway for hot `(source, content_sha256, byte_range)` slices (size/TTL in config).
- Tests for segment lookup and line window assembly with overlap.

**Acceptance**

- Documented tool contract in plan appendix or `docs/indexer.md`.
- Integration test: retrieve hit → adjacent API returns overlapping neighbor chunks.
- Stale source rejected in strict coherence mode.

**Status:** `todo`

---

## Phase 8 — Re-index and ship

**Goal.** All workspaces indexed with manifest schema; operator docs and feature record updated.

**Deliverables**

- Coordinate [`indexer-sync-state-sqlite-and-force-reindex.md`](indexer-sync-state-sqlite-and-force-reindex.md) Phase 1–4 minimum: SQLite sync state + **Re-index workspace** button bumps `reindex_generation` → full re-upload.
- Ship migration note: operators run **Re-index all workspaces** once (or clear sync state + restart indexer).
- Update [`docs/indexer.md`](../indexer.md), [`docs/version-v0.2.md`](../version-v0.2.md) payload section, [`features/indexer-ingest-pipeline.md`](../features/indexer-ingest-pipeline.md) or new [`features/indexer-manifest-ingest.md`](../features/indexer-manifest-ingest.md).
- Mark plan **shipped**; add **As-built** link in this table.

**Acceptance**

- After re-index, every Qdrant point has `chunk_schema: 2` and non-zero `start_line`.
- Storage stats show expected point counts; chat turn shows gutter lines on real repo file.

**Status:** `todo`

---

## Open questions

1. **File bytes for expansion tools** — Should `GET /v1/rag/context` read only from **Qdrant chunk overlap**, **indexer `read-segment`**, or **gateway LRU cache** with indexer fallback? Recommendation: indexer read for line-accurate windows; Qdrant neighbors for quick adjacent chunks; cache in gateway with `content_sha256` key.

2. **Coherence and tooling toggles** — Per-workspace columns on `workspaces` (`coherence_mode`, `tooling_enabled`) vs global `gateway.yaml` `rag.coherence` / `rag.tooling`? Recommendation: global defaults in YAML + optional per-workspace override in `workspaces.metadata_json` when UI needs it.

3. **Sync-state fields** — Store `chunk_schema` and `chunk_count` in indexer sync SQLite row to detect manifest upgrades without full hash change? Recommendation: yes; bump forces re-ingest when gateway increments schema.

4. **External tool compatibility** — Which third-party ingest clients must be tested (Continue, custom scripts)? List before Phase 3 ships.

5. **Session transport without whole file** — Confirm v1: indexer always chunks locally; session API carries **manifest JSON on complete** only (transport PUT optional future). No byte reassembly for embedding.

---

## References

- Chunk (today): `chimera/chimera-gateway/internal/rag/chunk/chunk.go`
- Ingest (today): `chimera/chimera-indexer/internal/indexer/ingest.go`, `chimera/chimera-gateway/internal/rag/service.go`
- Payload: `chimera/chimera-gateway/internal/vectorstore/vectorstore.go`
- Chat RAG UI: `embed/embedui/chat/render/messages.js`, `chat/gatewayClient.js`
- History: `operatorstore/conversations.go`, migration `000003_conversation_history.sql`
- Version roadmap: [`version-v0.4.md`](../version-v0.4.md)
