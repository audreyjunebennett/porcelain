"""Automatically prepare recent Moto X conversations for speaker identification.

The daemon is intentionally separate from the live receiver. It processes closed
conversations first and may checkpoint a long open conversation only after a
quiet speech gap. Completed source captures are recorded in SQLite, making every
restart and retry idempotent.
"""

from __future__ import annotations

import argparse
import hashlib
import os
import sqlite3
import subprocess
import sys
import time
import traceback
from collections import defaultdict
from contextlib import contextmanager
from dataclasses import dataclass
from datetime import datetime, timedelta
from pathlib import Path


SCRIPTS_DIR = Path(__file__).resolve().parent
BASE_DIR = SCRIPTS_DIR.parent / "motox_audio_data"
DEFAULT_DATABASE = Path(os.environ.get("MOTOX_V1_DATABASE", BASE_DIR / "motox_v1.sqlite3"))
DEFAULT_OUTPUT_DIR = SCRIPTS_DIR.parent / "diarization_data" / "derived" / "diarization"
DEFAULT_DIARIZATION_PYTHON = Path(
    os.environ.get(
        "MOTOX_DIARIZATION_PYTHON",
        SCRIPTS_DIR.parents[1] / ".venv-motox-diarization" / "Scripts" / "python.exe",
    )
)
TIMESTAMP_FORMAT = "%Y-%m-%d_%H-%M-%S"


@dataclass(frozen=True)
class PipelineBatch:
    conversation_id: str
    capture_ids: tuple[str, ...]
    kind: str
    speech_seconds: float

    @property
    def job_id(self) -> str:
        payload = "|".join((self.conversation_id, *self.capture_ids))
        return hashlib.sha256(payload.encode("utf-8")).hexdigest()[:24]


def connect(database: Path) -> sqlite3.Connection:
    connection = sqlite3.connect(database, timeout=30)
    connection.row_factory = sqlite3.Row
    return connection


@contextmanager
def database_connection(database: Path):
    connection = connect(database)
    try:
        yield connection
        connection.commit()
    finally:
        connection.close()


def ensure_schema(connection: sqlite3.Connection) -> None:
    connection.executescript(
        """
        CREATE TABLE IF NOT EXISTS identification_pipeline_runs (
            job_id TEXT PRIMARY KEY,
            conversation_id TEXT NOT NULL,
            kind TEXT NOT NULL,
            capture_count INTEGER NOT NULL,
            status TEXT NOT NULL,
            report_path TEXT,
            attempts INTEGER NOT NULL DEFAULT 0,
            error TEXT,
            created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
            started_at TEXT,
            finished_at TEXT
        );
        CREATE TABLE IF NOT EXISTS identification_pipeline_captures (
            capture_id TEXT PRIMARY KEY,
            conversation_id TEXT NOT NULL,
            job_id TEXT NOT NULL REFERENCES identification_pipeline_runs(job_id),
            processed_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
        );
        """
    )


def choose_batch(
    database: Path,
    *,
    now: datetime | None = None,
    lookback_hours: float = 30.0,
    checkpoint_seconds: float = 600.0,
    quiet_seconds: float = 45.0,
    target_seconds: float = 300.0,
    minimum_speech_seconds: float = 8.0,
) -> PipelineBatch | None:
    now = now or datetime.now()
    cutoff = (now - timedelta(hours=max(1.0, lookback_hours))).strftime(TIMESTAMP_FORMAT)
    with database_connection(database) as connection:
        ensure_schema(connection)
        rows = connection.execute(
            """
            SELECT chunk.capture_id, chunk.captured_at, chunk.duration_seconds,
                   chunk.speech_seconds, chunk.conversation_id,
                   conversation.status, conversation.last_speech_at
            FROM chunks chunk
            JOIN conversations conversation
              ON conversation.conversation_id = chunk.conversation_id
            WHERE chunk.kind = 'speech' AND chunk.audio_path IS NOT NULL
              AND trim(chunk.transcript) <> '' AND chunk.captured_at >= ?
              AND NOT EXISTS (
                  SELECT 1 FROM identification_pipeline_captures covered
                  WHERE covered.capture_id = chunk.capture_id
              )
              AND NOT EXISTS (
                  SELECT 1 FROM review_annotations privacy
                  WHERE privacy.capture_id = chunk.capture_id
                    AND privacy.annotation_type = 'privacy'
                    AND privacy.label = 'Exclude'
                    AND privacy.reverted_at IS NULL
              )
            ORDER BY chunk.captured_at, chunk.capture_id
            """,
            (cutoff,),
        ).fetchall()

    grouped: dict[str, list[sqlite3.Row]] = defaultdict(list)
    for row in rows:
        grouped[str(row["conversation_id"])].append(row)
    eligible: list[tuple[int, str, PipelineBatch]] = []
    for conversation_id, captures in grouped.items():
        status = str(captures[0]["status"])
        speech = sum(float(row["speech_seconds"] or 0.0) for row in captures)
        duration = sum(float(row["duration_seconds"] or 30.0) for row in captures)
        if status == "closed":
            kind = "final"
        else:
            last_speech = datetime.strptime(str(captures[0]["last_speech_at"]), TIMESTAMP_FORMAT)
            if duration < checkpoint_seconds or (now - last_speech).total_seconds() < quiet_seconds:
                continue
            kind = "checkpoint"
        if speech < minimum_speech_seconds:
            continue

        selected: list[sqlite3.Row] = []
        selected_duration = 0.0
        for row in reversed(captures):
            duration_seconds = float(row["duration_seconds"] or 30.0)
            if selected and selected_duration + duration_seconds > target_seconds:
                break
            selected.append(row)
            selected_duration += duration_seconds
        selected.reverse()
        batch = PipelineBatch(
            conversation_id=conversation_id,
            capture_ids=tuple(str(row["capture_id"]) for row in selected),
            kind=kind,
            speech_seconds=sum(float(row["speech_seconds"] or 0.0) for row in selected),
        )
        newest = str(selected[-1]["captured_at"])
        eligible.append((0 if kind == "final" else 1, newest, batch))
    if not eligible:
        return None
    eligible.sort(key=lambda item: (item[0], item[1]), reverse=False)
    best_priority = eligible[0][0]
    same_priority = [item for item in eligible if item[0] == best_priority]
    return max(same_priority, key=lambda item: item[1])[2]


def live_speech_is_recent(database: Path, quiet_seconds: float) -> bool:
    """Keep backlog work from starting while live Whisper is active or imminent."""

    seconds = max(0.0, float(quiet_seconds))
    if seconds == 0:
        return False
    with database_connection(database) as connection:
        row = connection.execute(
            """
            SELECT EXISTS (
                SELECT 1 FROM chunks
                WHERE kind = 'speech'
                  AND received_at >= datetime('now', ?)
            )
            """,
            (f"-{seconds:g} seconds",),
        ).fetchone()
    return bool(row[0])


def record_started(database: Path, batch: PipelineBatch, report_path: Path) -> None:
    with database_connection(database) as connection:
        ensure_schema(connection)
        connection.execute(
            """
            INSERT INTO identification_pipeline_runs (
                job_id, conversation_id, kind, capture_count, status,
                report_path, attempts, started_at
            ) VALUES (?, ?, ?, ?, 'running', ?, 1, CURRENT_TIMESTAMP)
            ON CONFLICT(job_id) DO UPDATE SET
                status = 'running', report_path = excluded.report_path,
                attempts = identification_pipeline_runs.attempts + 1,
                error = NULL, started_at = CURRENT_TIMESTAMP, finished_at = NULL
            """,
            (batch.job_id, batch.conversation_id, batch.kind, len(batch.capture_ids), str(report_path)),
        )


def record_complete(database: Path, batch: PipelineBatch) -> None:
    with database_connection(database) as connection:
        connection.execute(
            """UPDATE identification_pipeline_runs
               SET status = 'complete', error = NULL, finished_at = CURRENT_TIMESTAMP
               WHERE job_id = ?""",
            (batch.job_id,),
        )
        connection.executemany(
            """INSERT OR REPLACE INTO identification_pipeline_captures
               (capture_id, conversation_id, job_id) VALUES (?, ?, ?)""",
            ((capture_id, batch.conversation_id, batch.job_id) for capture_id in batch.capture_ids),
        )


def record_failed(database: Path, batch: PipelineBatch, error: str) -> None:
    with database_connection(database) as connection:
        connection.execute(
            """UPDATE identification_pipeline_runs
               SET status = 'failed', error = ?, finished_at = CURRENT_TIMESTAMP
               WHERE job_id = ?""",
            (error[-2000:], batch.job_id),
        )


def run_batch(
    database: Path,
    output_dir: Path,
    batch: PipelineBatch,
    diarization_python: Path = DEFAULT_DIARIZATION_PYTHON,
) -> None:
    output_dir.mkdir(parents=True, exist_ok=True)
    report_path = output_dir / f"community-1-auto-{batch.job_id}.json"
    record_started(database, batch, report_path)
    common = ["--database", str(database)]
    diarization_command = [
        str(diarization_python),
        str(SCRIPTS_DIR / "run_community_diarization.py"),
        *common,
        "--conversation-id", batch.conversation_id,
        "--output-dir", str(output_dir),
        "--output-path", str(report_path),
    ]
    for capture_id in batch.capture_ids:
        diarization_command.extend(("--capture-id", capture_id))
    try:
        subprocess.run(diarization_command, check=True, timeout=1800)
        subprocess.run(
            [
                sys.executable,
                str(SCRIPTS_DIR / "build_identification_turns.py"),
                *common,
                "--report", str(report_path),
            ],
            check=True,
            timeout=1800,
        )
        record_complete(database, batch)
        print(
            f"completed {batch.kind} {batch.conversation_id}: "
            f"{len(batch.capture_ids)} captures, {batch.speech_seconds:.1f}s speech",
            flush=True,
        )
    except Exception as exc:
        record_failed(database, batch, repr(exc))
        raise


def lower_process_priority() -> None:
    try:
        import psutil
        process = psutil.Process()
        process.nice(psutil.BELOW_NORMAL_PRIORITY_CLASS if os.name == "nt" else 10)
    except Exception:
        pass


def run_daemon(args: argparse.Namespace) -> None:
    lower_process_priority()
    print(f"identification pipeline watching {args.database}", flush=True)
    while True:
        try:
            if live_speech_is_recent(
                args.database, args.live_speech_quiet_seconds
            ):
                time.sleep(args.poll_seconds)
                continue
            batch = choose_batch(
                args.database,
                lookback_hours=args.lookback_hours,
                checkpoint_seconds=args.checkpoint_seconds,
                quiet_seconds=args.quiet_seconds,
                target_seconds=args.target_seconds,
            )
            if batch:
                print(
                    f"starting {batch.kind} {batch.conversation_id}: "
                    f"{len(batch.capture_ids)} captures",
                    flush=True,
                )
                run_batch(
                    args.database,
                    args.output_dir,
                    batch,
                    args.diarization_python,
                )
                time.sleep(args.cooldown_seconds)
            else:
                time.sleep(args.poll_seconds)
        except KeyboardInterrupt:
            return
        except Exception:
            traceback.print_exc()
            time.sleep(max(args.poll_seconds, 60.0))


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--database", type=Path, default=DEFAULT_DATABASE)
    parser.add_argument("--output-dir", type=Path, default=DEFAULT_OUTPUT_DIR)
    parser.add_argument(
        "--diarization-python", type=Path, default=DEFAULT_DIARIZATION_PYTHON
    )
    parser.add_argument("--poll-seconds", type=float, default=45.0)
    parser.add_argument("--cooldown-seconds", type=float, default=30.0)
    parser.add_argument("--lookback-hours", type=float, default=30.0)
    parser.add_argument("--checkpoint-seconds", type=float, default=600.0)
    parser.add_argument("--quiet-seconds", type=float, default=45.0)
    parser.add_argument("--live-speech-quiet-seconds", type=float, default=75.0)
    parser.add_argument("--target-seconds", type=float, default=300.0)
    parser.add_argument("--once", action="store_true")
    args = parser.parse_args()
    if args.once:
        batch = choose_batch(
            args.database,
            lookback_hours=args.lookback_hours,
            checkpoint_seconds=args.checkpoint_seconds,
            quiet_seconds=args.quiet_seconds,
            target_seconds=args.target_seconds,
        )
        if batch:
            run_batch(
                args.database,
                args.output_dir,
                batch,
                args.diarization_python,
            )
        return 0
    run_daemon(args)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
