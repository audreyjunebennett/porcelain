"""Durable, reversible review data for the Moto X memory journal.

Raw audio and machine transcripts remain in the existing ``chunks`` table.
This module stores model proposals and human corrections as additive layers.
"""

from __future__ import annotations

import copy
import math
import sqlite3
import struct
import threading
import uuid
from collections import Counter, defaultdict
from contextlib import contextmanager
from datetime import datetime, timedelta
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
IDENTITY_LABELS = ("Ruby", "Lynn", "Raven")
MIN_IDENTIFICATION_SECONDS = 1.5
MAX_IDENTIFICATION_SECONDS = 8.0


def _now() -> str:
    return datetime.now().astimezone().isoformat(timespec="seconds")


def _normalized(values: list[float]) -> list[float]:
    vector = [float(value) for value in values]
    if not vector or any(not isfinite(value) for value in vector):
        raise ValueError("speaker embedding must contain finite values")
    magnitude = math.sqrt(sum(value * value for value in vector))
    if magnitude <= 1e-12:
        raise ValueError("speaker embedding cannot be empty")
    return [value / magnitude for value in vector]


def _pack_embedding(values: list[float]) -> tuple[bytes, int]:
    vector = _normalized(values)
    return struct.pack(f"<{len(vector)}f", *vector), len(vector)


def _unpack_embedding(blob: bytes, dimension: int) -> list[float]:
    if dimension <= 0 or len(blob) != dimension * 4:
        raise ValueError("invalid stored speaker embedding")
    return list(struct.unpack(f"<{dimension}f", blob))


def _mean_embedding(vectors: list[list[float]]) -> list[float]:
    dimension = len(vectors[0])
    mean = [
        sum(vector[index] for vector in vectors) / len(vectors)
        for index in range(dimension)
    ]
    return _normalized(mean)


def _cosine(left: list[float], right: list[float]) -> float:
    return sum(a * b for a, b in zip(left, right))


def _compatible_embedding_version(value: str | None) -> str | None:
    """Ignore hardware build suffixes for the same torchaudio model release."""

    return str(value).split("+", 1)[0] if value else None


def _range_overlap(
    left_start: float, left_end: float, right_start: float, right_end: float
) -> float:
    return max(0.0, min(left_end, right_end) - max(left_start, right_start))


def _timed_speaker_segments(
    words: list[dict[str, Any]], predictions: list[dict[str, Any]]
) -> list[dict[str, Any]]:
    """Project timed words into conservative, non-overlapping chat turns."""

    segments: list[dict[str, Any]] = []
    for word in sorted(words, key=lambda item: int(item.get("word_index", 0))):
        start = float(word["start_seconds"])
        end = max(start, float(word["end_seconds"]))
        midpoint = (start + end) / 2
        direct = [
            prediction
            for prediction in predictions
            if _range_overlap(
                start,
                end,
                float(prediction["audio_start_seconds"]),
                float(prediction["audio_end_seconds"]),
            ) > 0
            or float(prediction["audio_start_seconds"]) <= midpoint <= float(prediction["audio_end_seconds"])
        ]
        labels = list(dict.fromkeys(item["speaker"] for item in direct))
        if len(labels) > 1:
            speaker = "Mixed voices"
            source = "predicted"
        elif labels:
            speaker = labels[0]
            source = next(item["source"] for item in direct if item["speaker"] == speaker)
        else:
            nearby = [
                prediction
                for prediction in predictions
                if float(prediction["audio_start_seconds"]) - 0.18
                <= midpoint
                <= float(prediction["audio_end_seconds"]) + 0.18
            ]
            nearby_labels = list(dict.fromkeys(item["speaker"] for item in nearby))
            speaker = nearby_labels[0] if len(nearby_labels) == 1 else "Unsorted"
            source = (
                next(item["source"] for item in nearby if item["speaker"] == speaker)
                if speaker != "Unsorted"
                else "unassigned"
            )

        text = str(word.get("word") or "")
        previous = segments[-1] if segments else None
        if (
            previous
            and previous["speaker"] == speaker
            and previous["source"] == source
            and start - float(previous["audio_end_seconds"]) <= 1.25
        ):
            previous["text"] += text
            previous["audio_end_seconds"] = end
        else:
            segments.append(
                {
                    "speaker": speaker,
                    "source": source,
                    "text": text,
                    "audio_start_seconds": start,
                    "audio_end_seconds": end,
                }
            )

    for segment in segments:
        segment["text"] = segment["text"].strip()
    return [segment for segment in segments if segment["text"]]


def _paint_speaker_regions(
    base_regions: list[dict[str, Any]],
    corrections: list[dict[str, Any]],
    duration_seconds: float,
) -> list[dict[str, Any]]:
    """Paint newer human speaker ranges over older/predicted regions."""

    duration = max(0.0, float(duration_seconds))
    regions: list[dict[str, Any]] = []

    def paint(region: dict[str, Any]) -> None:
        start = max(0.0, min(duration, float(region["audio_start_seconds"])))
        end = max(start, min(duration, float(region["audio_end_seconds"])))
        if end <= start:
            return
        replacement = dict(region)
        replacement["audio_start_seconds"] = start
        replacement["audio_end_seconds"] = end
        remaining: list[dict[str, Any]] = []
        for existing in regions:
            old_start = float(existing["audio_start_seconds"])
            old_end = float(existing["audio_end_seconds"])
            if old_end <= start or old_start >= end:
                remaining.append(existing)
                continue
            if old_start < start:
                left = dict(existing)
                left["audio_end_seconds"] = start
                remaining.append(left)
            if old_end > end:
                right = dict(existing)
                right["audio_start_seconds"] = end
                remaining.append(right)
        remaining.append(replacement)
        regions[:] = sorted(
            remaining,
            key=lambda item: (
                float(item["audio_start_seconds"]),
                float(item["audio_end_seconds"]),
            ),
        )

    for candidate in base_regions:
        paint(candidate)
    for correction in corrections:
        paint(correction)

    merged: list[dict[str, Any]] = []
    for region in regions:
        previous = merged[-1] if merged else None
        if (
            previous
            and previous.get("speaker") == region.get("speaker")
            and previous.get("source") == region.get("source")
            and abs(
                float(previous["audio_end_seconds"])
                - float(region["audio_start_seconds"])
            ) < 0.001
        ):
            previous["audio_end_seconds"] = region["audio_end_seconds"]
        else:
            merged.append(region)
    return merged


def _speaker_annotation_region(
    annotation: dict[str, Any],
    words: list[dict[str, Any]],
    transcript: str,
    duration_seconds: float,
) -> dict[str, Any] | None:
    """Resolve a human text/audio annotation to one paintable audio region."""

    start = annotation.get("audio_start_seconds")
    end = annotation.get("audio_end_seconds")
    if start is None or end is None:
        start_char = int(annotation.get("start_char") or 0)
        end_char = int(annotation.get("end_char") or 0)
        selected_words = [
            word
            for word in words
            if int(word.get("char_end") or 0) > start_char
            and int(word.get("char_start") or 0) < end_char
        ]
        if selected_words:
            start = float(selected_words[0]["start_seconds"])
            end = float(selected_words[-1]["end_seconds"])
        elif start_char == 0 and end_char >= len(transcript):
            start, end = 0.0, duration_seconds
        else:
            return None
    start = max(0.0, min(float(duration_seconds), float(start)))
    end = max(start, min(float(duration_seconds), float(end)))
    if end <= start:
        return None
    return {
        "speaker": annotation["label"],
        "audio_start_seconds": start,
        "audio_end_seconds": end,
        "confidence": 1.0,
        "source": "confirmed",
    }


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
                    speaker_turn_id TEXT,
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
            if "speaker_turn_id" not in annotation_columns:
                connection.execute(
                    "ALTER TABLE review_annotations ADD COLUMN speaker_turn_id TEXT"
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
            connection.execute(
                """
                CREATE TABLE IF NOT EXISTS review_speaker_turns (
                    turn_id TEXT PRIMARY KEY,
                    capture_id TEXT NOT NULL,
                    conversation_id TEXT,
                    audio_start_seconds REAL NOT NULL,
                    audio_end_seconds REAL NOT NULL,
                    cluster_id TEXT NOT NULL,
                    embedding BLOB NOT NULL,
                    embedding_dim INTEGER NOT NULL,
                    quality REAL NOT NULL DEFAULT 0,
                    embedding_model TEXT NOT NULL,
                    embedding_version TEXT,
                    diarization_model TEXT,
                    source_id TEXT NOT NULL,
                    created_at TEXT NOT NULL,
                    FOREIGN KEY (capture_id) REFERENCES chunks(capture_id)
                )
                """
            )
            connection.execute(
                """
                CREATE INDEX IF NOT EXISTS idx_review_speaker_turns_capture
                ON review_speaker_turns(capture_id, audio_start_seconds)
                """
            )
            connection.execute(
                """
                CREATE INDEX IF NOT EXISTS idx_review_speaker_turns_model
                ON review_speaker_turns(embedding_model, embedding_dim, quality)
                """
            )
            connection.execute(
                """
                CREATE TABLE IF NOT EXISTS review_speaker_examples (
                    annotation_id TEXT PRIMARY KEY,
                    embedding BLOB NOT NULL,
                    embedding_dim INTEGER NOT NULL,
                    embedding_model TEXT NOT NULL,
                    embedding_version TEXT,
                    created_at TEXT NOT NULL,
                    FOREIGN KEY (annotation_id)
                        REFERENCES review_annotations(annotation_id)
                )
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

    def add_speaker_turns(
        self,
        turns: list[dict[str, Any]],
        *,
        embedding_model: str,
        embedding_version: str | None,
        diarization_model: str | None,
        source_id: str,
    ) -> int:
        """Store source-mapped diarized turns and normalized voice embeddings.

        ``turn_id`` values are deterministic in the importer, so importing the
        same report again updates its derived rows instead of duplicating them.
        """

        embedding_model = str(embedding_model).strip()
        source_id = str(source_id).strip()
        if not embedding_model or not source_id:
            raise ValueError("speaker turn provenance is required")
        created_at = _now()
        rows = []
        for turn in turns:
            turn_id = str(turn.get("turn_id", "")).strip()
            capture_id = str(turn.get("capture_id", "")).strip()
            cluster_id = str(turn.get("cluster_id", "")).strip()
            start = float(turn.get("audio_start_seconds", 0.0))
            end = float(turn.get("audio_end_seconds", 0.0))
            quality = float(turn.get("quality", 0.0))
            if not turn_id or not capture_id or not cluster_id:
                continue
            if not all(isfinite(value) for value in (start, end, quality)):
                raise ValueError("speaker turn times and quality must be finite")
            if start < 0 or end <= start:
                raise ValueError("speaker turn range is invalid")
            blob, dimension = _pack_embedding(list(turn.get("embedding") or []))
            rows.append(
                (
                    turn_id,
                    capture_id,
                    turn.get("conversation_id"),
                    round(start, 3),
                    round(end, 3),
                    cluster_id,
                    blob,
                    dimension,
                    max(0.0, min(1.0, quality)),
                    embedding_model,
                    embedding_version,
                    diarization_model,
                    source_id,
                    created_at,
                )
            )
        if not rows:
            return 0
        with self._write_lock, self._connect() as connection:
            capture_ids = sorted({row[1] for row in rows})
            existing = set()
            for offset in range(0, len(capture_ids), 900):
                batch = capture_ids[offset:offset + 900]
                placeholders = ",".join("?" for _ in batch)
                existing.update(
                    row["capture_id"]
                    for row in connection.execute(
                        f"SELECT capture_id FROM chunks WHERE capture_id IN ({placeholders})",
                        batch,
                    )
                )
            rows = [row for row in rows if row[1] in existing]
            connection.executemany(
                """
                INSERT INTO review_speaker_turns (
                    turn_id, capture_id, conversation_id, audio_start_seconds,
                    audio_end_seconds, cluster_id, embedding, embedding_dim,
                    quality, embedding_model, embedding_version,
                    diarization_model, source_id, created_at
                ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
                ON CONFLICT(turn_id) DO UPDATE SET
                    capture_id = excluded.capture_id,
                    conversation_id = excluded.conversation_id,
                    audio_start_seconds = excluded.audio_start_seconds,
                    audio_end_seconds = excluded.audio_end_seconds,
                    cluster_id = excluded.cluster_id,
                    embedding = excluded.embedding,
                    embedding_dim = excluded.embedding_dim,
                    quality = excluded.quality,
                    embedding_model = excluded.embedding_model,
                    embedding_version = excluded.embedding_version,
                    diarization_model = excluded.diarization_model,
                    source_id = excluded.source_id,
                    created_at = excluded.created_at
                """,
                rows,
            )
        return len(rows)

    @staticmethod
    def _speaker_labels_for_turns(
        turns: list[dict[str, Any]], annotations: list[dict[str, Any]]
    ) -> dict[str, str]:
        labels: dict[str, str] = {}
        turns_by_capture: dict[str, list[dict[str, Any]]] = defaultdict(list)
        for turn in turns:
            turns_by_capture[turn["capture_id"]].append(turn)
        for annotation in annotations:
            if annotation["annotation_type"] != "speaker":
                continue
            label = annotation.get("label")
            if label not in SPEAKER_LABELS:
                continue
            direct_turn = annotation.get("speaker_turn_id")
            if direct_turn:
                labels[str(direct_turn)] = str(label)
                continue
            start = annotation.get("audio_start_seconds")
            end = annotation.get("audio_end_seconds")
            if start is None or end is None:
                continue
            start, end = float(start), float(end)
            for turn in turns_by_capture.get(annotation["capture_id"], []):
                duration = turn["audio_end_seconds"] - turn["audio_start_seconds"]
                overlap = _range_overlap(
                    turn["audio_start_seconds"],
                    turn["audio_end_seconds"],
                    start,
                    end,
                )
                if duration > 0 and overlap / duration >= 0.65:
                    labels[turn["turn_id"]] = str(label)
        return labels

    @staticmethod
    def _non_voice_labels_for_turns(
        turns: list[dict[str, Any]], annotations: list[dict[str, Any]]
    ) -> set[str]:
        """Return turns already reviewed as sound-only or overlapping audio."""

        handled: set[str] = set()
        turns_by_capture: dict[str, list[dict[str, Any]]] = defaultdict(list)
        for turn in turns:
            turns_by_capture[turn["capture_id"]].append(turn)
        for annotation in annotations:
            if annotation["annotation_type"] not in ("sound", "overlap"):
                continue
            start = annotation.get("audio_start_seconds")
            end = annotation.get("audio_end_seconds")
            if start is None or end is None:
                handled.update(
                    turn["turn_id"]
                    for turn in turns_by_capture.get(annotation["capture_id"], [])
                )
                continue
            start, end = float(start), float(end)
            for turn in turns_by_capture.get(annotation["capture_id"], []):
                duration = turn["audio_end_seconds"] - turn["audio_start_seconds"]
                overlap = _range_overlap(
                    turn["audio_start_seconds"],
                    turn["audio_end_seconds"],
                    start,
                    end,
                )
                if duration > 0 and overlap / duration >= 0.65:
                    handled.add(turn["turn_id"])
        return handled

    @staticmethod
    def _television_contaminated_sources(
        turns: list[dict[str, Any]], annotations: list[dict[str, Any]]
    ) -> set[str]:
        """Infer report-sized TV contamination from repeated human answers."""

        sources_by_capture: dict[str, set[str]] = defaultdict(set)
        for turn in turns:
            sources_by_capture[turn["capture_id"]].add(turn["source_id"])
        reviewed: dict[str, set[str]] = defaultdict(set)
        television: dict[str, set[str]] = defaultdict(set)
        for annotation in annotations:
            if annotation["annotation_type"] not in ("speaker", "sound", "overlap"):
                continue
            capture_id = annotation["capture_id"]
            for source_id in sources_by_capture.get(capture_id, ()):
                reviewed[source_id].add(capture_id)
                if (
                    annotation["annotation_type"] == "sound"
                    and annotation.get("label") == "Television"
                ):
                    television[source_id].add(capture_id)
        return {
            source_id
            for source_id, tv_captures in television.items()
            if len(tv_captures) >= 3
            and len(tv_captures) / max(1, len(reviewed[source_id])) >= 0.6
        }

    @classmethod
    def _identity_profiles(
        cls,
        turns: list[dict[str, Any]],
        annotations: list[dict[str, Any]],
        speaker_examples: dict[str, dict[str, Any]],
    ) -> dict[str, Any]:
        labels = cls._speaker_labels_for_turns(turns, annotations)
        handled_non_voice = cls._non_voice_labels_for_turns(turns, annotations)
        compatible_vectors: dict[
            tuple[str, str | None, int], dict[str, list[list[float]]]
        ] = defaultdict(lambda: defaultdict(list))
        cluster_votes: dict[str, Counter[str]] = defaultdict(Counter)
        global_examples: Counter[str] = Counter()
        direct_annotations = {
            str(annotation["speaker_turn_id"]): annotation
            for annotation in annotations
            if annotation["annotation_type"] == "speaker"
            and annotation.get("speaker_turn_id")
        }
        for turn in turns:
            label = labels.get(turn["turn_id"])
            if label:
                cluster_votes[turn["cluster_id"]][label] += 1
            if label not in IDENTITY_LABELS or turn["turn_id"] in handled_non_voice:
                continue
            annotation = direct_annotations.get(turn["turn_id"])
            example = speaker_examples.get(
                annotation["annotation_id"] if annotation else ""
            )
            if example:
                key = (
                    example["embedding_model"],
                    _compatible_embedding_version(example["embedding_version"]),
                    int(example["embedding_dim"]),
                )
                vector = example["vector"]
            else:
                adjusted = annotation and (
                    abs(float(annotation["audio_start_seconds"]) - turn["audio_start_seconds"]) > 0.01
                    or abs(float(annotation["audio_end_seconds"]) - turn["audio_end_seconds"]) > 0.01
                )
                if adjusted:
                    continue
                key = (
                    turn["embedding_model"],
                    _compatible_embedding_version(turn["embedding_version"]),
                    int(turn["embedding_dim"]),
                )
                vector = turn["vector"]
            compatible_vectors[key][label].append(vector)
            global_examples[label] += 1
        centroids = {
            key: {
                label: _mean_embedding(vectors)
                for label, vectors in by_label.items()
            }
            for key, by_label in compatible_vectors.items()
        }
        return {
            "labels": labels,
            "handled_non_voice": handled_non_voice,
            "cluster_votes": cluster_votes,
            "global_examples": global_examples,
            "centroids": centroids,
        }

    def identification_candidates(
        self,
        *,
        limit: int = 24,
        exclude_turn_ids: set[str] | None = None,
        exclude_capture_ids: set[str] | None = None,
    ) -> list[dict[str, Any]]:
        """Rank exact diarized turns by expected speaker-learning value.

        Human answers build per-person embedding centroids. The queue then
        favors clean turns near the boundary between the two closest verified
        household voices, while spreading bootstrap questions across clusters.
        """

        limit = max(1, min(int(limit), 100))
        excluded = set(exclude_turn_ids or ())
        excluded_captures = set(exclude_capture_ids or ())
        with self._connect() as connection:
            rows = connection.execute(
                """
                SELECT t.*, c.captured_at, c.duration_seconds,
                       c.speech_seconds, c.audio_path, c.transcript,
                       c.conversation_id AS chunk_conversation_id
                FROM review_speaker_turns t
                JOIN chunks c ON c.capture_id = t.capture_id
                WHERE c.kind = 'speech' AND c.audio_path IS NOT NULL
                  AND trim(c.transcript) <> ''
                  AND (t.audio_end_seconds - t.audio_start_seconds) BETWEEN ? AND ?
                  AND NOT EXISTS (
                      SELECT 1 FROM review_annotations privacy
                      WHERE privacy.capture_id = c.capture_id
                        AND privacy.annotation_type = 'privacy'
                        AND privacy.label = 'Exclude'
                        AND privacy.reverted_at IS NULL
                  )
                ORDER BY t.created_at DESC, t.turn_id
                """,
                (MIN_IDENTIFICATION_SECONDS, MAX_IDENTIFICATION_SECONDS),
            ).fetchall()
            all_turns = []
            for row in rows:
                item = dict(row)
                if not Path(item["audio_path"]).is_file():
                    continue
                try:
                    item["vector"] = _unpack_embedding(
                        item.pop("embedding"), int(item["embedding_dim"])
                    )
                except ValueError:
                    continue
                all_turns.append(item)
            turns = [
                turn for turn in all_turns
                if turn["turn_id"] not in excluded
                and turn["capture_id"] not in excluded_captures
            ]
            annotations = [
                dict(row)
                for row in connection.execute(
                    """
                    SELECT * FROM review_annotations
                    WHERE reverted_at IS NULL
                    ORDER BY created_at, rowid
                    """
                ).fetchall()
            ]
            speaker_examples = {}
            for row in connection.execute(
                """
                SELECT example.* FROM review_speaker_examples example
                JOIN review_annotations annotation
                  ON annotation.annotation_id = example.annotation_id
                WHERE annotation.reverted_at IS NULL
                """
            ).fetchall():
                example = dict(row)
                try:
                    example["vector"] = _unpack_embedding(
                        example.pop("embedding"), int(example["embedding_dim"])
                    )
                except ValueError:
                    continue
                speaker_examples[example["annotation_id"]] = example
            profiles = self._identity_profiles(
                all_turns, annotations, speaker_examples
            )
            labels = profiles["labels"]
            handled_non_voice = profiles["handled_non_voice"]
            cluster_votes = profiles["cluster_votes"]
            global_examples = profiles["global_examples"]
            centroids = profiles["centroids"]
            contaminated_sources = self._television_contaminated_sources(
                all_turns, annotations
            )

            ranked = []
            for turn in turns:
                if (
                    turn["turn_id"] in labels
                    or turn["turn_id"] in handled_non_voice
                    or turn["source_id"] in contaminated_sources
                ):
                    continue
                key = (
                    turn["embedding_model"],
                    _compatible_embedding_version(turn["embedding_version"]),
                    int(turn["embedding_dim"]),
                )
                profile = centroids.get(key, {})
                similarities = {
                    label: _cosine(turn["vector"], centroid)
                    for label, centroid in profile.items()
                }
                votes = cluster_votes.get(turn["cluster_id"], Counter())
                if votes:
                    voted_label, voted_count = votes.most_common(1)[0]
                    if voted_label in similarities:
                        purity = voted_count / sum(votes.values())
                        similarities[voted_label] += 0.08 * purity
                scored_labels = sorted(
                    similarities, key=lambda label: similarities[label], reverse=True
                )
                if len(scored_labels) >= 2:
                    alternatives = scored_labels[:2]
                    margin = similarities[alternatives[0]] - similarities[alternatives[1]]
                    uncertainty = 1.0 - min(1.0, abs(margin) * 5.0)
                    learning_stage = "compare"
                    reason = "Closest verified voices; this answer sharpens their boundary."
                elif len(scored_labels) == 1:
                    remaining = sorted(
                        (label for label in IDENTITY_LABELS if label != scored_labels[0]),
                        key=lambda label: (global_examples[label], IDENTITY_LABELS.index(label)),
                    )
                    alternatives = [scored_labels[0], remaining[0]]
                    uncertainty = 0.85
                    learning_stage = "expand"
                    reason = f"Checking against {scored_labels[0]}'s verified examples."
                else:
                    alternatives = sorted(
                        IDENTITY_LABELS,
                        key=lambda label: (global_examples[label], IDENTITY_LABELS.index(label)),
                    )[:2]
                    uncertainty = 1.0
                    learning_stage = "bootstrap"
                    reason = "Building the first verified voice references."
                cluster_purity = 0.0
                if votes:
                    cluster_purity = votes.most_common(1)[0][1] / sum(votes.values())
                quality = max(0.0, min(1.0, float(turn["quality"])))
                priority = uncertainty * 0.68 + quality * 0.32 - cluster_purity * 0.18
                turn["identification"] = {
                    "turn_id": turn["turn_id"],
                    "audio_start_seconds": turn["audio_start_seconds"],
                    "audio_end_seconds": turn["audio_end_seconds"],
                    "duration_seconds": round(
                        turn["audio_end_seconds"] - turn["audio_start_seconds"], 3
                    ),
                    "alternatives": alternatives,
                    "prompt": f"{alternatives[0]} or {alternatives[1]}?",
                    "reason": reason,
                    "learning_stage": learning_stage,
                    "profile_examples": {
                        label: global_examples[label] for label in IDENTITY_LABELS
                    },
                    "cluster_id": turn["cluster_id"],
                }
                turn["priority"] = priority
                ranked.append(turn)

            ranked.sort(
                key=lambda turn: (
                    turn["priority"],
                    turn["quality"],
                    turn["captured_at"],
                    turn["turn_id"],
                ),
                reverse=True,
            )
            # Bootstrap across distinct diarization clusters before repeating
            # one cluster. This avoids replacing "latest clips" with another
            # repetitive queue.
            diverse = []
            repeated = []
            seen_clusters = set()
            for turn in ranked:
                if turn["cluster_id"] in seen_clusters:
                    repeated.append(turn)
                else:
                    diverse.append(turn)
                    seen_clusters.add(turn["cluster_id"])
            # One question per 30-second source capture keeps a noisy room or
            # television segment from dominating a single review batch.
            selected = []
            seen_captures = set()
            for turn in diverse + repeated:
                if turn["capture_id"] in seen_captures:
                    continue
                selected.append(turn)
                seen_captures.add(turn["capture_id"])
                if len(selected) >= limit:
                    break
            capture_ids = list(dict.fromkeys(turn["capture_id"] for turn in selected))
            if not capture_ids:
                return []
            placeholders = ",".join("?" for _ in capture_ids)
            chunk_rows = connection.execute(
                f"""
                SELECT capture_id, captured_at, duration_seconds, speech_seconds,
                       audio_path, transcript, conversation_id
                FROM chunks WHERE capture_id IN ({placeholders})
                """,
                capture_ids,
            ).fetchall()
            active = self._active_annotations(connection, capture_ids)
            proposals = self._latest_proposals(connection, capture_ids)
            transcriptions = self._latest_transcriptions(connection, capture_ids)

        decorated = self._decorate_rows(chunk_rows, active, proposals, transcriptions)
        by_capture = {item["capture_id"]: item for item in decorated}
        result = []
        for turn in selected:
            base = by_capture.get(turn["capture_id"])
            if not base:
                continue
            item = copy.deepcopy(base)
            question = turn["identification"]
            selected_words = [
                word
                for word in item.get("words", [])
                if float(word["end_seconds"]) > question["audio_start_seconds"]
                and float(word["start_seconds"]) < question["audio_end_seconds"]
            ]
            if selected_words:
                question["start_char"] = int(selected_words[0]["char_start"])
                question["end_char"] = int(selected_words[-1]["char_end"])
                question["selected_text"] = item["transcript"][
                    question["start_char"]:question["end_char"]
                ]
            else:
                question["start_char"] = 0
                question["end_char"] = 0
                question["selected_text"] = ""
            try:
                started = datetime.strptime(item["captured_at"], "%Y-%m-%d_%H-%M-%S")
                question["occurred_at"] = (
                    started + timedelta(seconds=question["audio_start_seconds"])
                ).isoformat(timespec="milliseconds")
            except ValueError:
                question["occurred_at"] = item["captured_at"]
            item["identification"] = question
            result.append(item)
        return result

    def journal_speaker_predictions(
        self, day: str | list[str] | tuple[str, ...]
    ) -> dict[str, dict[str, Any]]:
        """Return conservative derived speaker layers for prepared journal audio."""

        days = {day} if isinstance(day, str) else set(day)
        for value in days:
            datetime.strptime(value, "%Y-%m-%d")
        with self._connect() as connection:
            rows = connection.execute(
                """
                SELECT t.*, c.captured_at, c.audio_path, c.duration_seconds
                FROM review_speaker_turns t
                JOIN chunks c ON c.capture_id = t.capture_id
                WHERE c.kind = 'speech' AND c.audio_path IS NOT NULL
                  AND NOT EXISTS (
                      SELECT 1 FROM review_annotations privacy
                      WHERE privacy.capture_id = c.capture_id
                        AND privacy.annotation_type = 'privacy'
                        AND privacy.label = 'Exclude'
                        AND privacy.reverted_at IS NULL
                  )
                ORDER BY t.created_at, t.turn_id
                """
            ).fetchall()
            turns = []
            for row in rows:
                turn = dict(row)
                try:
                    turn["vector"] = _unpack_embedding(
                        turn.pop("embedding"), int(turn["embedding_dim"])
                    )
                except ValueError:
                    continue
                turns.append(turn)
            annotations = [
                dict(row)
                for row in connection.execute(
                    """
                    SELECT * FROM review_annotations
                    WHERE reverted_at IS NULL
                    ORDER BY created_at, rowid
                    """
                ).fetchall()
            ]
            speaker_examples = {}
            for row in connection.execute(
                """
                SELECT example.* FROM review_speaker_examples example
                JOIN review_annotations annotation
                  ON annotation.annotation_id = example.annotation_id
                WHERE annotation.reverted_at IS NULL
                """
            ).fetchall():
                example = dict(row)
                try:
                    example["vector"] = _unpack_embedding(
                        example.pop("embedding"), int(example["embedding_dim"])
                    )
                except ValueError:
                    continue
                speaker_examples[example["annotation_id"]] = example
        profiles = self._identity_profiles(turns, annotations, speaker_examples)
        turns_by_capture: dict[str, list[dict[str, Any]]] = defaultdict(list)
        for turn in turns:
            turns_by_capture[turn["capture_id"]].append(turn)
        contaminated = self._television_contaminated_sources(turns, annotations)
        by_capture: dict[str, list[dict[str, Any]]] = defaultdict(list)
        for turn in turns:
            if str(turn["captured_at"])[:10] not in days:
                continue
            label = profiles["labels"].get(turn["turn_id"])
            human_labeled = label in SPEAKER_LABELS
            if turn["turn_id"] in profiles["handled_non_voice"]:
                continue
            if human_labeled or turn["source_id"] in contaminated:
                continue
            key = (
                turn["embedding_model"],
                _compatible_embedding_version(turn["embedding_version"]),
                int(turn["embedding_dim"]),
            )
            similarities = {
                identity: _cosine(turn["vector"], centroid)
                for identity, centroid in profiles["centroids"].get(key, {}).items()
                if profiles["global_examples"][identity] >= 2
            }
            ordered = sorted(similarities, key=similarities.get, reverse=True)
            if not ordered:
                continue
            label = ordered[0]
            best = similarities[label]
            second = similarities[ordered[1]] if len(ordered) > 1 else 0.0
            margin = best - second
            votes = profiles["cluster_votes"].get(turn["cluster_id"], Counter())
            if votes:
                voted_label, voted_count = votes.most_common(1)[0]
                purity = voted_count / sum(votes.values())
                if (
                    voted_label in IDENTITY_LABELS
                    and voted_count >= 2
                    and purity >= 0.8
                ):
                    label = voted_label
                    margin = max(margin, 0.08)
            if best < 0.72 or margin < 0.04:
                continue
            confidence = min(0.99, max(0.5, (best - 0.5) * 1.8 + margin))
            by_capture[turn["capture_id"]].append(
                {
                    "speaker": label,
                    "audio_start_seconds": turn["audio_start_seconds"],
                    "audio_end_seconds": turn["audio_end_seconds"],
                    "confidence": round(confidence, 3),
                    "source": "predicted",
                }
            )

        speaker_annotations: dict[str, list[dict[str, Any]]] = defaultdict(list)
        for annotation in annotations:
            if (
                annotation["annotation_type"] == "speaker"
                and annotation.get("label") in SPEAKER_LABELS
            ):
                speaker_annotations[annotation["capture_id"]].append(annotation)

        with self._connect() as connection:
            day_placeholders = ",".join("?" for _ in days)
            annotated_rows = connection.execute(
                f"""
                SELECT DISTINCT c.capture_id, c.duration_seconds
                FROM chunks c
                JOIN review_annotations annotation
                  ON annotation.capture_id = c.capture_id
                WHERE substr(c.captured_at, 1, 10) IN ({day_placeholders})
                  AND c.kind = 'speech'
                  AND annotation.annotation_type = 'speaker'
                  AND annotation.reverted_at IS NULL
                  AND NOT EXISTS (
                      SELECT 1 FROM review_annotations privacy
                      WHERE privacy.capture_id = c.capture_id
                        AND privacy.annotation_type = 'privacy'
                        AND privacy.label = 'Exclude'
                        AND privacy.reverted_at IS NULL
                  )
                """,
                tuple(sorted(days)),
            ).fetchall()
            annotated_durations = {
                row["capture_id"]: float(row["duration_seconds"] or 30.0)
                for row in annotated_rows
            }
            capture_ids = set(by_capture) | set(annotated_durations)
            transcriptions = self._latest_transcriptions(
                connection, list(capture_ids)
            )

        result = {}
        for capture_id in capture_ids:
            transcription = transcriptions.get(capture_id, {})
            duration = annotated_durations.get(capture_id)
            if duration is None:
                duration = max(
                    float(turn.get("duration_seconds") or 30.0)
                    for turn in turns_by_capture[capture_id]
                )
            corrections = []
            for annotation in speaker_annotations.get(capture_id, []):
                region = _speaker_annotation_region(
                    annotation,
                    transcription.get("words", []),
                    str(transcription.get("transcript") or ""),
                    duration,
                )
                if region:
                    corrections.append(region)
            predictions = _paint_speaker_regions(
                by_capture.get(capture_id, []), corrections, duration
            )
            if not predictions:
                continue
            speakers = list(dict.fromkeys(row["speaker"] for row in predictions))
            result[capture_id] = {
                "capture_id": capture_id,
                "duration_seconds": duration,
                "speakers": speakers,
                "turns": predictions,
                "segments": _timed_speaker_segments(
                    transcription.get("words", []), predictions
                ),
                "source": (
                    "confirmed"
                    if all(row["source"] == "confirmed" for row in predictions)
                    else "predicted"
                ),
            }
        return result

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
        speaker_turn_id: str | None = None,
        speaker_embedding: list[float] | None = None,
        speaker_embedding_model: str | None = None,
        speaker_embedding_version: str | None = None,
    ) -> dict[str, Any]:
        annotation_type = str(annotation_type).strip().lower()
        if annotation_type not in ANNOTATION_TYPES:
            raise ValueError("unsupported annotation type")
        if annotation_type != "transcript" and label not in ALL_LABELS:
            raise ValueError("unsupported label")
        if annotation_type == "transcript" and not (replacement_text or "").strip():
            raise ValueError("replacement text is required")
        packed_speaker_embedding = None
        if speaker_embedding is not None:
            if annotation_type != "speaker" or not speaker_turn_id:
                raise ValueError("speaker embedding requires a speaker turn")
            speaker_embedding_model = str(speaker_embedding_model or "").strip()
            if not speaker_embedding_model:
                raise ValueError("speaker embedding model is required")
            packed_speaker_embedding = _pack_embedding(speaker_embedding)

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

            speaker_turn_id = str(speaker_turn_id or "").strip() or None
            if speaker_turn_id:
                if annotation_type != "speaker":
                    raise ValueError("speaker turn can only receive a speaker label")
                turn = connection.execute(
                    """
                    SELECT capture_id, audio_start_seconds, audio_end_seconds
                    FROM review_speaker_turns WHERE turn_id = ?
                    """,
                    (speaker_turn_id,),
                ).fetchone()
                if not turn or turn["capture_id"] != capture_id:
                    raise ValueError("speaker turn does not match the capture")
                # The diarized turn is the proposed crop. A human trim may
                # refine it while the deterministic turn ID keeps provenance.
                if audio_start_seconds is None and audio_end_seconds is None:
                    audio_start_seconds = float(turn["audio_start_seconds"])
                    audio_end_seconds = float(turn["audio_end_seconds"])

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
                if speaker_turn_id:
                    overlap = _range_overlap(
                        audio_start_seconds,
                        audio_end_seconds,
                        float(turn["audio_start_seconds"]),
                        float(turn["audio_end_seconds"]),
                    )
                    if overlap < 0.25:
                        if packed_speaker_embedding:
                            raise ValueError("trimmed speaker range must overlap the proposed turn")
                        audio_start_seconds = float(turn["audio_start_seconds"])
                        audio_end_seconds = float(turn["audio_end_seconds"])
                    if audio_end_seconds - audio_start_seconds < 0.5:
                        raise ValueError("trimmed speaker range must be at least 0.5 seconds")
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
                "speaker_turn_id": speaker_turn_id,
                "source": "human",
                "created_at": _now(),
                "reverted_at": None,
            }
            existing = connection.execute(
                """
                SELECT * FROM review_annotations
                WHERE reverted_at IS NULL
                  AND capture_id = ?
                  AND start_char = ?
                  AND end_char = ?
                  AND audio_start_seconds IS ?
                  AND audio_end_seconds IS ?
                  AND annotation_type = ?
                  AND label IS ?
                  AND replacement_text IS ?
                  AND speaker_turn_id IS ?
                ORDER BY created_at, annotation_id
                LIMIT 1
                """,
                (
                    capture_id,
                    start_char,
                    end_char,
                    audio_start_seconds,
                    audio_end_seconds,
                    annotation_type,
                    label,
                    annotation["replacement_text"],
                    speaker_turn_id,
                ),
            ).fetchone()
            if existing:
                if packed_speaker_embedding:
                    connection.execute(
                        """
                        INSERT INTO review_speaker_examples (
                            annotation_id, embedding, embedding_dim,
                            embedding_model, embedding_version, created_at
                        ) VALUES (?, ?, ?, ?, ?, ?)
                        ON CONFLICT(annotation_id) DO UPDATE SET
                            embedding = excluded.embedding,
                            embedding_dim = excluded.embedding_dim,
                            embedding_model = excluded.embedding_model,
                            embedding_version = excluded.embedding_version,
                            created_at = excluded.created_at
                        """,
                        (
                            existing["annotation_id"],
                            packed_speaker_embedding[0],
                            packed_speaker_embedding[1],
                            speaker_embedding_model,
                            speaker_embedding_version,
                            _now(),
                        ),
                    )
                return dict(existing)
            connection.execute(
                """
                INSERT INTO review_annotations (
                    annotation_id, capture_id, start_char, end_char, selected_text,
                    audio_start_seconds, audio_end_seconds, annotation_type, label,
                    replacement_text, speaker_turn_id, source, created_at, reverted_at
                ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
                """,
                tuple(annotation.values()),
            )
            if packed_speaker_embedding:
                connection.execute(
                    """
                    INSERT INTO review_speaker_examples (
                        annotation_id, embedding, embedding_dim,
                        embedding_model, embedding_version, created_at
                    ) VALUES (?, ?, ?, ?, ?, ?)
                    """,
                    (
                        annotation["annotation_id"],
                        packed_speaker_embedding[0],
                        packed_speaker_embedding[1],
                        speaker_embedding_model,
                        speaker_embedding_version,
                        _now(),
                    ),
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

    def set_quick_speaker_label(
        self,
        *,
        capture_id: str,
        audio_start_seconds: float,
        audio_end_seconds: float,
        label: str | None,
    ) -> dict[str, Any]:
        """Replace the speaker correction for one dashboard audio region.

        ``None`` means Unsorted: overlapping human speaker corrections are
        reverted and no replacement is added. Other annotation kinds remain
        untouched.
        """

        if label not in (None, "Ruby", "Lynn"):
            raise ValueError("quick speaker label must be Unsorted, Ruby, or Lynn")
        start = float(audio_start_seconds)
        end = float(audio_end_seconds)
        if not isfinite(start) or not isfinite(end):
            raise ValueError("audio range times must be finite")

        with self._write_lock, self._connect() as connection:
            chunk = connection.execute(
                """
                SELECT duration_seconds,
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
            duration = max(0.0, float(chunk["duration_seconds"] or 30.0))
            start = round(max(0.0, start), 3)
            end = round(min(duration, end), 3)
            if end <= start:
                raise ValueError("audio range end must be after its start")

            active = connection.execute(
                """
                SELECT * FROM review_annotations
                WHERE capture_id = ? AND annotation_type = 'speaker'
                  AND reverted_at IS NULL
                ORDER BY created_at, annotation_id
                """,
                (capture_id,),
            ).fetchall()
            conflicting_ids = []
            for annotation in active:
                old_start = annotation["audio_start_seconds"]
                old_end = annotation["audio_end_seconds"]
                if old_start is None or old_end is None:
                    old_start, old_end = 0.0, duration
                old_start, old_end = float(old_start), float(old_end)
                shorter = min(end - start, old_end - old_start)
                overlap = _range_overlap(start, end, old_start, old_end)
                if shorter > 0 and overlap / shorter >= 0.65:
                    conflicting_ids.append(annotation["annotation_id"])

            changed_at = _now()
            if conflicting_ids:
                connection.executemany(
                    """
                    UPDATE review_annotations SET reverted_at = ?
                    WHERE annotation_id = ? AND reverted_at IS NULL
                    """,
                    [(changed_at, annotation_id) for annotation_id in conflicting_ids],
                )

            annotation = None
            if label is not None:
                matching_turn = connection.execute(
                    """
                    SELECT turn_id, audio_start_seconds, audio_end_seconds
                    FROM review_speaker_turns
                    WHERE capture_id = ?
                    ORDER BY audio_start_seconds, turn_id
                    """,
                    (capture_id,),
                ).fetchall()
                best_turn_id = None
                best_coverage = 0.0
                for turn in matching_turn:
                    turn_start = float(turn["audio_start_seconds"])
                    turn_end = float(turn["audio_end_seconds"])
                    turn_duration = turn_end - turn_start
                    overlap = _range_overlap(start, end, turn_start, turn_end)
                    coverage = overlap / turn_duration if turn_duration > 0 else 0.0
                    target_coverage = overlap / (end - start)
                    if target_coverage < 0.65:
                        coverage = 0.0
                    if coverage > best_coverage:
                        best_turn_id = turn["turn_id"]
                        best_coverage = coverage
                if best_coverage < 0.65:
                    best_turn_id = None

                transcript = str(chunk["transcript"] or "")
                annotation = {
                    "annotation_id": uuid.uuid4().hex,
                    "capture_id": capture_id,
                    "start_char": 0,
                    "end_char": 0,
                    "selected_text": "",
                    "audio_start_seconds": start,
                    "audio_end_seconds": end,
                    "annotation_type": "speaker",
                    "label": label,
                    "replacement_text": None,
                    "speaker_turn_id": best_turn_id,
                    "source": "human",
                    "created_at": changed_at,
                    "reverted_at": None,
                }
                connection.execute(
                    """
                    INSERT INTO review_annotations (
                        annotation_id, capture_id, start_char, end_char, selected_text,
                        audio_start_seconds, audio_end_seconds, annotation_type, label,
                        replacement_text, speaker_turn_id, source, created_at, reverted_at
                    ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
                    """,
                    tuple(annotation.values()),
                )

        return {
            "capture_id": capture_id,
            "audio_start_seconds": start,
            "audio_end_seconds": end,
            "label": label or "Unsorted",
            "reverted_count": len(conflicting_ids),
            "annotation": annotation,
        }

    def active_speaker_annotations(
        self, capture_ids: list[str]
    ) -> dict[str, list[dict[str, Any]]]:
        """Return active human speaker ranges for lightweight dashboard projection."""

        capture_ids = list(dict.fromkeys(str(value) for value in capture_ids if value))
        if not capture_ids:
            return {}
        placeholders = ",".join("?" for _ in capture_ids)
        with self._connect() as connection:
            rows = connection.execute(
                f"""
                SELECT * FROM review_annotations
                WHERE capture_id IN ({placeholders})
                  AND annotation_type = 'speaker'
                  AND label IN ('Ruby', 'Lynn')
                  AND reverted_at IS NULL
                ORDER BY created_at, annotation_id
                """,
                capture_ids,
            ).fetchall()
        result: dict[str, list[dict[str, Any]]] = defaultdict(list)
        for row in rows:
            result[row["capture_id"]].append(dict(row))
        return dict(result)

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
