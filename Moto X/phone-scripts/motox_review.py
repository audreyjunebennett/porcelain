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
from math import isfinite
from pathlib import Path
from typing import Any


SPEAKER_LABELS = ("Ruby", "Lynn", "Raven", "Other", "Unknown person", "Not sure")
SOUND_LABELS = ("Hahli", "Lam", "Television", "Music", "Not speech")
SPECIAL_LABELS = ("Overlap",)
BOUNDARY_LABELS = ("Continues before", "Continues after", "Continues both")
PRIVACY_LABELS = ("Exclude",)
ALL_LABELS = SPEAKER_LABELS + SOUND_LABELS + SPECIAL_LABELS + BOUNDARY_LABELS + PRIVACY_LABELS
ANNOTATION_TYPES = ("speaker", "sound", "overlap", "transcript", "boundary", "privacy")


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
                    audio_start_seconds REAL,
                    audio_end_seconds REAL,
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
            annotation_columns = {
                row["name"]
                for row in connection.execute("PRAGMA table_info(review_annotations)")
            }
            if "audio_start_seconds" not in annotation_columns:
                connection.execute(
                    "ALTER TABLE review_annotations ADD COLUMN audio_start_seconds REAL"
                )
            if "audio_end_seconds" not in annotation_columns:
                connection.execute(
                    "ALTER TABLE review_annotations ADD COLUMN audio_end_seconds REAL"
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

    def _latest_transcriptions(
        self, connection: sqlite3.Connection, capture_ids: list[str]
    ) -> dict[str, dict[str, Any]]:
        if not capture_ids:
            return {}
        placeholders = ",".join("?" for _ in capture_ids)
        passes = connection.execute(
            f"""
            SELECT * FROM transcription_passes
            WHERE is_current = 1 AND capture_id IN ({placeholders})
            ORDER BY created_at DESC, pass_id DESC
            """,
            capture_ids,
        ).fetchall()
        result: dict[str, dict[str, Any]] = {}
        for row in passes:
            result.setdefault(row["capture_id"], dict(row))
        pass_ids = [row["pass_id"] for row in result.values()]
        if pass_ids:
            word_placeholders = ",".join("?" for _ in pass_ids)
            words = connection.execute(
                f"""
                SELECT * FROM transcription_words
                WHERE pass_id IN ({word_placeholders})
                ORDER BY pass_id, word_index
                """,
                pass_ids,
            ).fetchall()
            by_pass: dict[str, list[dict[str, Any]]] = {}
            for word in words:
                by_pass.setdefault(word["pass_id"], []).append(dict(word))
            for item in result.values():
                item["words"] = by_pass.get(item["pass_id"], [])
        return result

    @staticmethod
    def _corrected_transcript(text: str, annotations: list[dict[str, Any]]) -> str:
        corrections = [
            row for row in annotations
            if row["annotation_type"] == "transcript" and row.get("replacement_text") is not None
        ]
        # Applying from right to left preserves the original character anchors.
        for row in sorted(corrections, key=lambda item: (item["start_char"], item["created_at"]), reverse=True):
            start = max(0, min(len(text), int(row["start_char"])))
            end = max(start, min(len(text), int(row["end_char"])))
            text = text[:start] + str(row["replacement_text"]) + text[end:]
        return text

    def _decorate_rows(
        self,
        rows: list[sqlite3.Row],
        annotations: dict[str, list[dict[str, Any]]],
        proposals: dict[str, dict[str, Any]],
        transcriptions: dict[str, dict[str, Any]],
    ) -> list[dict[str, Any]]:
        result = []
        for row in rows:
            audio_path = Path(row["audio_path"])
            if not audio_path.is_file():
                continue
            item = dict(row)
            item["duration_seconds"] = float(item["duration_seconds"] or 30.0)
            item["speech_seconds"] = float(item["speech_seconds"] or 0.0)
            item_annotations = annotations.get(row["capture_id"], [])
            transcription = transcriptions.get(row["capture_id"])
            item["raw_transcript"] = item["transcript"]
            if transcription:
                item["transcript"] = transcription["transcript"]
                item["words"] = transcription.get("words", [])
                item["transcription_pass"] = {
                    key: transcription.get(key)
                    for key in ("pass_id", "pass_kind", "model_name", "model_version", "created_at")
                }
            else:
                item["words"] = []
                item["transcription_pass"] = None
            item["annotations"] = item_annotations
            item["corrected_transcript"] = self._corrected_transcript(
                item["transcript"], item_annotations
            )
            proposal = proposals.get(row["capture_id"])
            item["proposal"] = proposal
            item["review_group"] = (
                (proposal.get("label") or proposal.get("cluster_id")) if proposal else None
            ) or "Unsorted"
            item.pop("audio_path", None)
            result.append(item)
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
                  AND NOT EXISTS (
                      SELECT 1 FROM review_annotations excluded
                      WHERE excluded.capture_id = chunks.capture_id
                        AND excluded.annotation_type = 'privacy'
                        AND excluded.label = 'Exclude'
                        AND excluded.reverted_at IS NULL
                  )
                  {before_clause}
                ORDER BY captured_at DESC, capture_id DESC
                LIMIT 600
                """,
                parameters,
            ).fetchall()
            ids = [row["capture_id"] for row in rows]
            annotations = self._active_annotations(connection, ids)
            proposals = self._latest_proposals(connection, ids)
            transcriptions = self._latest_transcriptions(connection, ids)

        decorated = self._decorate_rows(rows, annotations, proposals, transcriptions)
        result: list[dict[str, Any]] = []
        accumulated = 0.0
        for item in decorated:
            if group and group != "all" and item["review_group"] != group:
                continue
            result.append(item)
            accumulated += item["duration_seconds"]
            if len(result) >= limit or accumulated >= target_seconds:
                break
        return result

    def context(self, capture_id: str, radius: int = 2) -> list[dict[str, Any]]:
        """Return expandable neighboring speech from the same conversation."""
        radius = max(1, min(int(radius), 12))
        with self._connect() as connection:
            target = connection.execute(
                "SELECT conversation_id FROM chunks WHERE capture_id = ?", (capture_id,)
            ).fetchone()
            if not target:
                raise KeyError("unknown capture")
            if target["conversation_id"] is None:
                ids = [capture_id]
            else:
                ordered = connection.execute(
                    """
                    SELECT capture_id FROM chunks
                    WHERE conversation_id = ? AND kind = 'speech' AND audio_path IS NOT NULL
                    ORDER BY captured_at, capture_id
                    """,
                    (target["conversation_id"],),
                ).fetchall()
                all_ids = [row["capture_id"] for row in ordered]
                index = all_ids.index(capture_id)
                ids = all_ids[max(0, index - radius): index + radius + 1]
            placeholders = ",".join("?" for _ in ids)
            rows = connection.execute(
                f"""
                SELECT capture_id, captured_at, duration_seconds, speech_seconds,
                       audio_path, transcript, conversation_id
                FROM chunks WHERE capture_id IN ({placeholders})
                ORDER BY captured_at, capture_id
                """,
                ids,
            ).fetchall()
            annotations = self._active_annotations(connection, ids)
            proposals = self._latest_proposals(connection, ids)
            transcriptions = self._latest_transcriptions(connection, ids)
        return self._decorate_rows(rows, annotations, proposals, transcriptions)

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
        audio_start_seconds: float | None = None,
        audio_end_seconds: float | None = None,
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
                """
                SELECT c.duration_seconds,
                       COALESCE((
                           SELECT tp.transcript FROM transcription_passes tp
                           WHERE tp.capture_id = c.capture_id AND tp.is_current = 1
                           ORDER BY tp.created_at DESC, tp.pass_id DESC LIMIT 1
                       ), c.transcript) AS transcript
                FROM chunks c WHERE c.capture_id = ?
                """,
                (capture_id,),
            ).fetchone()
            if not chunk:
                raise KeyError("unknown capture")
            transcript = chunk["transcript"] or ""

            has_audio_start = audio_start_seconds is not None
            has_audio_end = audio_end_seconds is not None
            if has_audio_start != has_audio_end:
                raise ValueError("audio range requires both start and end times")
            if has_audio_start:
                audio_start_seconds = float(audio_start_seconds)
                audio_end_seconds = float(audio_end_seconds)
                if not isfinite(audio_start_seconds) or not isfinite(audio_end_seconds):
                    raise ValueError("audio range times must be finite")
                duration = max(0.0, float(chunk["duration_seconds"] or 30.0))
                audio_start_seconds = max(0.0, audio_start_seconds)
                audio_end_seconds = min(duration, audio_end_seconds)
                if audio_end_seconds <= audio_start_seconds:
                    raise ValueError("audio range end must be after its start")
                audio_start_seconds = round(audio_start_seconds, 3)
                audio_end_seconds = round(audio_end_seconds, 3)

            start_char = max(0, int(start_char))
            end_char = min(len(transcript), int(end_char))
            if end_char <= start_char:
                if has_audio_start:
                    start_char, end_char = 0, 0
                else:
                    start_char, end_char = 0, len(transcript)
            selected_text = transcript[start_char:end_char]
            annotation = {
                "annotation_id": uuid.uuid4().hex,
                "capture_id": capture_id,
                "start_char": start_char,
                "end_char": end_char,
                "selected_text": selected_text,
                "audio_start_seconds": audio_start_seconds,
                "audio_end_seconds": audio_end_seconds,
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
                    audio_start_seconds, audio_end_seconds, annotation_type, label,
                    replacement_text, source, created_at, reverted_at
                ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
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
                       MAX(COALESCE(c.duration_seconds, 30.0)) AS duration_seconds,
                       MAX(length(c.transcript)) AS transcript_chars,
                       SUM(CASE
                           WHEN a.audio_start_seconds IS NOT NULL
                            AND a.audio_end_seconds IS NOT NULL
                           THEN MAX(0.0, a.audio_end_seconds - a.audio_start_seconds)
                           ELSE 0.0
                       END) AS timed_seconds,
                       SUM(CASE
                           WHEN a.audio_start_seconds IS NULL
                             OR a.audio_end_seconds IS NULL
                           THEN MAX(1, a.end_char - a.start_char)
                           ELSE 0
                       END) AS labeled_chars
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
                estimated = float(row["speech_seconds"] or 0.0) * fraction
                timed = float(row["timed_seconds"] or 0.0)
                seconds[label] += min(
                    float(row["duration_seconds"] or 30.0), timed + estimated
                )
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
