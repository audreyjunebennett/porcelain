"""Run a safe Community-1 diarization experiment on real Moto X audio.

The worker reads the journal database in read-only mode, selects a contiguous
window from one conversation, decodes source captures in memory, and writes a
derived JSON report. It never edits source audio, transcripts, or identities.
"""

from __future__ import annotations

import argparse
import json
import os
import sqlite3
import subprocess
from dataclasses import dataclass
from datetime import datetime
from pathlib import Path

import numpy as np
import pyannote.audio
import torch
from pyannote.audio import Pipeline


SAMPLE_RATE = 16_000
SEPARATOR_SECONDS = 0.25
MODEL_ID = "pyannote/speaker-diarization-community-1"


@dataclass(frozen=True)
class Capture:
    capture_id: str
    captured_at: str
    duration_seconds: float
    speech_seconds: float
    audio_path: Path


def _connect_readonly(database: Path) -> sqlite3.Connection:
    connection = sqlite3.connect(f"file:{database.as_posix()}?mode=ro", uri=True)
    connection.row_factory = sqlite3.Row
    return connection


def choose_conversation(database: Path, conversation_id: str | None) -> str:
    if conversation_id:
        return conversation_id
    with _connect_readonly(database) as connection:
        row = connection.execute(
            """
            SELECT conversation_id
            FROM chunks
            WHERE kind = 'speech' AND audio_path IS NOT NULL
              AND trim(transcript) <> '' AND conversation_id IS NOT NULL
            GROUP BY conversation_id
            HAVING SUM(COALESCE(speech_seconds, 0)) >= 30
            ORDER BY MAX(captured_at) DESC
            LIMIT 1
            """
        ).fetchone()
    if not row:
        raise RuntimeError("no conversation with enough speech was found")
    return str(row["conversation_id"])


def conversation_captures(database: Path, conversation_id: str) -> list[Capture]:
    with _connect_readonly(database) as connection:
        rows = connection.execute(
            """
            SELECT capture_id, captured_at, duration_seconds, speech_seconds,
                   audio_path
            FROM chunks
            WHERE conversation_id = ? AND kind = 'speech'
              AND audio_path IS NOT NULL AND trim(transcript) <> ''
            ORDER BY captured_at, capture_id
            """,
            (conversation_id,),
        ).fetchall()
    captures = []
    for row in rows:
        path = Path(row["audio_path"])
        if path.is_file():
            captures.append(
                Capture(
                    capture_id=str(row["capture_id"]),
                    captured_at=str(row["captured_at"]),
                    duration_seconds=float(row["duration_seconds"] or 30.0),
                    speech_seconds=float(row["speech_seconds"] or 0.0),
                    audio_path=path,
                )
            )
    if not captures:
        raise RuntimeError(f"conversation has no readable captures: {conversation_id}")
    return captures


def strongest_window(captures: list[Capture], target_seconds: float) -> list[Capture]:
    """Choose a contiguous target-sized window with the most detected speech."""

    best: tuple[float, float, list[Capture]] | None = None
    for start in range(len(captures)):
        duration = 0.0
        speech = 0.0
        window = []
        for capture in captures[start:]:
            next_duration = duration + capture.duration_seconds
            if window and next_duration > target_seconds + 30.0:
                break
            window.append(capture)
            duration = next_duration
            speech += capture.speech_seconds
            if duration >= target_seconds - 30.0:
                candidate = (speech, -abs(duration - target_seconds), list(window))
                if best is None or candidate[:2] > best[:2]:
                    best = candidate
    if best is None:
        return captures
    return best[2]


def decode(path: Path, ffmpeg: str) -> torch.Tensor:
    process = subprocess.run(
        [
            ffmpeg,
            "-hide_banner",
            "-loglevel",
            "error",
            "-i",
            str(path),
            "-ac",
            "1",
            "-ar",
            str(SAMPLE_RATE),
            "-f",
            "f32le",
            "pipe:1",
        ],
        capture_output=True,
        timeout=90,
        check=False,
    )
    if process.returncode != 0 or not process.stdout:
        detail = process.stderr.decode("utf-8", errors="replace")[:500]
        raise RuntimeError(f"FFmpeg failed for {path.name}: {detail}")
    return torch.from_numpy(np.frombuffer(process.stdout, dtype=np.float32).copy())


def assemble(captures: list[Capture], ffmpeg: str) -> tuple[torch.Tensor, list[dict]]:
    pieces: list[torch.Tensor] = []
    source_map: list[dict] = []
    cursor = 0
    separator = torch.zeros(round(SAMPLE_RATE * SEPARATOR_SECONDS))
    for index, capture in enumerate(captures, 1):
        waveform = decode(capture.audio_path, ffmpeg)
        start = cursor / SAMPLE_RATE
        pieces.append(waveform)
        cursor += waveform.numel()
        end = cursor / SAMPLE_RATE
        source_map.append(
            {
                "capture_id": capture.capture_id,
                "captured_at": capture.captured_at,
                "source_path": str(capture.audio_path),
                "batch_start": round(start, 3),
                "batch_end": round(end, 3),
                "detected_speech_seconds": capture.speech_seconds,
            }
        )
        print(f"decoded {index}/{len(captures)} {capture.capture_id}", flush=True)
        if index < len(captures):
            pieces.append(separator)
            cursor += separator.numel()
    return torch.cat(pieces), source_map


def project_turns(annotation, source_map: list[dict]) -> list[dict]:
    turns = []
    for turn, _, speaker in annotation.itertracks(yield_label=True):
        for source in source_map:
            overlap_start = max(float(turn.start), source["batch_start"])
            overlap_end = min(float(turn.end), source["batch_end"])
            if overlap_end <= overlap_start:
                continue
            turns.append(
                {
                    "capture_id": source["capture_id"],
                    "speaker_cluster": str(speaker),
                    "source_start": round(overlap_start - source["batch_start"], 3),
                    "source_end": round(overlap_end - source["batch_start"], 3),
                    "batch_start": round(overlap_start, 3),
                    "batch_end": round(overlap_end, 3),
                }
            )
    return turns


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--database", type=Path, required=True)
    parser.add_argument("--conversation-id")
    parser.add_argument("--target-seconds", type=float, default=300.0)
    parser.add_argument("--output-dir", type=Path, required=True)
    parser.add_argument("--ffmpeg", default="ffmpeg")
    args = parser.parse_args()

    conversation_id = choose_conversation(args.database, args.conversation_id)
    captures = strongest_window(
        conversation_captures(args.database, conversation_id),
        max(60.0, min(args.target_seconds, 1800.0)),
    )
    waveform, source_map = assemble(captures, args.ffmpeg)

    token = os.environ.get("HF_TOKEN")
    if not token:
        raise RuntimeError("HF_TOKEN is not available")
    pipeline = Pipeline.from_pretrained(MODEL_ID, token=token)
    device = torch.device("cuda" if torch.cuda.is_available() else "cpu")
    pipeline.to(device)
    with torch.inference_mode():
        result = pipeline({"waveform": waveform.unsqueeze(0), "sample_rate": SAMPLE_RATE})

    ordinary = getattr(result, "speaker_diarization", result)
    exclusive = getattr(result, "exclusive_speaker_diarization", ordinary)
    report = {
        "schema_version": 1,
        "generated_at": datetime.now().astimezone().isoformat(timespec="seconds"),
        "model": MODEL_ID,
        "pyannote_audio_version": pyannote.audio.__version__,
        "torch_version": torch.__version__,
        "device": str(device),
        "conversation_id": conversation_id,
        "batch_seconds": round(waveform.numel() / SAMPLE_RATE, 3),
        "detected_speech_seconds": round(sum(c.speech_seconds for c in captures), 3),
        "sources": source_map,
        "ordinary_turns": project_turns(ordinary, source_map),
        "exclusive_turns": project_turns(exclusive, source_map),
    }
    args.output_dir.mkdir(parents=True, exist_ok=True)
    stamp = datetime.now().strftime("%Y%m%d-%H%M%S")
    destination = args.output_dir / f"community-1-{stamp}.json"
    destination.write_text(json.dumps(report, indent=2), encoding="utf-8")
    print(f"wrote {destination}", flush=True)
    print(
        f"batch={report['batch_seconds']}s speech={report['detected_speech_seconds']}s "
        f"ordinary_turns={len(report['ordinary_turns'])} "
        f"exclusive_turns={len(report['exclusive_turns'])}",
        flush=True,
    )


if __name__ == "__main__":
    main()
