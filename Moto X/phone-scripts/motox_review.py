"""Durable, reversible review data for the Moto X memory journal.

Raw audio and machine transcripts remain in the existing ``chunks`` table.
This module stores model proposals and human corrections as additive layers.
"""

from __future__ import annotations

import sqlite3
import threading
import uuid
from contextlib import contextmanager
from datetime import datetime
from pathlib import Path
from typing import Any


SPEAKER_LABELS = ("Ruby", "Lynn", "Raven", "Other")
SOUND_LABELS = ("Hahli", "Lam", "Television", "Music", "Not speech")
SPECIAL_LABELS = ("Overlap",)
ALL_LABELS = SPEAKER_LABELS + SOUND_LABELS + SPECIAL_LABELS
ANNOTATION_TYPES = ("speaker", "sound", "overlap", "transcript")


def _now() -> str:
    return datetime.now().astimezone().isoformat(timespec="seconds")


class MotoXReviewStore:
    """Read review candidates and append corrections to a Moto X database."""

    def __init__(self, database_path: Path):
        self.database_path = Path(database_path)
        self._write_lock = threading.Lock()
        self._initialize()

    @contextmanager
    def _connect(self):
        connection = sqlite3.connect(self.database_path, timeout=15)
        connection.row_factory = sqlite3.Row
        connection.execute("PRAGMA foreign_keys = ON")
        connection.execute("PRAGMA busy_timeout = 15000")
        try:
            yield connection
            connection.commit()
        except Exception:
            connection.rollback()
            raise
        finally:
            connection.close()

    def _initialize(self) -> None:
        self.database_path.parent.mkdir(parents=True, exist_ok=True)
        with self._connect() as connection:
            connection.execute(
                """
                CREATE TABLE IF NOT EXISTS review_annotations (
                    annotation_id TEXT PRIMARY KEY,
                    capture_id TEXT NOT NULL,
                    start_char INTEGER NOT NULL,
                    end_char INTEGER NOT NULL,
                    selected_text TEXT NOT NULL DEFAULT '',
                    annotation_type TEXT NOT NULL,
                    label TEXT,
                    replacement_text TEXT,
                    source TEXT NOT NULL DEFAULT 'human',
                    created_at TEXT NOT NULL,
                    reverted_at TEXT,
                    FOREIGN KEY (capture_id) REFERENCES chunks(capture_id)
                )
                """
            )
            connection.execute(
                """
                CREATE INDEX IF NOT EXISTS idx_review_annotations_capture_active
                ON review_annotations(capture_id, created_at)
                WHERE reverted_at IS NULL
                """
            )
            connection.execute(
                """
                CREATE TABLE IF NOT EXISTS review_proposals (
                    proposal_id TEXT PRIMARY KEY,
                    capture_id TEXT NOT NULL,
                    start_char INTEGER NOT NULL DEFAULT 0,
                    end_char INTEGER NOT NULL DEFAULT 0,
                    proposal_type TEXT NOT NULL,
                    label TEXT,
                    confidence REAL,
                    cluster_id TEXT,
                    model_name TEXT NOT NULL,
                    model_version TEXT,
                    created_at TEXT NOT NULL,
                    FOREIGN KEY (capture_id) REFERENCES chunks(capture_id)
                )
                """
            )
            connection.execute(
                """
                CREATE INDEX IF NOT EXISTS idx_review_proposals_capture_type
                ON review_proposals(capture_id, proposal_type, created_at)
                """
            )
            connection.execute("PRAGMA optimize")

    def _active_annotations(
        self, connection: sqlite3.Connection, capture_ids: list[str]
    ) -> dict[str, list[dict[str, Any]]]:
        if not capture_ids:
            return {}
        placeholders = ",".join("?" for _ in capture_ids)
        rows = connection.execute(
            f"""
            SELECT * FROM review_annotations
            WHERE reverted_at IS NULL AND capture_id IN ({placeholders})
            ORDER BY created_at, annotation_id
            """,
            capture_ids,
        ).fetchall()
        result: dict[str, list[dict[str, Any]]] = {}
        for row in rows:
            result.setdefault(row["capture_id"], []).append(dict(row))
        return result

    def _latest_proposals(
        self, connection: sqlite3.Connection, capture_ids: list[str]
    ) -> dict[str, dict[str, Any]]:
        if not capture_ids:
            return {}
        placeholders = ",".join("?" for _ in capture_ids)
        rows = connection.execute(
            f"""
            SELECT * FROM review_proposals
            WHERE capture_id IN ({placeholders})
            ORDER BY created_at DESC, proposal_id DESC
            """,
            capture_ids,
        ).fetchall()
        result: dict[str, dict[str, Any]] = {}
        for row in rows:
            result.setdefault(row["capture_id"], dict(row))
        return result

    def candidates(
        self,
        *,
        limit: int = 24,
        target_seconds: float = 300,
        before: str | None = None,
        group: str | None = None,
    ) -> list[dict[str, Any]]:
        """Return enough recent speech chunks to fill a review batch.

        ``group`` may be a proposed identity or anonymous cluster. A human label
        never silently replaces the underlying machine proposal.
        """

        limit = max(1, min(int(limit), 100))
        target_seconds = max(30.0, min(float(target_seconds), 1800.0))
        with self._connect() as connection:
            parameters: list[Any] = []
            before_clause = ""
            if before:
                before_clause = "AND captured_at < ?"
                parameters.append(before)
            rows = connection.execute(
                f"""
                SELECT capture_id, captured_at, duration_seconds, speech_seconds,
                       audio_path, transcript, conversation_id
                FROM chunks
                WHERE kind = 'speech'
                  AND trim(transcript) <> ''
                  AND audio_path IS NOT NULL
                  {before_clause}
                ORDER BY captured_at DESC, capture_id DESC
                LIMIT 600
                """,
                parameters,
            ).fetchall()
            ids = [row["capture_id"] for row in rows]
            annotations = self._active_annotations(connection, ids)
            proposals = self._latest_proposals(connection, ids)

        result: list[dict[str, Any]] = []
        accumulated = 0.0
        for row in rows:
            audio_path = Path(row["audio_path"])
            if not audio_path.is_file():
                continue
            proposal = proposals.get(row["capture_id"])
            proposed_group = None
            if proposal:
                proposed_group = proposal.get("label") or proposal.get("cluster_id")
            if group and group != "all" and (proposed_group or "Unsorted") != group:
                continue
            item = dict(row)
            item["duration_seconds"] = float(item["duration_seconds"] or 30.0)
            item["speech_seconds"] = float(item["speech_seconds"] or 0.0)
            item["annotations"] = annotations.get(row["capture_id"], [])
            item["proposal"] = proposal
            item["review_group"] = proposed_group or "Unsorted"
            item.pop("audio_path", None)
            result.append(item)
            accumulated += item["duration_seconds"]
            if len(result) >= limit or accumulated >= target_seconds:
                break
        return result

    def get_audio_path(self, capture_id: str) -> Path | None:
        with self._connect() as connection:
            row = connection.execute(
                "SELECT audio_path FROM chunks WHERE capture_id = ?", (capture_id,)
            ).fetchone()
        if not row or not row["audio_path"]:
            return None
        path = Path(row["audio_path"])
        return path if path.is_file() else None

    def add_annotation(
        self,
        *,
        capture_id: str,
        start_char: int,
        end_char: int,
        annotation_type: str,
        selected_text: str = "",
        label: str | None = None,
        replacement_text: str | None = None,
    ) -> dict[str, Any]:
        annotation_type = str(annotation_type).strip().lower()
        if annotation_type not in ANNOTATION_TYPES:
            raise ValueError("unsupported annotation type")
        if annotation_type != "transcript" and label not in ALL_LABELS:
            raise ValueError("unsupported label")
        if annotation_type == "transcript" and not (replacement_text or "").strip():
            raise ValueError("replacement text is required")

        with self._write_lock, self._connect() as connection:
            chunk = connection.execute(
                "SELECT transcript FROM chunks WHERE capture_id = ?", (capture_id,)
            ).fetchone()
            if not chunk:
                raise KeyError("unknown capture")
            transcript = chunk["transcript"] or ""
            start_char = max(0, int(start_char))
            end_char = min(len(transcript), int(end_char))
            if end_char <= start_char:
                start_char, end_char = 0, len(transcript)
            selected_text = transcript[start_char:end_char]
            annotation = {
                "annotation_id": uuid.uuid4().hex,
                "capture_id": capture_id,
                "start_char": start_char,
                "end_char": end_char,
                "selected_text": selected_text,
                "annotation_type": annotation_type,
                "label": label,
                "replacement_text": (
                    replacement_text.strip() if replacement_text is not None else None
                ),
                "source": "human",
                "created_at": _now(),
                "reverted_at": None,
            }
            connection.execute(
                """
                INSERT INTO review_annotations (
                    annotation_id, capture_id, start_char, end_char, selected_text,
                    annotation_type, label, replacement_text, source, created_at,
                    reverted_at
                ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
                """,
                tuple(annotation.values()),
            )
        return annotation

    def revert_annotation(self, annotation_id: str) -> bool:
        with self._write_lock, self._connect() as connection:
            cursor = connection.execute(
                """
                UPDATE review_annotations SET reverted_at = ?
                WHERE annotation_id = ? AND reverted_at IS NULL
                """,
                (_now(), annotation_id),
            )
        return cursor.rowcount == 1

    def progress(self) -> dict[str, Any]:
        seconds = {label: 0.0 for label in SPEAKER_LABELS + SOUND_LABELS[:2]}
        counts = {label: 0 for label in seconds}
        with self._connect() as connection:
            rows = connection.execute(
                """
                SELECT a.label, a.capture_id,
                       MAX(COALESCE(c.speech_seconds, c.duration_seconds, 30.0)) AS speech_seconds,
                       MAX(length(c.transcript)) AS transcript_chars,
                       SUM(MAX(1, a.end_char - a.start_char)) AS labeled_chars
                FROM review_annotations a
                JOIN chunks c ON c.capture_id = a.capture_id
                WHERE a.reverted_at IS NULL
                  AND a.annotation_type IN ('speaker', 'sound')
                  AND a.label IS NOT NULL
                GROUP BY a.label, a.capture_id
                """
            ).fetchall()
        for row in rows:
            label = row["label"]
            if label in seconds:
                fraction = min(
                    1.0,
                    float(row["labeled_chars"] or 0.0)
                    / max(1.0, float(row["transcript_chars"] or 0.0)),
                )
                seconds[label] += float(row["speech_seconds"] or 0.0) * fraction
                counts[label] += 1
        return {
            label: {"seconds": round(seconds[label], 1), "clips": counts[label]}
            for label in seconds
        }

    def groups(self) -> list[dict[str, Any]]:
        with self._connect() as connection:
            rows = connection.execute(
                """
                SELECT COALESCE(label, cluster_id, 'Unsorted') AS name,
                       COUNT(DISTINCT capture_id) AS clips,
                       AVG(confidence) AS confidence
                FROM review_proposals
                GROUP BY COALESCE(label, cluster_id, 'Unsorted')
                ORDER BY clips DESC, name
                """
            ).fetchall()
        result = [dict(row) for row in rows]
        if not any(row["name"] == "Unsorted" for row in result):
            result.append({"name": "Unsorted", "clips": None, "confidence": None})
        return result

    def add_proposals(
        self,
        proposals: list[dict[str, Any]],
        *,
        model_name: str,
        model_version: str | None = None,
    ) -> int:
        """Append a versioned batch of model guesses without accepting them."""

        created_at = _now()
        rows = []
        for proposal in proposals:
            capture_id = str(proposal.get("capture_id", ""))
            if not capture_id:
                continue
            rows.append(
                (
                    uuid.uuid4().hex,
                    capture_id,
                    max(0, int(proposal.get("start_char", 0))),
                    max(0, int(proposal.get("end_char", 0))),
                    str(proposal.get("proposal_type", "speaker")),
                    proposal.get("label"),
                    proposal.get("confidence"),
                    proposal.get("cluster_id"),
                    model_name,
                    model_version,
                    created_at,
                )
            )
        if not rows:
            return 0
        with self._write_lock, self._connect() as connection:
            connection.executemany(
                """
                INSERT INTO review_proposals (
                    proposal_id, capture_id, start_char, end_char, proposal_type,
                    label, confidence, cluster_id, model_name, model_version,
                    created_at
                ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
                """,
                rows,
            )
        return len(rows)
