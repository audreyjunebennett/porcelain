"""Durable Moto X v1 conversation journal and status model.

This module deliberately has no Flask, Torch, or Whisper dependencies.  The
receiver can use it from its threaded request handlers, and the state machine
can be tested without loading the audio stack.
"""

from __future__ import annotations

import os
import re
import sqlite3
import threading
import uuid
from contextlib import contextmanager
from dataclasses import dataclass
from datetime import datetime, timedelta
from pathlib import Path
from typing import Any


LEGACY_EPOCH_FILENAME_RE = re.compile(r"^(\d{10})$")


def legacy_capture_identity(filename: str | None) -> tuple[str, str] | None:
    """Recover stable identity/time from record.py's ``<epoch>.aac`` names."""
    stem = Path(filename or "").stem
    match = LEGACY_EPOCH_FILENAME_RE.fullmatch(stem)
    if match is None:
        return None
    try:
        captured = datetime.fromtimestamp(int(match.group(1)))
    except (OverflowError, OSError, ValueError):
        return None
    if not 2000 <= captured.year <= 2100:
        return None
    return f"legacy-{stem}", captured.strftime("%Y-%m-%d_%H-%M-%S")


TIMESTAMP_FORMAT = "%Y-%m-%d_%H-%M-%S"


def parse_timestamp(value: str) -> datetime:
    return datetime.strptime(value, TIMESTAMP_FORMAT)


def display_time(value: str) -> str:
    return parse_timestamp(value).strftime("%I:%M:%S %p").lstrip("0")


@dataclass(frozen=True)
class ChunkEvent:
    capture_id: str
    captured_at: str
    kind: str
    duration_seconds: float | None = None
    speech_seconds: float | None = None
    rms: float | None = None
    audio_path: str | None = None
    transcript: str = ""
    speaker: str = "Unsorted"


class MotoXStore:
    """SQLite-backed event store with deterministic Markdown projections."""

    def __init__(
        self,
        database_path: Path,
        daily_dir: Path,
        gap_seconds: int = 120,
        expected_chunk_seconds: int = 45,
    ) -> None:
        self.database_path = Path(database_path)
        self.daily_dir = Path(daily_dir)
        self.gap_seconds = int(gap_seconds)
        self.expected_chunk_seconds = int(expected_chunk_seconds)
        self._lock = threading.RLock()
        self.database_path.parent.mkdir(parents=True, exist_ok=True)
        self.daily_dir.mkdir(parents=True, exist_ok=True)
        self._initialize()

    def _connect(self) -> sqlite3.Connection:
        conn = sqlite3.connect(self.database_path, timeout=10)
        conn.row_factory = sqlite3.Row
        conn.execute("PRAGMA foreign_keys = ON")
        conn.execute("PRAGMA busy_timeout = 10000")
        return conn

    @contextmanager
    def _connection(self):
        conn = self._connect()
        try:
            yield conn
        finally:
            conn.close()

    def _initialize(self) -> None:
        with self._connection() as conn:
            conn.executescript(
                """
                PRAGMA journal_mode = WAL;

                CREATE TABLE IF NOT EXISTS conversations (
                    conversation_id TEXT PRIMARY KEY,
                    started_at TEXT NOT NULL,
                    last_speech_at TEXT NOT NULL,
                    ended_at TEXT,
                    status TEXT NOT NULL CHECK (status IN ('open', 'closed')),
                    close_reason TEXT
                );

                CREATE UNIQUE INDEX IF NOT EXISTS one_open_conversation
                    ON conversations(status) WHERE status = 'open';

                CREATE TABLE IF NOT EXISTS chunks (
                    capture_id TEXT PRIMARY KEY,
                    captured_at TEXT NOT NULL,
                    kind TEXT NOT NULL CHECK (kind IN ('speech', 'ambient', 'silence')),
                    duration_seconds REAL,
                    speech_seconds REAL,
                    rms REAL,
                    audio_path TEXT,
                    transcript TEXT NOT NULL DEFAULT '',
                    speaker TEXT NOT NULL DEFAULT 'Unsorted',
                    conversation_id TEXT REFERENCES conversations(conversation_id),
                    received_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
                );

                CREATE INDEX IF NOT EXISTS chunks_by_time ON chunks(captured_at);
                CREATE INDEX IF NOT EXISTS chunks_by_conversation
                    ON chunks(conversation_id, captured_at);

                CREATE TABLE IF NOT EXISTS transcription_passes (
                    pass_id TEXT PRIMARY KEY,
                    capture_id TEXT NOT NULL REFERENCES chunks(capture_id),
                    pass_kind TEXT NOT NULL,
                    model_name TEXT NOT NULL,
                    model_version TEXT,
                    transcript TEXT NOT NULL,
                    is_current INTEGER NOT NULL DEFAULT 1,
                    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
                );

                CREATE INDEX IF NOT EXISTS transcription_passes_current
                    ON transcription_passes(capture_id, pass_kind, is_current, created_at);

                CREATE TABLE IF NOT EXISTS transcription_words (
                    pass_id TEXT NOT NULL REFERENCES transcription_passes(pass_id),
                    word_index INTEGER NOT NULL,
                    word TEXT NOT NULL,
                    start_seconds REAL NOT NULL,
                    end_seconds REAL NOT NULL,
                    probability REAL,
                    char_start INTEGER NOT NULL,
                    char_end INTEGER NOT NULL,
                    PRIMARY KEY (pass_id, word_index)
                );

                CREATE TABLE IF NOT EXISTS recorder_status (
                    singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
                    reported_at TEXT NOT NULL,
                    recorder TEXT NOT NULL,
                    queue_depth INTEGER NOT NULL,
                    cycle_seconds REAL
                );
                """
            )
            conn.commit()

    def record_transcription_pass(
        self,
        capture_id: str,
        transcript: str,
        words: list[dict[str, Any]],
        *,
        pass_kind: str = "quick_chunk",
        model_name: str = "faster-whisper",
        model_version: str | None = None,
    ) -> str:
        """Append a versioned ASR pass and its source-audio word timings."""
        pass_id = uuid.uuid4().hex
        with self._lock, self._connection() as conn:
            if conn.execute(
                "SELECT 1 FROM chunks WHERE capture_id = ?", (capture_id,)
            ).fetchone() is None:
                raise KeyError("unknown capture")
            conn.execute("BEGIN IMMEDIATE")
            conn.execute(
                "UPDATE transcription_passes SET is_current = 0 "
                "WHERE capture_id = ? AND pass_kind = ? AND is_current = 1",
                (capture_id, pass_kind),
            )
            conn.execute(
                """
                INSERT INTO transcription_passes (
                    pass_id, capture_id, pass_kind, model_name, model_version,
                    transcript, is_current
                ) VALUES (?, ?, ?, ?, ?, ?, 1)
                """,
                (
                    pass_id,
                    capture_id,
                    pass_kind,
                    model_name,
                    model_version,
                    transcript,
                ),
            )
            rows = []
            for index, word in enumerate(words):
                start = max(0.0, float(word["start_seconds"]))
                end = max(start, float(word["end_seconds"]))
                char_start = max(0, int(word["char_start"]))
                char_end = max(char_start, int(word["char_end"]))
                rows.append(
                    (
                        pass_id,
                        index,
                        str(word["word"]),
                        start,
                        end,
                        word.get("probability"),
                        char_start,
                        char_end,
                    )
                )
            if rows:
                conn.executemany(
                    """
                    INSERT INTO transcription_words (
                        pass_id, word_index, word, start_seconds, end_seconds,
                        probability, char_start, char_end
                    ) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
                    """,
                    rows,
                )
            conn.commit()
        return pass_id

    def has_chunk(self, capture_id: str) -> bool:
        with self._connection() as conn:
            row = conn.execute(
                "SELECT 1 FROM chunks WHERE capture_id = ?", (capture_id,)
            ).fetchone()
        return row is not None

    def record_chunk(self, event: ChunkEvent) -> dict[str, Any]:
        if event.kind not in {"speech", "ambient", "silence"}:
            raise ValueError(f"unsupported chunk kind: {event.kind}")
        parse_timestamp(event.captured_at)
        if not event.capture_id.strip():
            raise ValueError("capture_id must not be empty")

        affected_dates = {event.captured_at[:10]}
        with self._lock:
            with self._connection() as conn:
                conn.execute("BEGIN IMMEDIATE")
                duplicate = conn.execute(
                    "SELECT conversation_id FROM chunks WHERE capture_id = ?",
                    (event.capture_id,),
                ).fetchone()
                if duplicate is not None:
                    conn.rollback()
                    return {
                        "duplicate": True,
                        "capture_id": event.capture_id,
                        "conversation_id": duplicate["conversation_id"],
                    }

                active = conn.execute(
                    "SELECT * FROM conversations WHERE status = 'open'"
                ).fetchone()
                conversation_id: str | None = None
                closed_id: str | None = None

                if active is not None:
                    rows = conn.execute(
                        "SELECT DISTINCT substr(captured_at, 1, 10) AS day "
                        "FROM chunks WHERE conversation_id = ?",
                        (active["conversation_id"],),
                    ).fetchall()
                    affected_dates.update(row["day"] for row in rows)

                if event.kind == "speech":
                    if active is not None and self._gap(active, event.captured_at) >= self.gap_seconds:
                        closed_id = active["conversation_id"]
                        self._close(conn, active, event.captured_at, "speech_gap")
                        active = None
                    if active is None:
                        conversation_id = self._start(conn, event.captured_at)
                    else:
                        conversation_id = active["conversation_id"]
                        conn.execute(
                            "UPDATE conversations SET last_speech_at = ? "
                            "WHERE conversation_id = ?",
                            (event.captured_at, conversation_id),
                        )
                elif active is not None:
                    gap = self._gap(active, event.captured_at)
                    if gap >= self.gap_seconds:
                        closed_id = active["conversation_id"]
                        self._close(conn, active, event.captured_at, "silence_gap")
                    elif event.kind == "ambient":
                        # Ambient context may belong to an open conversation, but it
                        # never resets the speech clock.
                        conversation_id = active["conversation_id"]

                conn.execute(
                    """
                    INSERT INTO chunks (
                        capture_id, captured_at, kind, duration_seconds,
                        speech_seconds, rms, audio_path, transcript, speaker,
                        conversation_id
                    ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
                    """,
                    (
                        event.capture_id,
                        event.captured_at,
                        event.kind,
                        event.duration_seconds,
                        event.speech_seconds,
                        event.rms,
                        event.audio_path,
                        event.transcript.strip(),
                        event.speaker,
                        conversation_id,
                    ),
                )
                conn.commit()

            for day in sorted(affected_dates):
                self.render_day(day)

        return {
            "duplicate": False,
            "capture_id": event.capture_id,
            "conversation_id": conversation_id,
            "closed_conversation_id": closed_id,
            "daily_path": str(self.daily_path(event.captured_at[:10])),
        }

    def _gap(self, active: sqlite3.Row, captured_at: str) -> float:
        return (
            parse_timestamp(captured_at) - parse_timestamp(active["last_speech_at"])
        ).total_seconds()

    def _start(self, conn: sqlite3.Connection, captured_at: str) -> str:
        conversation_id = f"conversation_{captured_at}"
        suffix = 1
        candidate = conversation_id
        while conn.execute(
            "SELECT 1 FROM conversations WHERE conversation_id = ?", (candidate,)
        ).fetchone():
            suffix += 1
            candidate = f"{conversation_id}_{suffix}"
        conn.execute(
            "INSERT INTO conversations "
            "(conversation_id, started_at, last_speech_at, status) "
            "VALUES (?, ?, ?, 'open')",
            (candidate, captured_at, captured_at),
        )
        return candidate

    @staticmethod
    def _close(
        conn: sqlite3.Connection,
        active: sqlite3.Row,
        observed_at: str,
        reason: str,
    ) -> None:
        # End time is the last speech, not the later chunk that proved the gap.
        conn.execute(
            "UPDATE conversations SET status = 'closed', ended_at = ?, "
            "close_reason = ? WHERE conversation_id = ?",
            (active["last_speech_at"], reason, active["conversation_id"]),
        )

    def daily_path(self, day: str) -> Path:
        datetime.strptime(day, "%Y-%m-%d")
        return self.daily_dir / f"{day}.md"

    @staticmethod
    def _privacy_filter(conn: sqlite3.Connection, chunk_alias: str) -> str:
        exists = conn.execute(
            "SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = 'review_annotations'"
        ).fetchone()
        if not exists:
            return ""
        return f"""
            AND NOT EXISTS (
                SELECT 1 FROM review_annotations excluded
                WHERE excluded.capture_id = {chunk_alias}.capture_id
                  AND excluded.annotation_type = 'privacy'
                  AND excluded.label = 'Exclude'
                  AND excluded.reverted_at IS NULL
            )
        """

    def _journal_content_for_range(
        self,
        start_at: str,
        end_at: str,
        heading: str,
        *,
        show_dates: bool = False,
    ) -> str:
        with self._connection() as conn:
            privacy_filter = self._privacy_filter(conn, "k")
            conversations = conn.execute(
                f"""
                SELECT DISTINCT c.*
                FROM conversations c
                JOIN chunks k ON k.conversation_id = c.conversation_id
                WHERE k.captured_at >= ? AND k.captured_at < ?
                  AND k.kind = 'speech'
                {privacy_filter}
                ORDER BY c.started_at, c.conversation_id
                """,
                (start_at, end_at),
            ).fetchall()

            lines = [
                f"# Moto X Journal — {heading}",
                "",
                "<!-- Generated from motox_v1.sqlite3. Rebuild instead of editing this file. -->",
                "",
            ]
            if not conversations:
                lines.extend(["_No conversations recorded._", ""])

            for conversation in conversations:
                privacy_filter = self._privacy_filter(conn, "chunks")
                chunks = conn.execute(
                    f"""
                    SELECT * FROM chunks
                    WHERE conversation_id = ?
                      AND captured_at >= ? AND captured_at < ?
                      {privacy_filter}
                    ORDER BY captured_at, capture_id
                    """,
                    (conversation["conversation_id"], start_at, end_at),
                ).fetchall()
                speech = [row for row in chunks if row["kind"] == "speech"]
                if not speech:
                    continue
                def journal_time(value: str) -> str:
                    if not show_dates:
                        return display_time(value)
                    return parse_timestamp(value).strftime("%b %d · %I:%M:%S %p").replace(" 0", " ")

                start = journal_time(speech[0]["captured_at"])
                end = journal_time(speech[-1]["captured_at"])
                state = "active" if conversation["status"] == "open" else "completed"
                lines.extend(
                    [
                        f"## {start}–{end} · {state}",
                        "",
                        f"<a id=\"{conversation['conversation_id']}\"></a>",
                        "",
                    ]
                )
                for chunk in speech:
                    transcript = chunk["transcript"].strip()
                    if transcript:
                        lines.extend([f"**{journal_time(chunk['captured_at'])}**", "", transcript, ""])
                    if chunk["audio_path"]:
                        audio_name = Path(chunk["audio_path"]).name
                        lines.extend([f"[Audio](../audio/{audio_name})", ""])
                ambient_count = sum(1 for row in chunks if row["kind"] == "ambient")
                if ambient_count:
                    lines.extend([f"<!-- {ambient_count} ambient context chunk(s) attached. -->", ""])

        return "\n".join(lines).rstrip() + "\n"

    def render_day(self, day: str) -> Path:
        path = self.daily_path(day)
        start = datetime.strptime(day, "%Y-%m-%d")
        content = self._journal_content_for_range(
            start.strftime(TIMESTAMP_FORMAT),
            (start + timedelta(days=1)).strftime(TIMESTAMP_FORMAT),
            day,
        )
        temp_path = path.with_suffix(".md.tmp")
        temp_path.write_text(content, encoding="utf-8")
        os.replace(temp_path, path)
        return path

    def journal_text(self, day: str) -> str:
        path = self.render_day(day)
        return path.read_text(encoding="utf-8")

    def recent_journal_text(
        self, now: datetime | None = None, *, hours: float = 24.0
    ) -> str:
        """Render an exact rolling window without altering dated journal files."""

        now = now or datetime.now()
        safe_hours = max(1.0, min(float(hours), 168.0))
        start = now - timedelta(hours=safe_hours)
        return self._journal_content_for_range(
            start.strftime(TIMESTAMP_FORMAT),
            now.strftime(TIMESTAMP_FORMAT),
            f"last {safe_hours:g} hours",
            show_dates=True,
        )

    def recent_transcript(self, limit: int = 4, offset: int = 0) -> list[dict[str, Any]]:
        safe_limit = max(1, min(int(limit), 20))
        safe_offset = max(0, int(offset))
        with self._connection() as conn:
            privacy_filter = self._privacy_filter(conn, "chunks")
            rows = conn.execute(
                f"""
                SELECT capture_id, captured_at, transcript, speaker, conversation_id,
                       duration_seconds
                FROM chunks
                WHERE kind = 'speech' AND trim(transcript) <> ''
                {privacy_filter}
                ORDER BY captured_at DESC, capture_id DESC
                LIMIT ? OFFSET ?
                """,
                (safe_limit, safe_offset),
            ).fetchall()
        return [dict(row) for row in reversed(rows)]

    def update_recorder_status(
        self,
        reported_at: str,
        recorder: str,
        queue_depth: int,
        cycle_seconds: float | None = None,
    ) -> None:
        parse_timestamp(reported_at)
        safe_depth = max(0, int(queue_depth))
        with self._connection() as conn:
            conn.execute(
                """
                INSERT INTO recorder_status (
                    singleton, reported_at, recorder, queue_depth, cycle_seconds
                ) VALUES (1, ?, ?, ?, ?)
                ON CONFLICT(singleton) DO UPDATE SET
                    reported_at = excluded.reported_at,
                    recorder = excluded.recorder,
                    queue_depth = excluded.queue_depth,
                    cycle_seconds = excluded.cycle_seconds
                """,
                (reported_at, recorder.strip() or "unknown", safe_depth, cycle_seconds),
            )
            conn.commit()

    def status(self, now: datetime | None = None) -> dict[str, Any]:
        now = now or datetime.now()
        day = now.strftime("%Y-%m-%d")
        with self._connection() as conn:
            latest = conn.execute(
                "SELECT * FROM chunks ORDER BY captured_at DESC, capture_id DESC LIMIT 1"
            ).fetchone()
            active = conn.execute(
                "SELECT * FROM conversations WHERE status = 'open'"
            ).fetchone()
            counts = conn.execute(
                """
                SELECT kind, count(*) AS count FROM chunks
                WHERE substr(captured_at, 1, 10) = ? GROUP BY kind
                """,
                (day,),
            ).fetchall()
            recorder = conn.execute(
                "SELECT reported_at, recorder, queue_depth, cycle_seconds "
                "FROM recorder_status WHERE singleton = 1"
            ).fetchone()

        age_seconds: int | None = None
        health = "waiting"
        if latest is not None:
            age_seconds = max(
                0, int((now - parse_timestamp(latest["captured_at"])).total_seconds())
            )
            if age_seconds <= self.expected_chunk_seconds * 2:
                health = "healthy"
            elif age_seconds <= self.expected_chunk_seconds * 4:
                health = "late"
            else:
                health = "stale"

        return {
            "capture_health": health,
            "last_chunk_at": latest["captured_at"] if latest else None,
            "last_chunk_kind": latest["kind"] if latest else None,
            "last_chunk_age_seconds": age_seconds,
            "active_conversation": dict(active) if active else None,
            "today": day,
            "today_counts": {row["kind"]: row["count"] for row in counts},
            "daily_path": str(self.daily_path(day)),
            "conversation_gap_seconds": self.gap_seconds,
            "recorder": dict(recorder) if recorder else None,
        }


def event_from_receiver(
    capture_id: str,
    captured_at: str,
    kind: str,
    classification: dict[str, Any],
    audio_path: Path | None,
    transcript: str,
    speaker: str,
) -> ChunkEvent:
    return ChunkEvent(
        capture_id=capture_id,
        captured_at=captured_at,
        kind=kind,
        duration_seconds=classification.get("duration_seconds"),
        speech_seconds=classification.get("speech_seconds"),
        rms=classification.get("rms"),
        audio_path=str(audio_path) if audio_path else None,
        transcript=transcript,
        speaker=speaker,
    )
