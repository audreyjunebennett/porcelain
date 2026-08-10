"""Create anonymous Moto X voice-group proposals for the review interface.

This is an intentionally conservative bootstrap worker. It uses public WavLM
features to group recent speech clips and never assigns a human identity.
Community-1 can replace these proposals later without changing corrections.
"""

from __future__ import annotations

import argparse
import math
import sqlite3
import subprocess
from pathlib import Path

import numpy as np
import torch
import torchaudio
from sklearn.cluster import KMeans

from motox_review import MotoXReviewStore


SAMPLE_RATE = 16000
FRAME_SECONDS = 1
MAX_AUDIO_SECONDS = 12


def recent_speech(database: Path, limit: int) -> list[dict]:
    uri = f"file:{database.as_posix()}?mode=ro"
    connection = sqlite3.connect(uri, uri=True)
    connection.row_factory = sqlite3.Row
    try:
        rows = connection.execute(
            """
            SELECT capture_id, audio_path, transcript, captured_at
            FROM chunks
            WHERE kind = 'speech' AND audio_path IS NOT NULL
              AND trim(transcript) <> ''
            ORDER BY captured_at DESC, capture_id DESC
            LIMIT ?
            """,
            (limit * 3,),
        ).fetchall()
    finally:
        connection.close()
    result = []
    for row in rows:
        path = Path(row["audio_path"])
        if path.is_file():
            result.append(dict(row))
        if len(result) >= limit:
            break
    return result


def decode_pcm(path: Path, ffmpeg: str) -> torch.Tensor:
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
        timeout=60,
        check=False,
    )
    if process.returncode != 0 or len(process.stdout) < SAMPLE_RATE * 4:
        raise RuntimeError(process.stderr.decode("utf-8", errors="replace")[:300])
    return torch.from_numpy(np.frombuffer(process.stdout, dtype=np.float32).copy())


def loud_speech_candidate(waveform: torch.Tensor) -> torch.Tensor:
    """Keep energetic one-second regions while retaining their time order."""

    frame = SAMPLE_RATE * FRAME_SECONDS
    count = waveform.numel() // frame
    if count <= MAX_AUDIO_SECONDS:
        return waveform[: count * frame]
    framed = waveform[: count * frame].reshape(count, frame)
    rms = torch.sqrt(torch.mean(framed.square(), dim=1) + 1e-10)
    keep = min(MAX_AUDIO_SECONDS, max(4, int(math.ceil(count * 0.45))))
    indices = torch.topk(rms, keep).indices.sort().values
    return framed[indices].reshape(-1)


def embeddings(rows: list[dict], ffmpeg: str) -> tuple[list[dict], np.ndarray]:
    device = torch.device("cuda" if torch.cuda.is_available() else "cpu")
    bundle = torchaudio.pipelines.WAVLM_BASE_PLUS
    model = bundle.get_model().to(device).eval()
    usable = []
    vectors = []
    with torch.inference_mode():
        for index, row in enumerate(rows, 1):
            try:
                waveform = loud_speech_candidate(decode_pcm(Path(row["audio_path"]), ffmpeg))
                features, _ = model.extract_features(waveform[None].to(device))
                vector = features[-1].mean(dim=1).squeeze(0)
                vector = torch.nn.functional.normalize(vector, dim=0)
                vectors.append(vector.cpu().numpy())
                usable.append(row)
                print(f"embedded {index}/{len(rows)} {row['capture_id']}", flush=True)
            except Exception as exc:
                print(f"skipped {row['capture_id']}: {exc}", flush=True)
    return usable, np.stack(vectors)


def cluster_proposals(rows: list[dict], vectors: np.ndarray, clusters: int) -> list[dict]:
    clusters = min(max(2, clusters), len(rows))
    model = KMeans(n_clusters=clusters, random_state=20260808, n_init=20)
    labels = model.fit_predict(vectors)
    distances = model.transform(vectors)
    proposals = []
    for row, cluster, distance_row in zip(rows, labels, distances):
        own = float(distance_row[cluster])
        alternatives = np.delete(distance_row, cluster)
        nearest_other = float(alternatives.min()) if alternatives.size else own + 1.0
        confidence = max(0.0, min(0.99, 1.0 - own / max(nearest_other, 1e-6)))
        proposals.append(
            {
                "capture_id": row["capture_id"],
                "start_char": 0,
                "end_char": len(row["transcript"] or ""),
                "proposal_type": "speaker",
                "cluster_id": f"Voice group {chr(65 + int(cluster))}",
                "confidence": round(confidence, 4),
            }
        )
    return proposals


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--database", type=Path, required=True)
    parser.add_argument("--limit", type=int, default=48)
    parser.add_argument("--clusters", type=int, default=6)
    parser.add_argument("--ffmpeg", default="ffmpeg")
    args = parser.parse_args()

    rows = recent_speech(args.database, max(6, min(args.limit, 200)))
    if len(rows) < 6:
        raise SystemExit("not enough readable speech clips")
    rows, vectors = embeddings(rows, args.ffmpeg)
    proposals = cluster_proposals(rows, vectors, args.clusters)
    stored = MotoXReviewStore(args.database).add_proposals(
        proposals,
        model_name="torchaudio/wavlm-base-plus-kmeans",
        model_version="pilot-1",
    )
    print(f"stored {stored} anonymous voice-group proposals", flush=True)


if __name__ == "__main__":
    main()
