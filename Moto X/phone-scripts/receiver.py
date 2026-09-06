import hashlib
import json
import os
import re
import signal
import subprocess
import sys
import threading
import wave
from datetime import datetime, timedelta
from pathlib import Path

# Load D:\Rebirth\.env if present (gitignored, holds HF_TOKEN and gateway creds)
try:
    from dotenv import load_dotenv
    _env = Path(__file__).resolve().parent.parent / ".env"
    if _env.exists():
        load_dotenv(_env)
except ImportError:
    pass

import httpx
import numpy as np
import torch
from flask import Flask, Response, jsonify, request, send_file, send_from_directory

from motox_journal_view import render_journal_html
from motox_review import MotoXReviewStore
from motox_v1 import MotoXStore, event_from_receiver, legacy_capture_identity


APP = Flask(__name__)
DASHBOARD_ASSET_DIR = Path(__file__).resolve().parent / "dashboard_assets"

BASE_DIR = Path(
    os.environ.get(
        "MOTOX_AUDIO_DIR",
        str(Path(__file__).resolve().parent.parent / "motox_audio_data"),
    )
)
INBOX_DIR = BASE_DIR / "incoming"
AUDIO_DIR = BASE_DIR / "audio"
TRANSCRIPT_DIR = BASE_DIR / "transcripts"
CONVERSATION_DIR = BASE_DIR / "conversations"
WORK_DIR = BASE_DIR / "work"
ERROR_DIR = BASE_DIR / "errors"
SILENCE_LOG = BASE_DIR / "silence_log.md"
CONVERSATION_STATE = BASE_DIR / "conversation_state.json"

# Timestamped append-only log (Syncthing: live next to receiver.py under Moto X/)
RECEIVER_LOG = Path(
    os.environ.get(
        "MOTOX_RECEIVER_LOG",
        str(Path(__file__).resolve().parent / "receiver_log.md"),
    )
)
_receiver_log_lock = threading.Lock()

ALLOWED_SUFFIXES = {".m4a", ".mp3", ".mp4", ".aac", ".amr", ".3gp", ".wav", ".ogg", ".flac"}
AMBIENT_RMS_FLOOR = float(os.environ.get("MOTOX_AMBIENT_RMS_FLOOR", "0.006"))
MIN_SPEECH_SECONDS = float(os.environ.get("MOTOX_MIN_SPEECH_SECONDS", "0.40"))
WHISPER_MODEL = os.environ.get("MOTOX_WHISPER_MODEL", "large-v3")
WHISPER_DEVICE = os.environ.get("MOTOX_WHISPER_DEVICE", "cuda")
WHISPER_COMPUTE_TYPE = os.environ.get("MOTOX_WHISPER_COMPUTE_TYPE", "float16")
FFMPEG_BIN = os.environ.get("MOTOX_FFMPEG_BIN", "ffmpeg")
DEFAULT_SPEAKER = os.environ.get("MOTOX_DEFAULT_SPEAKER", "Ruby")
CONVERSATION_GAP_SECONDS = int(os.environ.get("MOTOX_CONVERSATION_GAP_SECONDS", "120"))
V1_ENABLED = os.environ.get("MOTOX_V1_ENABLED", "1") == "1"
V1_DATABASE = Path(os.environ.get("MOTOX_V1_DATABASE", str(BASE_DIR / "motox_v1.sqlite3")))
V1_DAILY_DIR = Path(os.environ.get("MOTOX_V1_DAILY_DIR", str(BASE_DIR / "daily")))
WHISPER_PROMPT = os.environ.get(
    "MOTOX_WHISPER_PROMPT",
    (
        "Casual close-mic English journal speech by Ruby about Codex, Claude, "
        "Porcelain, Locus, Chimera, Rebirth, Moto X, Termux, receiver.py, record.py, "
        "Whisper, faster-whisper, Silero VAD, audio transcription, and local AI."
    ),
)

# Gateway auto-ingest config
GATEWAY_URL            = os.environ.get("CHIMERA_GATEWAY_URL",    "http://localhost:3000")
GATEWAY_TOKEN          = os.environ.get("CHIMERA_GATEWAY_TOKEN",  "")
GATEWAY_INGEST_ENABLED = os.environ.get("MOTOX_GATEWAY_INGEST", "0") == "1"

# Diarization config — token must come from .env or system env
HF_TOKEN = os.environ.get("HF_TOKEN", "")
if not HF_TOKEN:
    print("  [!] WARNING: HF_TOKEN not set — diarization will fail. Add to D:\\Rebirth\\.env.", file=sys.stderr)
UNKNOWN_SPEAKER = os.environ.get("MOTOX_UNKNOWN_SPEAKER", "friend")
ENABLE_DIARIZATION = os.environ.get("MOTOX_DIARIZATION", "0") == "1"

for directory in (INBOX_DIR, AUDIO_DIR, TRANSCRIPT_DIR, CONVERSATION_DIR, WORK_DIR, ERROR_DIR):
    directory.mkdir(parents=True, exist_ok=True)

V1_STORE = (
    MotoXStore(V1_DATABASE, V1_DAILY_DIR, gap_seconds=CONVERSATION_GAP_SECONDS)
    if V1_ENABLED
    else None
)
REVIEW_STORE = MotoXReviewStore(V1_DATABASE) if V1_STORE is not None else None
_REVIEW_EMBEDDING_LOCK = threading.Lock()
_REVIEW_EMBEDDING_MODEL = None
_REVIEW_EMBEDDING_VERSION = None


def extract_review_speaker_embedding(
    capture_id: str, start_seconds: float, end_seconds: float
) -> tuple[list[float], str, str]:
    """Embed the exact human-trimmed range used as a voice example."""

    audio_path = REVIEW_STORE.get_audio_path(capture_id) if REVIEW_STORE else None
    if audio_path is None:
        raise RuntimeError("speaker example audio is unavailable")
    duration = float(end_seconds) - float(start_seconds)
    process = subprocess.run(
        [
            FFMPEG_BIN,
            "-hide_banner",
            "-loglevel",
            "error",
            "-ss",
            str(float(start_seconds)),
            "-t",
            str(duration),
            "-i",
            str(audio_path),
            "-ac",
            "1",
            "-ar",
            "16000",
            "-f",
            "f32le",
            "pipe:1",
        ],
        capture_output=True,
        timeout=90,
        check=False,
    )
    if process.returncode != 0 or not process.stdout:
        raise RuntimeError("could not decode the trimmed speaker example")
    waveform = torch.from_numpy(
        np.frombuffer(process.stdout, dtype=np.float32).copy()
    )
    if waveform.numel() < 8000:
        raise RuntimeError("trimmed speaker example is too short")

    global _REVIEW_EMBEDDING_MODEL, _REVIEW_EMBEDDING_VERSION
    with _REVIEW_EMBEDDING_LOCK, torch.inference_mode():
        if _REVIEW_EMBEDDING_MODEL is None:
            import torchaudio

            _REVIEW_EMBEDDING_MODEL = (
                torchaudio.pipelines.WAVLM_BASE_PLUS.get_model().cpu().eval()
            )
            _REVIEW_EMBEDDING_VERSION = torchaudio.__version__
        features, _ = _REVIEW_EMBEDDING_MODEL.extract_features(waveform[None])
        vector = torch.nn.functional.normalize(
            features[-1].mean(dim=1).squeeze(0), dim=0
        )
    return (
        vector.cpu().tolist(),
        "torchaudio/wavlm-base-plus",
        str(_REVIEW_EMBEDDING_VERSION),
    )

print("Loading Silero VAD model...")
torch.set_num_threads(1)
vad_model, vad_utils = torch.hub.load(
    repo_or_dir="snakers4/silero-vad",
    model="silero_vad",
    force_reload=False,
)
(get_speech_timestamps, _, read_audio, *_) = vad_utils
print("VAD ready.")

whisper_model = None
diarization_pipeline = None
_diarization_failed = False


def timestamp_now():
    return datetime.now().strftime("%Y-%m-%d_%H-%M-%S")


def receiver_log(msg: str) -> None:
    """Append one line to RECEIVER_LOG (Syncthing-visible on PC) and echo to stdout."""
    ts = datetime.now().strftime("%Y-%m-%d %H:%M:%S")
    line = f"[{ts}] {msg}"
    print(line)
    try:
        RECEIVER_LOG.parent.mkdir(parents=True, exist_ok=True)
        with _receiver_log_lock:
            with open(RECEIVER_LOG, "a", encoding="utf-8") as f:
                f.write(line + "\n")
    except OSError:
        pass


def parse_timestamp(timestamp):
    return datetime.strptime(timestamp, "%Y-%m-%d_%H-%M-%S")


def safe_suffix(filename):
    suffix = Path(filename or "").suffix.lower()
    return suffix if suffix in ALLOWED_SUFFIXES else ".m4a"


CAPTURE_ID_RE = re.compile(r"^[A-Za-z0-9_.-]{1,100}$")
CAPTURE_TIMESTAMP_RE = re.compile(r"^\d{4}-\d{2}-\d{2}_\d{2}-\d{2}-\d{2}$")


def safe_capture_id(value, fallback):
    value = (value or "").strip()
    return value if CAPTURE_ID_RE.fullmatch(value) else fallback


def safe_capture_timestamp(value, fallback):
    value = (value or "").strip()
    if not CAPTURE_TIMESTAMP_RE.fullmatch(value):
        return fallback
    try:
        parse_timestamp(value)
    except ValueError:
        return fallback
    return value


def record_v1_chunk(capture_id, captured_at, kind, classification, audio_path, transcript, speaker):
    if V1_STORE is None:
        return None
    try:
        result = V1_STORE.record_chunk(
            event_from_receiver(
                capture_id,
                captured_at,
                kind,
                classification,
                audio_path,
                transcript,
                speaker,
            )
        )
        if result.get("closed_conversation_id"):
            receiver_log(
                f"[chunk {capture_id}] v1 finalized: "
                f"{result['closed_conversation_id']}"
            )
        return result
    except Exception as exc:
        # Journal projection is additive during the v1 rollout. Never sacrifice
        # an audio upload because its projection failed.
        receiver_log(f"[chunk {capture_id}] v1 journal error: {exc}")
        return None


def run_ffmpeg_to_wav(source_path, wav_path):
    command = [
        FFMPEG_BIN,
        "-y",
        "-hide_banner",
        "-loglevel",
        "error",
        "-i",
        str(source_path),
        "-ac",
        "1",
        "-ar",
        "16000",
        "-c:a",
        "pcm_s16le",
        str(wav_path),
    ]
    return subprocess.run(command, capture_output=True, text=True, timeout=90)


def read_pcm16_wav(wav_path):
    with wave.open(str(wav_path), "rb") as wav_file:
        channels = wav_file.getnchannels()
        sample_width = wav_file.getsampwidth()
        sample_rate = wav_file.getframerate()
        frame_count = wav_file.getnframes()
        raw = wav_file.readframes(frame_count)

    if channels != 1 or sample_width != 2 or sample_rate != 16000:
        raise ValueError(
            f"expected 16 kHz mono PCM16 WAV, got {sample_rate} Hz, "
            f"{channels} channel(s), {sample_width * 8}-bit"
        )

    audio_np = np.frombuffer(raw, dtype=np.int16).astype(np.float32) / 32768.0
    return torch.from_numpy(audio_np.copy())


def classify_chunk_from_tensor(wav):
    duration_seconds = float(wav.shape[0]) / 16000.0 if wav.shape[0] else 0.0
    speech_timestamps = get_speech_timestamps(wav, vad_model, sampling_rate=16000)
    speech_seconds = sum((item["end"] - item["start"]) / 16000.0 for item in speech_timestamps)

    audio_np = wav.numpy()
    rms = float(np.sqrt(np.mean(audio_np**2))) if audio_np.size else 0.0

    if speech_seconds >= MIN_SPEECH_SECONDS:
        chunk_type = "speech"
    elif rms >= AMBIENT_RMS_FLOOR:
        chunk_type = "ambient"
    else:
        chunk_type = "silence"

    return {
        "type": chunk_type,
        "duration_seconds": round(duration_seconds, 3),
        "speech_seconds": round(speech_seconds, 3),
        "rms": round(rms, 6),
        "speech_regions": len(speech_timestamps),
    }


def classify_chunk(wav_path):
    return classify_chunk_from_tensor(read_pcm16_wav(wav_path))


def get_whisper_model():
    global whisper_model
    if whisper_model is None:
        from faster_whisper import WhisperModel

        print(f"Loading faster-whisper model: {WHISPER_MODEL} ({WHISPER_DEVICE}, {WHISPER_COMPUTE_TYPE})")
        whisper_model = WhisperModel(
            WHISPER_MODEL,
            device=WHISPER_DEVICE,
            compute_type=WHISPER_COMPUTE_TYPE,
        )
        print("Whisper ready.")
    return whisper_model


def get_diarization_pipeline():
    global diarization_pipeline, _diarization_failed
    if _diarization_failed or not ENABLE_DIARIZATION:
        return None
    if diarization_pipeline is not None:
        return diarization_pipeline
    try:
        from pyannote.audio import Pipeline
        print("Loading pyannote diarization model (first run may download ~800MB)...")
        diarization_pipeline = Pipeline.from_pretrained(
            "pyannote/speaker-diarization-3.1",
            token=HF_TOKEN,
        )
        device = torch.device("cuda" if torch.cuda.is_available() else "cpu")
        diarization_pipeline.to(device)
        print(f"Diarization ready on {device}.")
        return diarization_pipeline
    except Exception as exc:
        print(f"Diarization load failed (transcripts will have no speaker labels): {exc}")
        _diarization_failed = True
        return None


def transcribe_audio(audio_path):
    try:
        model = get_whisper_model()
        segments_gen, info = model.transcribe(
            str(audio_path),
            language="en",
            beam_size=5,
            best_of=5,
            temperature=0.0,
            condition_on_previous_text=False,
            initial_prompt=WHISPER_PROMPT,
            vad_filter=True,
            word_timestamps=True,
        )
        segments = list(segments_gen)
        text = " ".join(s.text.strip() for s in segments if s.text.strip()).strip()
        timed_text, words = timed_words_from_segments(segments)
        return (
            text or "[speech detected, but no transcript text returned]",
            {
                "language": getattr(info, "language", None),
                "language_probability": getattr(info, "language_probability", None),
                "timed_text": timed_text,
                "words": words,
            },
            segments,
        )
    except ImportError:
        return "[transcription unavailable: install faster-whisper]", {}, []
    except Exception as exc:
        return f"[transcription error: {exc}]", {}, []


def timed_words_from_segments(segments):
    """Build exact display text plus word/audio offsets from faster-whisper."""
    text = ""
    result = []
    for segment in segments:
        for word in getattr(segment, "words", None) or []:
            token = str(getattr(word, "word", ""))
            if not token:
                continue
            if not text:
                token = token.lstrip()
            start_char = len(text)
            text += token
            result.append(
                {
                    "word": token,
                    "start_seconds": float(getattr(word, "start", 0.0)),
                    "end_seconds": float(getattr(word, "end", 0.0)),
                    "probability": getattr(word, "probability", None),
                    "char_start": start_char,
                    "char_end": len(text),
                }
            )
    return text.strip(), result


def diarize_audio(wav_tensor):
    """Run speaker diarization on a pre-loaded waveform tensor.
    Accepts the 1D float32 tensor returned by read_pcm16_wav().
    Returns [(start, end, speaker_id), ...] or [].
    """
    pipeline = get_diarization_pipeline()
    if pipeline is None:
        return []
    try:
        # pyannote expects (channels, time) — unsqueeze adds the channel dim
        audio_input = {
            "waveform": wav_tensor.unsqueeze(0),
            "sample_rate": 16000,
        }
        diarization = pipeline(audio_input)
        # pyannote 4.x returns DiarizeOutput (dataclass); older versions return
        # Annotation directly.  Use exclusive_speaker_diarization for transcription
        # (no overlapping turns) when available.
        annotation = getattr(
            diarization,
            "exclusive_speaker_diarization",
            getattr(diarization, "speaker_diarization", diarization),
        )
        return [
            (turn.start, turn.end, speaker)
            for turn, _, speaker in annotation.itertracks(yield_label=True)
        ]
    except Exception as exc:
        receiver_log(f"diarization error (plain transcript): {exc}")
        return []


def map_speakers(diarization_segments):
    """Give diarization clusters anonymous display names.

    Identity is a separate evidence-backed layer. Duration, microphone
    proximity, and ordering never turn an anonymous cluster into Ruby, Lynn,
    Raven, or another known identity.
    """
    if not diarization_segments:
        return {}

    speech_time = {}
    for start, end, sp in diarization_segments:
        speech_time[sp] = speech_time.get(sp, 0.0) + (end - start)

    sorted_speakers = sorted(speech_time, key=lambda speaker: str(speaker))
    return {
        speaker: f"Voice {chr(65 + index)}"
        for index, speaker in enumerate(sorted_speakers)
    }


def format_diarized_transcript(whisper_segments, diarization_segments, speaker_mapping):
    """Build a speaker-annotated transcript from Whisper segments + diarization."""
    if not diarization_segments or not speaker_mapping or not whisper_segments:
        return " ".join(s.text.strip() for s in whisper_segments if s.text.strip())

    lines = []
    current_speaker = None
    current_words = []

    for seg in whisper_segments:
        text = seg.text.strip()
        if not text:
            continue

        # Find which speaker was active at the midpoint of this Whisper segment
        mid = (seg.start + seg.end) / 2
        speaker_id = next(
            (sp for start, end, sp in diarization_segments if start <= mid <= end),
            None,
        )
        name = speaker_mapping.get(speaker_id, UNKNOWN_SPEAKER)

        if name != current_speaker:
            if current_words:
                lines.append(f"**{current_speaker}:** {' '.join(current_words)}")
            current_speaker = name
            current_words = [text]
        else:
            current_words.append(text)

    if current_words and current_speaker:
        lines.append(f"**{current_speaker}:** {' '.join(current_words)}")

    return "\n\n".join(lines) if lines else "[no speech detected in segments]"


def append_silence_log(timestamp, classification):
    with SILENCE_LOG.open("a", encoding="utf-8") as file:
        file.write(
            f"- {timestamp}: silence discarded "
            f"(duration={classification['duration_seconds']}s, rms={classification['rms']})\n"
        )


def load_conversation_state():
    if not CONVERSATION_STATE.exists():
        return {}
    try:
        return json.loads(CONVERSATION_STATE.read_text(encoding="utf-8"))
    except Exception:
        return {}


def save_conversation_state(state):
    CONVERSATION_STATE.write_text(json.dumps(state, indent=2), encoding="utf-8")


def should_start_conversation(state, timestamp, chunk_type):
    if chunk_type == "speech" and not state.get("active_id"):
        return True
    last_activity = state.get("last_activity")
    if not last_activity:
        return chunk_type == "speech"

    gap = (parse_timestamp(timestamp) - parse_timestamp(last_activity)).total_seconds()
    return gap > CONVERSATION_GAP_SECONDS


def update_conversation(timestamp, chunk_type, final_audio, classification, transcript, speaker):
    if chunk_type not in {"speech", "ambient"}:
        return None

    state = load_conversation_state()
    if should_start_conversation(state, timestamp, chunk_type):
        conversation_id = f"conversation_{timestamp}"
        conversation_path = CONVERSATION_DIR / f"{conversation_id}.md"
        state = {
            "active_id": conversation_id,
            "active_path": str(conversation_path),
            "started_at": timestamp,
            "last_activity": timestamp,
        }
        with conversation_path.open("w", encoding="utf-8") as file:
            file.write(f"# Moto X Conversation - {timestamp}\n\n")
    else:
        conversation_path = Path(state["active_path"])
        state["last_activity"] = timestamp

    with conversation_path.open("a", encoding="utf-8") as file:
        if chunk_type == "speech":
            file.write(f"## {timestamp}\n\n")
            file.write(transcript.strip() + "\n\n")
        else:
            file.write(f"<!-- {timestamp}: ambient audio kept; rms={classification.get('rms')} -->\n\n")

    save_conversation_state(state)
    return {
        "id": state["active_id"],
        "path": state["active_path"],
        "started_at": state["started_at"],
        "last_activity": state["last_activity"],
    }


def write_transcript(
    timestamp,
    chunk_type,
    final_audio,
    classification,
    transcript,
    whisper_info,
    speaker,
    conversation_info,
):
    md_path = TRANSCRIPT_DIR / f"{timestamp}_{chunk_type}.md"
    title = "Moto X speech transcript" if chunk_type == "speech" else "Moto X ambient audio"
    with md_path.open("w", encoding="utf-8") as file:
        file.write(f"# {title} - {timestamp}\n\n")
        if chunk_type == "speech":
            file.write(transcript.strip() + "\n")
        else:
            file.write(
                "Ambient audio was kept for review. "
                f"RMS: {classification.get('rms')}; "
                f"speech seconds: {classification.get('speech_seconds')}.\n"
            )
    return md_path


def _ingest_to_gateway(path: Path, project: str = "transcripts"):
    """Fire-and-forget: ingest a markdown file into the Chimera Gateway (RAG).
    Runs in a daemon thread so it never blocks the recording pipeline.
    Silently no-ops if the gateway is not running.
    """
    try:
        text = path.read_text(encoding="utf-8", errors="replace")
        if not text.strip():
            return
        content_hash = "sha256:" + hashlib.sha256(text.encode()).hexdigest()
        with httpx.Client(timeout=30) as client:
            resp = client.post(
                f"{GATEWAY_URL}/v1/ingest",
                headers={
                    "Authorization":     f"Bearer {GATEWAY_TOKEN}",
                    "Content-Type":      "application/json",
                    "X-Chimera-Project": project,
                },
                json={
                    "text":         text,
                    "source":       path.name,
                    "content_hash": content_hash,
                },
            )
            if resp.status_code == 200:
                chunks = resp.json().get("chunks", "?")
                receiver_log(f"OK gateway ingested {path.name} -> {chunks} chunks [{project}]")
            else:
                body = (resp.text or "")[:200]
                receiver_log(f"ERROR gateway ingest HTTP {resp.status_code} for {path.name}: {body}")
    except Exception as exc:
        receiver_log(f"ERROR gateway ingest failed ({path.name}): {exc}")


def ingest_if_gateway(path: Path, project: str = "transcripts"):
    """Schedule a background ingest if gateway ingest is enabled."""
    if GATEWAY_INGEST_ENABLED:
        threading.Thread(
            target=_ingest_to_gateway,
            args=(path, project),
            daemon=True,
        ).start()


DASHBOARD_HTML = r"""<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width,initial-scale=1,viewport-fit=cover">
  <meta name="theme-color" content="#000000">
  <meta name="color-scheme" content="dark">
  <meta name="apple-mobile-web-app-capable" content="yes">
  <meta name="apple-mobile-web-app-status-bar-style" content="black-translucent">
  <meta name="apple-mobile-web-app-title" content="Claudia">
  <link rel="manifest" href="/dashboard-assets/manifest.webmanifest">
  <link rel="icon" type="image/png" sizes="32x32" href="/dashboard-assets/favicon-32.png">
  <link rel="apple-touch-icon" sizes="180x180" href="/dashboard-assets/apple-touch-icon.png">
  <title>Claudia</title>
  <style>
    :root { color-scheme: dark; font-family: system-ui, sans-serif; background: #000; }
    html { background: #000; }
    body { margin: 0; background: #000; color: #ddd8cf; min-height: 100vh; }
    main { max-width: 34rem; margin: auto; padding: 8vh 1.25rem 3rem; }
    header { display: flex; align-items: center; gap: .65rem; color: #a99f91; }
    #dot { width: .55rem; height: .55rem; border-radius: 50%; background: #777; }
    #dot.healthy { background: #78b892; box-shadow: 0 0 .6rem #78b89277; }
    #dot.late { background: #c9a55b; }
    #dot.stale { background: #b96b68; }
    .muted { color: #776f65; font-size: .82rem; }
    .top-actions { display: flex; gap: .5rem; margin: 1.15rem 0 1.4rem; }
    .top-actions a { flex: 1; text-align: center; }
    #recent { display: flex; flex-direction: column; gap: .65rem; margin: 1rem 0 3rem; }
    .bubble { box-sizing: border-box; width: fit-content; max-width: 88%; padding: .72rem .82rem; border: 1px solid #332c37; border-radius: 1rem 1rem 1rem .3rem; background: #19161c; }
    .bubble.speaker-ruby { align-self: flex-end; border-color: #68487a; border-radius: 1rem 1rem .3rem 1rem; background: linear-gradient(145deg, #392445, #2b1c35); }
    .bubble.speaker-lynn { align-self: flex-start; border-color: #37644f; background: linear-gradient(145deg, #1e3a2d, #182d24); }
    .bubble.speaker-raven { align-self: flex-start; border-color: #744936; background: linear-gradient(145deg, #3d271f, #2f1e19); }
    .bubble-meta { display: flex; justify-content: space-between; gap: .8rem; margin-bottom: .3rem; font-size: .7rem; }
    .speaker-trigger { color: #d7a9f1; }
    .speaker-lynn .speaker-trigger { color: #8fd6ad; }
    .speaker-raven .speaker-trigger { color: #e3a27e; }
    .speaker-unsorted .speaker-trigger, .speaker-mixed-voices .speaker-trigger { color: #aaa1ad; }
    .bubble-meta time { color: #776f7b; white-space: nowrap; }
    .speaker-picker { position: relative; }
    .speaker-trigger { appearance: none; border: 0; border-radius: .5rem; background: transparent; font: inherit; font-weight: 700; padding: .18rem .28rem; margin: -.18rem -.28rem; cursor: pointer; }
    .speaker-trigger:focus-visible { outline: 2px solid currentColor; outline-offset: 2px; }
    .speaker-menu { position: absolute; z-index: 5; top: calc(100% + .35rem); left: -.3rem; min-width: 8.5rem; padding: .32rem; border: 1px solid #504655; border-radius: .75rem; background: #17131b; box-shadow: 0 .7rem 2rem #000b; }
    .speaker-ruby .speaker-menu { left: auto; right: -.3rem; }
    .speaker-menu[hidden] { display: none; }
    .speaker-menu button { display: block; width: 100%; padding: .58rem .65rem; border: 0; border-radius: .5rem; background: transparent; color: #ddd8cf; font: inherit; text-align: left; cursor: pointer; }
    .speaker-menu button:hover, .speaker-menu button:focus-visible { background: #302936; outline: none; }
    .bubble p { margin: 0; line-height: 1.42; overflow-wrap: anywhere; }
    footer { display: grid; gap: .3rem; text-align: center; }
    a { box-sizing: border-box; color: #aaa195; text-decoration: none; border: 1px solid #302d29; padding: .55rem .75rem; border-radius: 999px; }
    @media (max-width: 420px) {
      .top-actions { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); }
      .top-actions a { display: block; padding: .6rem .45rem; white-space: nowrap; }
    }
  </style>
</head>
<body><main>
  <header><span id="dot"></span><span id="health">waiting for a capture</span></header>
  <nav class="top-actions"><a href="/review">teach Claudia</a><a id="journal" href="#">today's journal</a></nav>
  <section id="recent"><p class="muted">Recent words will appear here.</p></section>
  <footer><span class="muted" id="counts">No chunks today yet</span><span class="muted" id="updated"></span></footer>
</main>
<script>
const escapeHtml = value => String(value).replace(/[&<>'"]/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;',"'":'&#39;','"':'&quot;'}[c]));
const speakerClass = value => String(value || 'Unsorted').toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '') || 'unsorted';
const renderRecent = rows => rows.flatMap(row => (row.segments || []).map(segment => {
  const offset = Number(segment.audio_start_seconds);
  const end = Number(segment.audio_end_seconds);
  const time = row.captured_at.slice(11).replaceAll('-', ':') + (Number.isFinite(offset) && offset > 0 ? ` +${offset.toFixed(1)}s` : '');
  return `<article class="bubble speaker-${speakerClass(segment.speaker)}" data-speaker-bubble><div class="bubble-meta"><span class="speaker-picker"><button class="speaker-trigger" type="button" aria-expanded="false" data-capture="${escapeHtml(row.capture_id)}" data-start="${Number.isFinite(offset) ? offset : 0}" data-end="${Number.isFinite(end) ? end : Number(row.duration_seconds || 30)}" data-speaker="${escapeHtml(segment.speaker)}">${escapeHtml(segment.speaker)} ▾</button><span class="speaker-menu" hidden><button type="button" data-quick-speaker="Unsorted">Unsorted</button><button type="button" data-quick-speaker="Ruby">Ruby</button><button type="button" data-quick-speaker="Lynn">Lynn</button></span></span><time>${escapeHtml(time)}</time></div><p>${escapeHtml(segment.text)}</p></article>`;
})).join('');
const closeSpeakerMenus = () => document.querySelectorAll('.speaker-menu:not([hidden])').forEach(menu => {
  menu.hidden = true;
  menu.previousElementSibling?.setAttribute('aria-expanded', 'false');
});
document.addEventListener('click', async event => {
  const choice = event.target.closest('[data-quick-speaker]');
  if (choice) {
    const picker = choice.closest('.speaker-picker');
    const trigger = picker.querySelector('.speaker-trigger');
    const label = choice.dataset.quickSpeaker;
    closeSpeakerMenus();
    if (label === trigger.dataset.speaker) return;
    choice.disabled = true;
    try {
      const response = await fetch('/api/motox/review/quick-speaker', {
        method: 'POST', headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({capture_id: trigger.dataset.capture, audio_start_seconds: Number(trigger.dataset.start), audio_end_seconds: Number(trigger.dataset.end), label})
      });
      const saved = await response.json();
      if (!response.ok) throw new Error(saved.error || 'Could not save speaker');
      const bubble = trigger.closest('[data-speaker-bubble]');
      bubble.className = `bubble speaker-${speakerClass(saved.label)}`;
      trigger.dataset.speaker = saved.label;
      trigger.textContent = `${saved.label} ▾`;
      document.querySelector('#updated').textContent = `${saved.label} saved`;
    } catch (error) {
      document.querySelector('#updated').textContent = error.message;
    } finally {
      choice.disabled = false;
    }
    return;
  }
  const trigger = event.target.closest('.speaker-trigger');
  if (trigger) {
    const menu = trigger.nextElementSibling;
    const opening = menu.hidden;
    closeSpeakerMenus();
    menu.hidden = !opening;
    trigger.setAttribute('aria-expanded', String(opening));
    return;
  }
  if (!event.target.closest('.speaker-picker')) closeSpeakerMenus();
});
document.addEventListener('scroll', closeSpeakerMenus, {passive: true, capture: true});
async function refresh() {
  try {
    const [status, recent] = await Promise.all([
      fetch('/api/motox/status').then(r => r.json()),
      fetch('/api/motox/recent?limit=8').then(r => r.json())
    ]);
    const dot = document.querySelector('#dot');
    dot.className = status.capture_health;
    document.querySelector('#health').textContent = status.capture_health === 'healthy' ? 'capture is healthy' : `capture is ${status.capture_health}`;
    const c = status.today_counts || {};
    const queue = status.recorder ? ` · ${status.recorder.queue_depth} queued` : '';
    document.querySelector('#counts').textContent = `${c.speech || 0} speech · ${c.ambient || 0} ambient · ${c.silence || 0} silent${queue}`;
    document.querySelector('#journal').href = '/journal/recent';
    document.querySelector('#recent').innerHTML = recent.length ? renderRecent([...recent].reverse()) : '<p class="muted">Recent words will appear here.</p>';
    document.querySelector('#updated').textContent = `updated ${new Date().toLocaleTimeString([], {hour:'numeric', minute:'2-digit'})}`;
  } catch (_) {
    document.querySelector('#health').textContent = 'receiver unavailable';
    document.querySelector('#dot').className = 'stale';
  }
}
refresh(); setInterval(refresh, 10000);
</script></body></html>"""


@APP.route("/dashboard-assets/<path:filename>", methods=["GET"])
def dashboard_asset(filename):
    max_age = 0 if filename in {"review.js", "review.css", "manifest.webmanifest"} else 86400
    response = send_from_directory(DASHBOARD_ASSET_DIR, filename, max_age=max_age)
    if max_age == 0:
        response.cache_control.no_store = True
    return response


@APP.route("/dashboard", methods=["GET"])
def dashboard():
    return Response(DASHBOARD_HTML, mimetype="text/html")


@APP.route("/review", methods=["GET"])
def motox_review_page():
    response = send_from_directory(DASHBOARD_ASSET_DIR, "review.html", max_age=0)
    response.cache_control.no_store = True
    return response


@APP.route("/journal/<day>", methods=["GET"])
def motox_journal_page(day):
    if V1_STORE is None:
        return "Moto X v1 journal is disabled", 503
    try:
        recent = day == "recent"
        now = datetime.now()
        content = V1_STORE.recent_journal_text(now) if recent else V1_STORE.journal_text(day)
        if REVIEW_STORE:
            prediction_days = (
                [(now - timedelta(days=1)).strftime("%Y-%m-%d"), now.strftime("%Y-%m-%d")]
                if recent else [day]
            )
            predictions = REVIEW_STORE.journal_speaker_predictions(prediction_days)
        else:
            predictions = {}
    except ValueError:
        return "invalid date; expected YYYY-MM-DD", 400
    speaker_layers = {}
    for capture_id, prediction in predictions.items():
        audio_path = REVIEW_STORE.get_audio_path(capture_id)
        if audio_path and audio_path.name in content:
            speaker_layers[audio_path.name] = prediction
    return Response(
        render_journal_html(
            content,
            "Last 24 hours" if recent else day,
            speaker_layers,
            source_path="/api/motox/journal/recent" if recent else None,
            page=max(1, request.args.get("page", 1, type=int) or 1),
            page_size=12 if recent else None,
            page_path="/journal/recent" if recent else None,
        ),
        mimetype="text/html",
    )


@APP.route("/api/motox/journal-audio/<filename>", methods=["GET"])
def motox_journal_audio(filename):
    if Path(filename).name != filename or Path(filename).suffix.lower() not in ALLOWED_SUFFIXES:
        return "audio not found", 404
    audio_path = AUDIO_DIR / filename
    if not audio_path.is_file():
        return "audio not found", 404
    try:
        audio_path.resolve().relative_to(AUDIO_DIR.resolve())
    except ValueError:
        return "audio not found", 404
    return send_file(audio_path, conditional=True, download_name=audio_path.name)


@APP.route("/api/motox/status", methods=["GET"])
def motox_status():
    if V1_STORE is None:
        return jsonify({"error": "Moto X v1 journal is disabled"}), 503
    return jsonify(V1_STORE.status())


@APP.route("/api/motox/recent", methods=["GET"])
def motox_recent():
    if V1_STORE is None:
        return jsonify([])
    try:
        limit = int(request.args.get("limit", "8"))
    except ValueError:
        limit = 8
    rows = V1_STORE.recent_transcript(limit)
    predictions = {}
    active_speakers = {}
    if REVIEW_STORE and rows:
        days = sorted({row["captured_at"][:10] for row in rows})
        predictions = REVIEW_STORE.journal_speaker_predictions(days)
        active_speakers = REVIEW_STORE.active_speaker_annotations(
            [row["capture_id"] for row in rows]
        )
    for row in rows:
        layer = predictions.get(row["capture_id"], {})
        segments = list(layer.get("segments") or [])
        if not segments:
            text = re.sub(r"^\*\*[^*]+:\*\*\s*", "", row["transcript"]).strip()
            duration = max(0.0, float(row.get("duration_seconds") or 30.0))
            speaker = "Unsorted"
            for annotation in active_speakers.get(row["capture_id"], []):
                start = annotation.get("audio_start_seconds")
                end = annotation.get("audio_end_seconds")
                coverage = 1.0 if start is None or end is None else (
                    max(0.0, float(end) - float(start)) / duration if duration else 0.0
                )
                if coverage >= 0.65:
                    speaker = annotation["label"]
            segments = [{
                "speaker": speaker,
                "source": "confirmed" if speaker != "Unsorted" else "unassigned",
                "text": text,
                "audio_start_seconds": 0.0,
                "audio_end_seconds": duration,
            }]
        row["segments"] = segments
    return jsonify(rows)


@APP.route("/api/motox/review/quick-speaker", methods=["POST"])
def motox_review_quick_speaker():
    if REVIEW_STORE is None:
        return jsonify({"error": "Moto X review is disabled"}), 503
    payload = request.get_json(silent=True) or {}
    capture_id = str(payload.get("capture_id", ""))
    if not CAPTURE_ID_RE.fullmatch(capture_id):
        return jsonify({"error": "invalid capture id"}), 400
    label = payload.get("label")
    if label == "Unsorted":
        label = None
    try:
        result = REVIEW_STORE.set_quick_speaker_label(
            capture_id=capture_id,
            audio_start_seconds=payload.get("audio_start_seconds"),
            audio_end_seconds=payload.get("audio_end_seconds"),
            label=label,
        )
        return jsonify(result)
    except KeyError:
        return jsonify({"error": "unknown capture"}), 404
    except (TypeError, ValueError) as exc:
        return jsonify({"error": str(exc)}), 400


@APP.route("/api/motox/review/candidates", methods=["GET"])
def motox_review_candidates():
    if REVIEW_STORE is None:
        return jsonify({"error": "Moto X review is disabled"}), 503
    try:
        limit = int(request.args.get("limit", "24"))
        target_seconds = float(request.args.get("target_seconds", "300"))
        before = request.args.get("before") or None
        group = request.args.get("group") or None
        if before is not None and not CAPTURE_TIMESTAMP_RE.fullmatch(before):
            raise ValueError("invalid before timestamp")
        return jsonify(
            REVIEW_STORE.candidates(
                limit=limit,
                target_seconds=target_seconds,
                before=before,
                group=group,
            )
        )
    except (TypeError, ValueError) as exc:
        return jsonify({"error": str(exc)}), 400


@APP.route("/api/motox/review/progress", methods=["GET"])
def motox_review_progress():
    if REVIEW_STORE is None:
        return jsonify({"error": "Moto X review is disabled"}), 503
    return jsonify(REVIEW_STORE.progress())


@APP.route("/api/motox/review/identification", methods=["GET"])
def motox_review_identification():
    if REVIEW_STORE is None:
        return jsonify({"error": "Moto X review is disabled"}), 503
    try:
        limit = int(request.args.get("limit", "24"))
        excluded = {
            value for value in request.args.getlist("exclude")
            if re.fullmatch(r"[a-f0-9]{32}", value)
        }
        excluded_captures = {
            value for value in request.args.getlist("exclude_capture")
            if CAPTURE_ID_RE.fullmatch(value)
        }
        return jsonify(
            REVIEW_STORE.identification_candidates(
                limit=limit,
                exclude_turn_ids=excluded,
                exclude_capture_ids=excluded_captures,
            )
        )
    except (TypeError, ValueError) as exc:
        return jsonify({"error": str(exc)}), 400


@APP.route("/api/motox/review/context/<capture_id>", methods=["GET"])
def motox_review_context(capture_id):
    if REVIEW_STORE is None or not CAPTURE_ID_RE.fullmatch(capture_id):
        return jsonify({"error": "review context not found"}), 404
    try:
        radius = int(request.args.get("radius", "2"))
        return jsonify(REVIEW_STORE.context(capture_id, radius))
    except KeyError:
        return jsonify({"error": "unknown capture"}), 404
    except (TypeError, ValueError) as exc:
        return jsonify({"error": str(exc)}), 400


@APP.route("/api/motox/review/capture/<capture_id>", methods=["GET"])
def motox_review_capture(capture_id):
    if REVIEW_STORE is None or not CAPTURE_ID_RE.fullmatch(capture_id):
        return jsonify({"error": "review capture not found"}), 404
    try:
        items = REVIEW_STORE.context(capture_id, 1)
        item = next(row for row in items if row["capture_id"] == capture_id)
        return jsonify(item)
    except (KeyError, StopIteration):
        return jsonify({"error": "unknown capture"}), 404


@APP.route("/api/motox/review/groups", methods=["GET"])
def motox_review_groups():
    if REVIEW_STORE is None:
        return jsonify({"error": "Moto X review is disabled"}), 503
    return jsonify(REVIEW_STORE.groups())


@APP.route("/api/motox/review/audio/<capture_id>", methods=["GET"])
def motox_review_audio(capture_id):
    if REVIEW_STORE is None or not CAPTURE_ID_RE.fullmatch(capture_id):
        return "audio not found", 404
    audio_path = REVIEW_STORE.get_audio_path(capture_id)
    if audio_path is None:
        return "audio not found", 404
    try:
        audio_path.resolve().relative_to(AUDIO_DIR.resolve())
    except ValueError:
        return "audio not found", 404
    return send_file(audio_path, conditional=True, download_name=audio_path.name)


@APP.route("/api/motox/review/annotations", methods=["POST"])
def motox_review_annotation():
    if REVIEW_STORE is None:
        return jsonify({"error": "Moto X review is disabled"}), 503
    payload = request.get_json(silent=True) or {}
    capture_id = str(payload.get("capture_id", ""))
    if not CAPTURE_ID_RE.fullmatch(capture_id):
        return jsonify({"error": "invalid capture id"}), 400
    try:
        speaker_embedding = None
        speaker_embedding_model = None
        speaker_embedding_version = None
        if (
            str(payload.get("annotation_type", "")).strip().lower() == "speaker"
            and payload.get("label") in ("Ruby", "Lynn", "Raven")
            and payload.get("speaker_turn_id")
            and payload.get("audio_start_seconds") is not None
            and payload.get("audio_end_seconds") is not None
        ):
            (
                speaker_embedding,
                speaker_embedding_model,
                speaker_embedding_version,
            ) = extract_review_speaker_embedding(
                capture_id,
                float(payload["audio_start_seconds"]),
                float(payload["audio_end_seconds"]),
            )
        annotation = REVIEW_STORE.add_annotation(
            capture_id=capture_id,
            start_char=int(payload.get("start_char", 0)),
            end_char=int(payload.get("end_char", 0)),
            annotation_type=str(payload.get("annotation_type", "")),
            label=payload.get("label"),
            replacement_text=payload.get("replacement_text"),
            audio_start_seconds=payload.get("audio_start_seconds"),
            audio_end_seconds=payload.get("audio_end_seconds"),
            speaker_turn_id=payload.get("speaker_turn_id"),
            speaker_embedding=speaker_embedding,
            speaker_embedding_model=speaker_embedding_model,
            speaker_embedding_version=speaker_embedding_version,
        )
        return jsonify(annotation), 201
    except KeyError:
        return jsonify({"error": "unknown capture"}), 404
    except (TypeError, ValueError) as exc:
        return jsonify({"error": str(exc)}), 400
    except RuntimeError as exc:
        return jsonify({"error": str(exc)}), 503


@APP.route("/api/motox/review/annotations/<annotation_id>", methods=["DELETE"])
def motox_review_annotation_revert(annotation_id):
    if REVIEW_STORE is None:
        return jsonify({"error": "Moto X review is disabled"}), 503
    if not re.fullmatch(r"[a-f0-9]{32}", annotation_id):
        return jsonify({"error": "invalid annotation id"}), 400
    if not REVIEW_STORE.revert_annotation(annotation_id):
        return jsonify({"error": "active annotation not found"}), 404
    return jsonify({"ok": True})


@APP.route("/api/motox/recorder-status", methods=["POST"])
def motox_recorder_status():
    if V1_STORE is None:
        return jsonify({"error": "Moto X v1 journal is disabled"}), 503
    payload = request.get_json(silent=True) or {}
    try:
        reported_at = safe_capture_timestamp(
            payload.get("reported_at"), timestamp_now()
        )
        queue_depth = max(0, int(payload.get("queue_depth", 0)))
        cycle = payload.get("cycle_seconds")
        cycle_seconds = float(cycle) if cycle is not None else None
        V1_STORE.update_recorder_status(
            reported_at,
            str(payload.get("recorder", "unknown"))[:40],
            queue_depth,
            cycle_seconds,
        )
    except (TypeError, ValueError):
        return jsonify({"error": "invalid recorder status"}), 400
    return jsonify({"ok": True})


@APP.route("/api/motox/journal/<day>", methods=["GET"])
def motox_journal(day):
    if V1_STORE is None:
        return "Moto X v1 journal is disabled", 503
    try:
        content = V1_STORE.recent_journal_text() if day == "recent" else V1_STORE.journal_text(day)
    except ValueError:
        return "invalid date; expected YYYY-MM-DD", 400
    return Response(content, mimetype="text/markdown")


@APP.route("/ping", methods=["GET"])
def ping():
    return "ok", 200


@APP.route("/transcribe", methods=["POST"])
def receive_audio():
    upload = request.files.get("audio")
    if not upload:
        return "no audio", 400

    received_timestamp = timestamp_now()
    legacy_identity = legacy_capture_identity(upload.filename)
    legacy_capture_id = legacy_identity[0] if legacy_identity else None
    legacy_captured_at = legacy_identity[1] if legacy_identity else None
    captured_at = safe_capture_timestamp(
        request.form.get("captured_at") or legacy_captured_at, received_timestamp
    )
    capture_id = safe_capture_id(
        request.form.get("capture_id") or legacy_capture_id, captured_at
    )
    if V1_STORE is not None and V1_STORE.has_chunk(capture_id):
        receiver_log(f"[chunk {capture_id}] duplicate upload acknowledged")
        return "already received", 200

    speaker = request.form.get("speaker", DEFAULT_SPEAKER).strip() or DEFAULT_SPEAKER
    suffix = safe_suffix(upload.filename)
    incoming_path = INBOX_DIR / f"{capture_id}_upload{suffix}"
    wav_path = WORK_DIR / f"{capture_id}_16k.wav"
    upload.save(incoming_path)

    try:
        conversion = run_ffmpeg_to_wav(incoming_path, wav_path)
    except FileNotFoundError:
        error_path = ERROR_DIR / incoming_path.name
        incoming_path.replace(error_path)
        message = (
            "ffmpeg was not found on the PC. Install ffmpeg or set "
            "MOTOX_FFMPEG_BIN to the full path of ffmpeg.exe."
        )
        receiver_log(f"[chunk {capture_id}] {message}")
        return message, 500
    if conversion.returncode != 0 or not wav_path.exists() or wav_path.stat().st_size < 1000:
        error_path = ERROR_DIR / incoming_path.name
        incoming_path.replace(error_path)
        if wav_path.exists():
            wav_path.unlink()
        receiver_log(f"[chunk {capture_id}] ffmpeg failed: {conversion.stderr.strip()}")
        return f"ffmpeg failed: {conversion.stderr.strip()}", 422

    wav_tensor = None
    try:
        wav_tensor = read_pcm16_wav(wav_path)
        classification = classify_chunk_from_tensor(wav_tensor)
    except Exception as exc:
        classification = {
            "type": "speech",
            "duration_seconds": None,
            "speech_seconds": None,
            "rms": None,
            "speech_regions": None,
            "vad_error": str(exc),
        }

    chunk_type = classification["type"]
    receiver_log(f"[chunk {capture_id}] classified as {chunk_type.upper()} {classification}")

    if chunk_type == "silence":
        record_v1_chunk(
            capture_id, captured_at, chunk_type, classification, None, "", speaker
        )
        incoming_path.unlink(missing_ok=True)
        wav_path.unlink(missing_ok=True)
        append_silence_log(captured_at, classification)
        return "silence discarded", 200

    final_audio = AUDIO_DIR / f"{capture_id}_{chunk_type}{suffix}"
    incoming_path.replace(final_audio)

    if chunk_type == "speech":
        transcript, whisper_info, whisper_segments = transcribe_audio(final_audio)

        # Run diarization using the pre-loaded tensor (avoids torchcodec on Windows)
        diarization_segments = diarize_audio(wav_tensor) if wav_tensor is not None else []
        if diarization_segments and whisper_segments:
            speaker_mapping = map_speakers(diarization_segments)
            transcript = format_diarized_transcript(
                whisper_segments, diarization_segments, speaker_mapping
            )
            receiver_log(f"[chunk {capture_id}] diarized: {list(speaker_mapping.values())}")
        else:
            transcript = f"**{speaker}:** {transcript.strip()}"
    else:
        transcript = "[ambient audio kept for review: traffic, room tone, music, or other non-silent context]"
        whisper_info = {}

    conversation_info = update_conversation(
        captured_at,
        chunk_type,
        final_audio,
        classification,
        transcript,
        speaker,
    )
    md_path = write_transcript(
        capture_id,
        chunk_type,
        final_audio,
        classification,
        transcript,
        whisper_info,
        speaker,
        conversation_info,
    )
    record_v1_chunk(
        capture_id,
        captured_at,
        chunk_type,
        classification,
        final_audio,
        transcript,
        speaker,
    )
    if chunk_type == "speech" and V1_STORE is not None and whisper_info.get("timed_text"):
        try:
            V1_STORE.record_transcription_pass(
                capture_id,
                whisper_info["timed_text"],
                whisper_info.get("words", []),
                pass_kind="quick_chunk",
                model_name="faster-whisper",
                model_version=WHISPER_MODEL,
            )
        except Exception as exc:
            receiver_log(f"[chunk {capture_id}] word timestamp storage error: {exc}")
    wav_path.unlink(missing_ok=True)

    receiver_log(f"[chunk {capture_id}] OK saved audio: {final_audio}")
    receiver_log(f"[chunk {capture_id}] OK transcript: {md_path}")
    if conversation_info:
        conv_path = Path(conversation_info["path"])
        receiver_log(f"[chunk {capture_id}] OK conversation: {conv_path}")
        ingest_if_gateway(conv_path, "transcripts")

    return "ok", 200


def shutdown(sig, frame):
    receiver_log("Receiver shutting down (SIGINT).")
    receiver_log(f"Files saved under: {BASE_DIR}")
    sys.exit(0)


signal.signal(signal.SIGINT, shutdown)

if __name__ == "__main__":
    host = os.environ.get("MOTOX_RECEIVER_HOST", "0.0.0.0")
    port = int(os.environ.get("MOTOX_RECEIVER_PORT", "8765"))
    receiver_log(f"Receiver starting on {host}:{port} — log file: {RECEIVER_LOG}")
    receiver_log(f"Saving audio/transcripts under: {BASE_DIR}")
    APP.run(host=host, port=port, threaded=True)
