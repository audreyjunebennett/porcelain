import argparse
import json
import os
import re
import sqlite3
from dataclasses import dataclass
from datetime import datetime, timezone
from pathlib import Path
from typing import Iterable, Optional


@dataclass(frozen=True)
class NormalizedChat:
    id: str
    source: str
    title: str
    content: str
    workspace: str
    created_at: str  # ISO-8601 string when possible


SCHEMA_SQL = """
CREATE TABLE IF NOT EXISTS chats (
  id TEXT PRIMARY KEY,
  source TEXT NOT NULL,
  title TEXT,
  content TEXT NOT NULL,
  workspace TEXT,
  created_at TEXT
);

CREATE INDEX IF NOT EXISTS idx_chats_source_created ON chats(source, created_at);

CREATE TABLE IF NOT EXISTS metadata (
  key TEXT PRIMARY KEY,
  value TEXT
);
"""


def _iso_or_empty(s: str) -> str:
    s = (s or "").strip()
    if not s:
        return ""
    # Already ISO-ish
    if re.match(r"^\d{4}-\d{2}-\d{2}T", s):
        return s
    if re.match(r"^\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}", s):
        # Cursor export uses "YYYY-MM-DD HH:MM"
        try:
            dt = datetime.strptime(s, "%Y-%m-%d %H:%M")
            return dt.isoformat()
        except Exception:
            return s
    return s


def connect_db(db_path: Path) -> sqlite3.Connection:
    db_path.parent.mkdir(parents=True, exist_ok=True)
    conn = sqlite3.connect(str(db_path))
    conn.execute("PRAGMA journal_mode=WAL;")
    conn.execute("PRAGMA synchronous=NORMAL;")
    conn.executescript(SCHEMA_SQL)
    return conn


def set_metadata(conn: sqlite3.Connection, key: str, value: str) -> None:
    conn.execute(
        "INSERT INTO metadata(key, value) VALUES(?, ?) "
        "ON CONFLICT(key) DO UPDATE SET value=excluded.value",
        (key, value),
    )


def upsert_chat(conn: sqlite3.Connection, chat: NormalizedChat) -> None:
    conn.execute(
        "INSERT INTO chats(id, source, title, content, workspace, created_at) "
        "VALUES(?, ?, ?, ?, ?, ?) "
        "ON CONFLICT(id) DO UPDATE SET "
        "source=excluded.source, title=excluded.title, content=excluded.content, "
        "workspace=excluded.workspace, created_at=excluded.created_at",
        (chat.id, chat.source, chat.title, chat.content, chat.workspace, chat.created_at),
    )


def load_cursor_snapshot(snapshot_path: Path) -> dict:
    try:
        return json.loads(snapshot_path.read_text(encoding="utf-8"))
    except FileNotFoundError:
        return {}


def index_cursor_snapshot_by_id(snapshot: dict) -> dict[str, dict]:
    out: dict[str, dict] = {}
    for c in (snapshot.get("chats") or []):
        if isinstance(c, dict) and c.get("id"):
            out[str(c["id"])] = c
    return out


CURSOR_MD_HEADER_RE = re.compile(r"^#\s+(.*)$")
CURSOR_MD_KV_RE = re.compile(r"^- \*\*(.+?)\*\*:\s*(.*)$")


def _extract_section(md_text: str, header: str) -> str:
    """
    Extract the body under a '## {header}' section until the next '## ' header.
    Returns empty string if section not present.
    """
    lines = md_text.splitlines()
    start = None
    for i, line in enumerate(lines):
        if line.strip().lower() == f"## {header}".lower():
            start = i + 1
            break
    if start is None:
        return ""
    out: list[str] = []
    for line in lines[start:]:
        if line.startswith("## "):
            break
        out.append(line)
    return "\n".join(out).strip()


def parse_cursor_export_md(md_text: str) -> tuple[str, str, str, str, dict]:
    """
    Returns: (title, composer_id, created_at, normalized_content, stats)
    """
    title = ""
    composer_id = ""
    created_at = ""
    in_messages = False
    current_role: Optional[str] = None
    buf: list[str] = []
    messages: list[tuple[str, str]] = []

    lines = md_text.splitlines()
    for line in lines:
        if not title:
            m = CURSOR_MD_HEADER_RE.match(line.strip())
            if m:
                title = m.group(1).strip()
                continue

        m = CURSOR_MD_KV_RE.match(line.strip())
        if m:
            k = m.group(1).strip().lower()
            v = m.group(2).strip()
            if k == "composer id":
                composer_id = v.strip("`").strip()
            elif k == "created":
                created_at = _iso_or_empty(v)
            continue

        if line.strip().lower() == "## messages":
            in_messages = True
            continue

        if in_messages:
            h = re.match(r"^###\s+(User|Assistant)\s*$", line.strip(), flags=re.I)
            if h:
                if current_role and buf:
                    messages.append((current_role, "\n".join(buf).strip()))
                current_role = h.group(1).lower()
                buf = []
                continue
            if current_role is not None:
                buf.append(line)

    if current_role and buf:
        messages.append((current_role, "\n".join(buf).strip()))

    inline_text = _extract_section(md_text, "Inline text")
    rich_text = _extract_section(md_text, "Rich text")
    message_sequence = _extract_section(md_text, "Message sequence")

    if messages:
        normalized = "\n\n".join(
            f"{role.capitalize()}:\n{content}".strip()
            for role, content in messages
            if (content or "").strip()
        ).strip()
        return title, composer_id, created_at, normalized, {
            "has_full_messages": True,
            "has_inline_text": bool(inline_text),
            "has_rich_text": bool(rich_text),
        }

    # Cursor exports often contain only metadata + message type sequence.
    parts: list[str] = []
    if inline_text:
        parts.append("Inline text:\n" + inline_text)
    if rich_text and rich_text != inline_text:
        parts.append("Rich text:\n" + rich_text)
    if message_sequence:
        parts.append("Message sequence:\n" + message_sequence)
    normalized = "\n\n".join(p.strip() for p in parts if p.strip()).strip()
    if not normalized:
        normalized = md_text.strip()

    return title, composer_id, created_at, normalized, {
        "has_full_messages": False,
        "has_inline_text": bool(inline_text),
        "has_rich_text": bool(rich_text),
    }


def iter_cursor_export_chats(
    export_dir: Path,
    snapshot_by_id: dict[str, dict],
    workspace: str,
) -> Iterable[tuple[NormalizedChat, dict]]:
    for md_path in sorted(export_dir.glob("composer_*.md")):
        md_text = md_path.read_text(encoding="utf-8", errors="replace")
        title, composer_id, created_at, content, parse_stats = parse_cursor_export_md(md_text)
        if not composer_id:
            composer_id = md_path.stem
        meta = snapshot_by_id.get(composer_id) or {}
        if not created_at:
            created_at = _iso_or_empty(str(meta.get("updated_at") or meta.get("created_at") or ""))
        chat_id = f"cursor:{composer_id}"
        # If the Cursor export is missing message bodies, enrich with snapshot metadata
        # so file paths remain searchable.
        if not parse_stats.get("has_full_messages", False):
            msg_count = meta.get("message_count")
            files = meta.get("files") if isinstance(meta.get("files"), list) else []
            enrich_parts: list[str] = []
            if msg_count is not None:
                enrich_parts.append(f"Message count (snapshot): {msg_count}")
            if files:
                enrich_parts.append("Files (snapshot):\n" + "\n".join(f"- {f}" for f in files[:200]))
                if len(files) > 200:
                    enrich_parts.append(f"... ({len(files) - 200} more file paths)")
            if enrich_parts:
                content = (content + "\n\n" + "\n\n".join(enrich_parts)).strip()
        yield (
            NormalizedChat(
                id=chat_id,
                source="cursor",
                title=title or (meta.get("title") or md_path.stem),
                content=content or "",
                workspace=workspace,
                created_at=created_at,
            ),
            {
                "path": str(md_path),
                "has_messages_section": "## Messages" in md_text or "## messages" in md_text,
                "has_inline_text": bool(parse_stats.get("has_inline_text")),
                "has_rich_text": bool(parse_stats.get("has_rich_text")),
                "has_full_messages": bool(parse_stats.get("has_full_messages")),
                "content_len": len(content or ""),
            },
        )


def flatten_mobile_messages(conv: dict) -> list[dict]:
    """
    mobile_conversations.json may store messages either:
    - conv["branches"] = [ [ {role, content, ...}, ... ] ]
    - conv["messages"] = [ ... ]
    """
    if isinstance(conv.get("branches"), list) and conv["branches"]:
        branch0 = conv["branches"][0]
        if isinstance(branch0, list):
            return [m for m in branch0 if isinstance(m, dict)]
    if isinstance(conv.get("messages"), list):
        return [m for m in conv["messages"] if isinstance(m, dict)]
    return []


def normalize_mobile_conversation(conv: dict, workspace: str) -> NormalizedChat:
    conv_id = str(conv.get("id") or "")
    title = str(conv.get("title") or "Mobile chat").strip()
    created_at = _iso_or_empty(str(conv.get("created_at") or conv.get("updated_at") or ""))
    messages = flatten_mobile_messages(conv)

    parts: list[str] = []
    for m in messages:
        role = (m.get("role") or "").strip() or "unknown"
        content = m.get("content")
        if isinstance(content, list):
            content = "\n".join(str(x) for x in content)
        content = (str(content) if content is not None else "").strip()
        if not content:
            continue
        parts.append(f"{role.capitalize()}:\n{content}".strip())

    full = "\n\n".join(parts).strip()
    return NormalizedChat(
        id=f"mobile:{conv_id or title}",
        source="mobile",
        title=title,
        content=full,
        workspace=workspace,
        created_at=created_at,
    )


SPEAKER_LINE_RE = re.compile(r"^\*\*(.+?)\*\*:\s*(.*)$")


def normalize_markdown_log(md_path: Path, workspace: str) -> NormalizedChat:
    text = md_path.read_text(encoding="utf-8", errors="replace")
    title = md_path.stem.replace("_", " ").strip()

    # Best-effort created_at from leading YYYY-MM-DD
    created_at = ""
    m = re.match(r"^(\d{4}-\d{2}-\d{2})_", md_path.name)
    if m:
        created_at = f"{m.group(1)}T00:00:00"

    # Best-effort message parsing: **Speaker**: message (possibly multi-line)
    messages: list[tuple[str, str]] = []
    current_speaker: Optional[str] = None
    buf: list[str] = []

    for line in text.splitlines():
        sm = SPEAKER_LINE_RE.match(line.strip())
        if sm:
            if current_speaker is not None and buf:
                messages.append((current_speaker, "\n".join(buf).strip()))
            current_speaker = sm.group(1).strip()
            buf = [sm.group(2).rstrip()]
            continue

        if current_speaker is not None:
            buf.append(line.rstrip())

    if current_speaker is not None and buf:
        messages.append((current_speaker, "\n".join(buf).strip()))

    if messages:
        content = "\n\n".join(
            f"{speaker}:\n{msg}".strip()
            for speaker, msg in messages
            if (msg or "").strip()
        ).strip()
    else:
        content = text.strip()

    return NormalizedChat(
        id=f"mdlog:{md_path.name}",
        source="discord",
        title=title,
        content=content,
        workspace=workspace,
        created_at=created_at,
    )


def main() -> int:
    parser = argparse.ArgumentParser(description="Ingest Cursor/mobile/markdown chats into SQLite.")
    parser.add_argument(
        "--db",
        default=r"D:\Rebirth\chat_indexer\chats.db",
        help="Output SQLite DB path",
    )
    parser.add_argument(
        "--workspace",
        default="audrey",
        help="Workspace label stored on each chat row",
    )
    parser.add_argument(
        "--cursor-snapshot",
        default=r"D:\Previously Claudia Core\.data\cursor_chats_snapshot.json",
        help="Cursor chats snapshot (metadata only)",
    )
    parser.add_argument(
        "--cursor-export-dir",
        default=r"D:\Previously Claudia Core\Documentation\Exported_Cursor_Chats",
        help="Directory containing exported Cursor chats markdown files (composer_*.md)",
    )
    parser.add_argument(
        "--mobile-json",
        default=r"D:\Previously Claudia Core\.data\mobile_conversations.json",
        help="mobile_conversations.json path",
    )
    parser.add_argument(
        "--markdown-dir",
        default=r"D:\Previously Claudia Core\.data\conversation_log",
        help="Directory containing markdown logs (*.md)",
    )
    parser.add_argument(
        "--reset",
        action="store_true",
        help="Delete existing chats table rows before ingest (keeps schema)",
    )

    args = parser.parse_args()

    db_path = Path(args.db)
    workspace = str(args.workspace)
    cursor_snapshot_path = Path(args.cursor_snapshot)
    cursor_export_dir = Path(args.cursor_export_dir)
    mobile_json_path = Path(args.mobile_json)
    markdown_dir = Path(args.markdown_dir)

    conn = connect_db(db_path)
    try:
        if args.reset:
            conn.execute("DELETE FROM chats;")

        set_metadata(conn, "ingest.ran_at", datetime.now(timezone.utc).isoformat().replace("+00:00", "Z"))
        set_metadata(conn, "ingest.db_path", str(db_path))
        set_metadata(conn, "ingest.workspace", workspace)

        # Cursor
        cursor_snapshot = load_cursor_snapshot(cursor_snapshot_path)
        cursor_by_id = index_cursor_snapshot_by_id(cursor_snapshot)
        set_metadata(conn, "cursor.snapshot.generated_at", str(cursor_snapshot.get("generated_at") or ""))
        set_metadata(conn, "cursor.snapshot.count", str(len(cursor_by_id)))

        cursor_export_stats = {
            "files_seen": 0,
            "has_full_messages": 0,
            "has_any_text": 0,
            "missing_content": 0,
        }
        if cursor_export_dir.exists():
            for chat, stats in iter_cursor_export_chats(cursor_export_dir, cursor_by_id, workspace):
                cursor_export_stats["files_seen"] += 1
                if stats.get("has_full_messages"):
                    cursor_export_stats["has_full_messages"] += 1
                if stats.get("has_inline_text") or stats.get("has_rich_text"):
                    cursor_export_stats["has_any_text"] += 1
                if (stats.get("content_len") or 0) < 50:
                    cursor_export_stats["missing_content"] += 1
                upsert_chat(conn, chat)
        set_metadata(conn, "cursor.export.dir", str(cursor_export_dir))
        set_metadata(conn, "cursor.export.files_seen", str(cursor_export_stats["files_seen"]))
        set_metadata(conn, "cursor.export.has_full_messages", str(cursor_export_stats["has_full_messages"]))
        set_metadata(conn, "cursor.export.has_any_text", str(cursor_export_stats["has_any_text"]))
        set_metadata(conn, "cursor.export.suspiciously_short", str(cursor_export_stats["missing_content"]))

        # Mobile
        mobile_count = 0
        if mobile_json_path.exists():
            mobile_data = json.loads(mobile_json_path.read_text(encoding="utf-8"))
            convs = mobile_data.get("conversations") or []
            for conv in convs:
                if not isinstance(conv, dict):
                    continue
                chat = normalize_mobile_conversation(conv, workspace)
                upsert_chat(conn, chat)
                mobile_count += 1
        set_metadata(conn, "mobile.json.path", str(mobile_json_path))
        set_metadata(conn, "mobile.conversations.count", str(mobile_count))

        # Markdown logs (Discord + other)
        md_count = 0
        if markdown_dir.exists():
            for md_path in sorted(markdown_dir.glob("*.md")):
                chat = normalize_markdown_log(md_path, workspace)
                upsert_chat(conn, chat)
                md_count += 1
        set_metadata(conn, "markdown.dir", str(markdown_dir))
        set_metadata(conn, "markdown.files_ingested", str(md_count))

        # Totals
        total = conn.execute("SELECT COUNT(*) FROM chats;").fetchone()[0]
        set_metadata(conn, "ingest.total_chats", str(total))
        for source, cnt in conn.execute("SELECT source, COUNT(*) FROM chats GROUP BY source;").fetchall():
            set_metadata(conn, f"ingest.source.{source}.count", str(cnt))

        conn.commit()
    finally:
        conn.close()

    try:
        db_bytes = db_path.stat().st_size
    except FileNotFoundError:
        db_bytes = 0

    # Post-run summary (also helps confirm Cursor content quality quickly).
    conn2 = sqlite3.connect(str(db_path))
    try:
        by_source = conn2.execute(
            "SELECT source, COUNT(*) FROM chats GROUP BY source ORDER BY COUNT(*) DESC"
        ).fetchall()
        cursor_only_seq = conn2.execute(
            "SELECT COUNT(*) FROM chats "
            "WHERE source='cursor' AND content LIKE '%Message sequence:%' "
            "AND content NOT LIKE '%Inline text:%' AND content NOT LIKE '%Rich text:%'"
        ).fetchone()[0]
    finally:
        conn2.close()

    print(f"Indexed chats into {db_path} ({db_bytes} bytes)")
    print("Counts by source:")
    for s, c in by_source:
        print(f"- {s}: {c}")
    print(f"Cursor chats with only message sequence (no inline/rich text): {cursor_only_seq}")

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
