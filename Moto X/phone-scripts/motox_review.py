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


def _range_overlap(
    left_start: float, left_end: float, right_start: float, right_end: float
) -> float:
    return max(0.0, min(left_end, right_end) - max(left_start, right_start))


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

    def identification_candidates(
        self, *, limit: int = 24, exclude_turn_ids: set[str] | None = None
    ) -> list[dict[str, Any]]:
        """Rank exact diarized turns by expected speaker-learning value.

        Human answers build per-person embedding centroids. The queue then
        favors clean turns near the boundary between the two closest verified
        household voices, while spreading bootstrap questions across clusters.
        """

        limit = max(1, min(int(limit), 100))
        excluded = set(exclude_turn_ids or ())
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
            turns = []
            for row in rows:
                item = dict(row)
                if item["turn_id"] in excluded or not Path(item["audio_path"]).is_file():
                    continue
                try:
                    item["vector"] = _unpack_embedding(
                        item.pop("embedding"), int(item["embedding_dim"])
                    )
                except ValueError:
                    continue
                turns.append(item)
            annotations = [
                dict(row)
                for row in connection.execute(
                    """
                    SELECT * FROM review_annotations
                    WHERE reverted_at IS NULL AND annotation_type = 'speaker'
                    ORDER BY created_at, annotation_id
                    """
                ).fetchall()
            ]

            labels = self._speaker_labels_for_turns(turns, annotations)
            compatible_vectors: dict[
                tuple[str, str | None, int], dict[str, list[list[float]]]
            ] = defaultdict(lambda: defaultdict(list))
            cluster_votes: dict[str, Counter[str]] = defaultdict(Counter)
            global_examples: Counter[str] = Counter()
            for turn in turns:
                label = labels.get(turn["turn_id"])
                if label:
                    cluster_votes[turn["cluster_id"]][label] += 1
                if label not in IDENTITY_LABELS:
                    continue
                key = (
                    turn["embedding_model"],
                    turn["embedding_version"],
                    int(turn["embedding_dim"]),
                )
                compatible_vectors[key][label].append(turn["vector"])
                global_examples[label] += 1

            centroids = {
                key: {
                    label: _mean_embedding(vectors)
                    for label, vectors in by_label.items()
                }
                for key, by_label in compatible_vectors.items()
            }

            ranked = []
            for turn in turns:
                if turn["turn_id"] in labels:
                    continue
                key = (
                    turn["embedding_model"],
                    turn["embedding_version"],
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
            selected = (diverse + repeated)[:limit]
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
                # The server, rather than the browser, owns the exact region
                # used as a training example.
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
