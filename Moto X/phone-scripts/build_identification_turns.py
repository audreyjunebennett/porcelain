"""Build adaptive speaker-identification questions from a diarization report.

The source report and audio remain unchanged. This worker filters Community-1
turns to short, useful regions, extracts public WavLM embeddings, and stores
only the derived vectors plus exact source-audio anchors in the review DB.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import math
import sqlite3
import subprocess
from collections import defaultdict
from pathlib import Path

import numpy as np
import torch
import torchaudio

from motox_review import MotoXReviewStore


SAMPLE_RATE = 16_000
EMBEDDING_MODEL = "torchaudio/wavlm-base-plus"
TARGET_SECONDS = 4.0


def merge_turns(
    turns: list[dict], *, min_seconds: float, max_seconds: float, merge_gap: float
) -> list[dict]:
    """Merge adjacent same-cluster fragments and retain concise clean turns."""

    ordered = sorted(
        turns,
        key=lambda turn: (
            str(turn["capture_id"]),
            float(turn["source_start"]),
            float(turn["source_end"]),
        ),
    )
    merged: list[dict] = []
    for raw in ordered:
        turn = {
            "capture_id": str(raw["capture_id"]),
            "speaker_cluster": str(raw["speaker_cluster"]),
            "source_start": max(0.0, float(raw["source_start"])),
            "source_end": max(0.0, float(raw["source_end"])),
        }
        if turn["source_end"] <= turn["source_start"]:
            continue
        previous = merged[-1] if merged else None
        if (
            previous
            and previous["capture_id"] == turn["capture_id"]
            and previous["speaker_cluster"] == turn["speaker_cluster"]
            and turn["source_start"] - previous["source_end"] <= merge_gap
            and turn["source_end"] - previous["source_start"] <= max_seconds
        ):
            previous["source_end"] = max(previous["source_end"], turn["source_end"])
        else:
            merged.append(turn)

    usable = []
    for turn in merged:
        duration = turn["source_end"] - turn["source_start"]
        if duration < min_seconds:
            continue
        if duration > max_seconds:
            center = (turn["source_start"] + turn["source_end"]) / 2
            half = min(TARGET_SECONDS, max_seconds) / 2
            turn["source_start"] = center - half
            turn["source_end"] = center + half
        usable.append(turn)
    return usable


def eligible_audio_paths(database: Path, capture_ids: set[str]) -> dict[str, Path]:
    if not capture_ids:
        return {}
    connection = sqlite3.connect(f"file:{database.as_posix()}?mode=ro", uri=True)
    connection.row_factory = sqlite3.Row
    try:
        result = {}
        ordered = sorted(capture_ids)
        for offset in range(0, len(ordered), 900):
            batch = ordered[offset:offset + 900]
            placeholders = ",".join("?" for _ in batch)
            rows = connection.execute(
                f"""
                SELECT capture_id, audio_path FROM chunks
                WHERE capture_id IN ({placeholders}) AND audio_path IS NOT NULL
                  AND NOT EXISTS (
                      SELECT 1 FROM review_annotations privacy
                      WHERE privacy.capture_id = chunks.capture_id
                        AND privacy.annotation_type = 'privacy'
                        AND privacy.label = 'Exclude'
                        AND privacy.reverted_at IS NULL
                  )
                """,
                batch,
            ).fetchall()
            for row in rows:
                path = Path(row["audio_path"])
                if path.is_file():
                    result[str(row["capture_id"])] = path
        return result
    finally:
        connection.close()


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
        detail = process.stderr.decode("utf-8", errors="replace")[:400]
        raise RuntimeError(f"FFmpeg failed for {path.name}: {detail}")
    return torch.from_numpy(np.frombuffer(process.stdout, dtype=np.float32).copy())


def turn_id(source_id: str, turn: dict) -> str:
    identity = "|".join(
        (
            source_id,
            turn["capture_id"],
            turn["speaker_cluster"],
            f"{turn['source_start']:.3f}",
            f"{turn['source_end']:.3f}",
        )
    )
    return hashlib.sha256(identity.encode("utf-8")).hexdigest()[:32]


def quality_score(waveform: torch.Tensor) -> float:
    duration = waveform.numel() / SAMPLE_RATE
    duration_quality = max(0.0, 1.0 - abs(duration - TARGET_SECONDS) / TARGET_SECONDS)
    rms = math.sqrt(float(torch.mean(waveform.square()).item()) + 1e-12)
    rms_quality = max(0.0, min(1.0, (math.log10(rms) + 3.0) / 2.0))
    return round(duration_quality * 0.72 + rms_quality * 0.28, 4)


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--database", type=Path, required=True)
    parser.add_argument("--report", type=Path, required=True)
    parser.add_argument("--ffmpeg", default="ffmpeg")
    parser.add_argument("--min-seconds", type=float, default=1.5)
    parser.add_argument("--max-seconds", type=float, default=8.0)
    parser.add_argument("--merge-gap", type=float, default=0.35)
    parser.add_argument("--dry-run", action="store_true")
    args = parser.parse_args()

    report = json.loads(args.report.read_text(encoding="utf-8"))
    source_id = args.report.stem
    turns = merge_turns(
        list(report.get("exclusive_turns") or []),
        min_seconds=max(0.75, float(args.min_seconds)),
        max_seconds=max(2.0, float(args.max_seconds)),
        merge_gap=max(0.0, float(args.merge_gap)),
    )
    paths = eligible_audio_paths(
        args.database, {turn["capture_id"] for turn in turns}
    )
    turns = [turn for turn in turns if turn["capture_id"] in paths]
    print(
        f"report={source_id} usable_turns={len(turns)} "
        f"captures={len({turn['capture_id'] for turn in turns})}",
        flush=True,
    )
    if args.dry_run or not turns:
        return

    device = torch.device("cuda" if torch.cuda.is_available() else "cpu")
    bundle = torchaudio.pipelines.WAVLM_BASE_PLUS
    model = bundle.get_model().to(device).eval()
    by_capture: dict[str, list[dict]] = defaultdict(list)
    for turn in turns:
        by_capture[turn["capture_id"]].append(turn)

    derived = []
    processed = 0
    with torch.inference_mode():
        for capture_id, capture_turns in by_capture.items():
            try:
                waveform = decode(paths[capture_id], args.ffmpeg)
            except Exception as exc:
                print(f"skipped capture {capture_id}: {exc}", flush=True)
                continue
            for turn in capture_turns:
                start_sample = round(turn["source_start"] * SAMPLE_RATE)
                end_sample = min(waveform.numel(), round(turn["source_end"] * SAMPLE_RATE))
                segment = waveform[start_sample:end_sample]
                if segment.numel() < round(args.min_seconds * SAMPLE_RATE):
                    continue
                try:
                    features, _ = model.extract_features(segment[None].to(device))
                    vector = features[-1].mean(dim=1).squeeze(0)
                    vector = torch.nn.functional.normalize(vector, dim=0)
                except Exception as exc:
                    print(f"skipped turn {capture_id} {turn['source_start']:.1f}s: {exc}", flush=True)
                    continue
                namespaced_cluster = f"{source_id}/{turn['speaker_cluster']}"
                derived.append(
                    {
                        "turn_id": turn_id(source_id, turn),
                        "capture_id": capture_id,
                        "conversation_id": report.get("conversation_id"),
                        "audio_start_seconds": round(turn["source_start"], 3),
                        "audio_end_seconds": round(turn["source_end"], 3),
                        "cluster_id": namespaced_cluster,
                        "embedding": vector.cpu().tolist(),
                        "quality": quality_score(segment),
                    }
                )
                processed += 1
                print(f"embedded {processed}/{len(turns)} {capture_id}", flush=True)

    stored = MotoXReviewStore(args.database).add_speaker_turns(
        derived,
        embedding_model=EMBEDDING_MODEL,
        embedding_version=torchaudio.__version__,
        diarization_model=str(report.get("model") or "unknown"),
        source_id=source_id,
    )
    print(f"stored {stored} adaptive identification turns", flush=True)


if __name__ == "__main__":
    main()
