# Chat Indexer (Sprint 1: Ingester)

This folder contains the **Sprint 1 ingester**: it reads Audrey’s existing chat exports and writes them into a single searchable SQLite database.

## Output

- SQLite DB: `D:\Rebirth\chat_indexer\chats.db`
- Tables:
  - `chats(id, source, title, content, workspace, created_at)`
  - `metadata(key, value)`

## How to run

Run the ingester (this rebuilds the `chats` rows each time):

```powershell
python D:\Rebirth\chat_indexer\ingest.py --reset
```

Defaults (can be overridden with flags in `ingest.py`):

- Cursor exports (markdown): `D:\Previously Claudia Core\Documentation\Exported_Cursor_Chats\composer_*.md`
- Cursor snapshot (metadata): `D:\Previously Claudia Core\.data\cursor_chats_snapshot.json`
- Mobile JSON: `D:\Previously Claudia Core\.data\mobile_conversations.json`
- Markdown logs: `D:\Previously Claudia Core\.data\conversation_log\*.md`

## What gets stored

Every source is normalized into a single row shape:

- `source`:
  - `cursor` for Cursor Composer exports
  - `mobile` for `mobile_conversations.json`
  - `discord` for `conversation_log/*.md` (Discord + other markdown logs)
- `title`: best-effort title (from export, JSON, or filename)
- `content`: best-effort plain text for search
- `workspace`: defaults to `audrey`
- `created_at`: best-effort timestamp (ISO where possible)

Cursor rows are additionally **enriched** (when possible) with file paths from `cursor_chats_snapshot.json` so you can search by referenced filenames even when message bodies are missing.

## Verified results (run on 2026-05-06)

These numbers come from the latest `--reset` ingestion run on **May 6, 2026**.

1) **Did `export_cursor_chats.py` give FULL message content, or just metadata?**

- The Cursor export markdowns in `D:\Previously Claudia Core\Documentation\Exported_Cursor_Chats` do **not** include full per-message bodies (`## Messages` / `### User` / `### Assistant` sections were not present).
- They contain **message sequence metadata** (bubble IDs + roles), plus sometimes **inline/rich text** blobs:
  - Cursor chats with any inline/rich text: **176 / 196**
  - Cursor chats with only message-sequence (no inline/rich): **14 / 196**
  - Cursor chats with full message bodies: **0 / 196**

2) **How many total conversations end up in the DB?**

- Total chats indexed: **220**
  - Cursor: **196**
  - Mobile: **10**
  - Markdown logs (“discord”): **14**

3) **Are any conversations missing or corrupted?**

- Cursor: all **196** threads are present as rows, but **full message bodies are missing** from the export; search quality is limited to inline/rich blobs (when present) + snapshot file-path enrichment (when snapshot entries exist).
- Cursor snapshot coverage note: `cursor_chats_snapshot.json` contains **46** chat entries, so only those 46 Cursor rows can be enriched with file lists from the snapshot.
- Mobile: all **10** conversations were ingested; **3** have no messages in the JSON (they still exist as empty conversations).
- Markdown logs: all **14** `conversation_log/*.md` files were ingested; speaker-line parsing worked for all files (fallback is raw markdown if format changes).

4) **What’s the total size of `chats.db`?**

- `D:\Rebirth\chat_indexer\chats.db` size: **7,286,784 bytes** (~6.95 MiB)

