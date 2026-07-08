import hashlib
import json
import os
import signal
import subprocess
import sys
import threading
import wave
from datetime import datetime
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
from flask import Flask, request


APP = Flask(__name__)

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
PRIMARY_SPEAKER = os.environ.get("MOTOX_PRIMARY_SPEAKER", "Ruby")
UNKNOWN_SPEAKER = os.environ.get("MOTOX_UNKNOWN_SPEAKER", "friend")
ENABLE_DIARIZATION = os.environ.get("MOTOX_DIARIZATION", "0") == "1"

for directory in (INBOX_DIR, AUDIO_DIR, TRANSCRIPT_DIR, CONVERSATION_DIR, WORK_DIR, ERROR_DIR):
    directory.mkdir(parents=True, exist_ok=True)

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
        )
        segments = list(segments_gen)
        text = " ".join(s.text.strip() for s in segments if s.text.strip()).strip()
        return (
            text or "[speech detected, but no transcript text returned]",
            {
                "language": getattr(info, "language", None),
                "language_probability": getattr(info, "language_probability", None),
            },
            segments,
        )
    except ImportError:
        return "[transcription unavailable: install faster-whisper]", {}, []
    except Exception as exc:
        return f"[transcription error: {exc}]", {}, []


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
    """Map pyannote speaker IDs to human names.

    Heuristic: the speaker with the most total speech time is PRIMARY_SPEAKER
    (Ruby), since she is wearing the lav mic. Others get UNKNOWN_SPEAKER
    (friend) if only one other speaker, or friend_1 / friend_2 etc. if several.
    """
    if not diarization_segments:
        return {}

    speech_time = {}
    for start, end, sp in diarization_segments:
        speech_time[sp] = speech_time.get(sp, 0.0) + (end - start)

    sorted_speakers = sorted(speech_time, key=lambda s: -speech_time[s])
    n_others = len(sorted_speakers) - 1

    mapping = {sorted_speakers[0]: PRIMARY_SPEAKER}
    for i, sp in enumerate(sorted_speakers[1:], 1):
        mapping[sp] = f"friend_{i}" if n_others > 1 else UNKNOWN_SPEAKER

    return mapping


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
                receiver_log(f"✓ gateway ingested {path.name} → {chunks} chunks [{project}]")
            else:
                body = (resp.text or "")[:200]
                receiver_log(f"✗ gateway ingest HTTP {resp.status_code} for {path.name}: {body}")
    except Exception as exc:
        receiver_log(f"✗ gateway ingest failed ({path.name}): {exc}")


def ingest_if_gateway(path: Path, project: str = "transcripts"):
    """Schedule a background ingest if gateway ingest is enabled."""
    if GATEWAY_INGEST_ENABLED:
        threading.Thread(
            target=_ingest_to_gateway,
            args=(path, project),
            daemon=True,
        ).start()


@APP.route("/ping", methods=["GET"])
def ping():
    return "ok", 200


@APP.route("/transcribe", methods=["POST"])
def receive_audio():
    upload = request.files.get("audio")
    if not upload:
        return "no audio", 400

    timestamp = timestamp_now()
    speaker = request.form.get("speaker", DEFAULT_SPEAKER).strip() or DEFAULT_SPEAKER
    suffix = safe_suffix(upload.filename)
    incoming_path = INBOX_DIR / f"{timestamp}_upload{suffix}"
    wav_path = WORK_DIR / f"{timestamp}_16k.wav"
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
        receiver_log(f"[chunk {timestamp}] {message}")
        return message, 500
    if conversion.returncode != 0 or not wav_path.exists() or wav_path.stat().st_size < 1000:
        error_path = ERROR_DIR / incoming_path.name
        incoming_path.replace(error_path)
        if wav_path.exists():
            wav_path.unlink()
        receiver_log(f"[chunk {timestamp}] ffmpeg failed: {conversion.stderr.strip()}")
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
    receiver_log(f"[chunk {timestamp}] classified as {chunk_type.upper()} {classification}")

    if chunk_type == "silence":
        incoming_path.unlink(missing_ok=True)
        wav_path.unlink(missing_ok=True)
        append_silence_log(timestamp, classification)
        return "silence discarded", 200

    final_audio = AUDIO_DIR / f"{timestamp}_{chunk_type}{suffix}"
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
            receiver_log(f"[chunk {timestamp}] diarized: {list(speaker_mapping.values())}")
        else:
            transcript = f"**{speaker}:** {transcript.strip()}"
    else:
        transcript = "[ambient audio kept for review: traffic, room tone, music, or other non-silent context]"
        whisper_info = {}

    conversation_info = update_conversation(
        timestamp,
        chunk_type,
        final_audio,
        classification,
        transcript,
        speaker,
    )
    md_path = write_transcript(
        timestamp,
        chunk_type,
        final_audio,
        classification,
        transcript,
        whisper_info,
        speaker,
        conversation_info,
    )
    wav_path.unlink(missing_ok=True)

    receiver_log(f"[chunk {timestamp}] ✓ saved audio: {final_audio}")
    receiver_log(f"[chunk {timestamp}] ✓ transcript: {md_path}")
    if conversation_info:
        conv_path = Path(conversation_info["path"])
        receiver_log(f"[chunk {timestamp}] ✓ conversation: {conv_path}")
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
