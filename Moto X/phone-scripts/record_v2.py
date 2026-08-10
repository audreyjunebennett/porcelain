"""Moto X recorder v2: durable capture with background delivery.

This is intentionally a separate entry point from record.py.  Syncthing may
copy it to the phone safely; it does nothing until Ruby launches it manually.
"""

import json
import os
import signal
import subprocess
import sys
import threading
import time
import uuid
from datetime import datetime

import requests


SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
PC_BASE_URL = os.environ.get(
    "MOTOX_PC_URL", "https://sunset.tailfa86ac.ts.net"
)
TRANSCRIBE_URL = f"{PC_BASE_URL.rstrip('/')}/transcribe"
PING_URL = f"{PC_BASE_URL.rstrip('/')}/ping"
STATUS_URL = f"{PC_BASE_URL.rstrip('/')}/api/motox/recorder-status"
CHUNK_SECONDS = int(os.environ.get("MOTOX_CHUNK_SECONDS", "30"))
QUEUE_DIR = os.environ.get(
    "MOTOX_V2_QUEUE_DIR", os.path.join(SCRIPT_DIR, "motox_v2_queue")
)
PROCESS_LOCK = os.environ.get(
    "MOTOX_V2_LOCK", os.path.join(SCRIPT_DIR, "motox_record_v2.lock")
)
RECORD_LOG = os.environ.get(
    "MOTOX_V2_RECORD_LOG", os.path.join(SCRIPT_DIR, "motox_record_v2_log.md")
)
SAMPLE_RATE = os.environ.get("MOTOX_SAMPLE_RATE", "44100")
BITRATE = os.environ.get("MOTOX_AUDIO_BITRATE", "128000")
SPEAKER_LABEL = os.environ.get("MOTOX_SPEAKER", "Ruby")
CONNECT_TIMEOUT = int(os.environ.get("MOTOX_CONNECT_TIMEOUT", "10"))
UPLOAD_TIMEOUT = int(os.environ.get("MOTOX_UPLOAD_TIMEOUT", "180"))

STOP = threading.Event()
LOG_LOCK = threading.Lock()
LAST_CYCLE_SECONDS = None


def timestamp_now():
    return datetime.now().strftime("%Y-%m-%d_%H-%M-%S")


def record_log(message):
    line = f"[{datetime.now().strftime('%Y-%m-%d %H:%M:%S')}] {message}"
    print(line)
    try:
        os.makedirs(os.path.dirname(RECORD_LOG) or ".", exist_ok=True)
        with LOG_LOCK:
            with open(RECORD_LOG, "a", encoding="utf-8") as handle:
                handle.write(line + "\n")
    except OSError:
        pass


def stop_recording():
    subprocess.run(
        ["termux-microphone-record", "-q"], capture_output=True, text=True
    )


def request_stop(sig=None, frame=None):
    record_log("Stopping after the current file is finalized...")
    STOP.set()
    stop_recording()


def process_is_alive(pid):
    if os.name == "nt":
        # Unlike POSIX, Python's os.kill(pid, 0) maps to TerminateProcess on
        # Windows and can kill the very process performing this lock check.
        # A zero-time wait on a process handle is the non-destructive probe.
        import ctypes

        synchronize = 0x00100000
        wait_timeout = 0x00000102
        kernel32 = ctypes.WinDLL("kernel32", use_last_error=True)
        kernel32.OpenProcess.argtypes = [
            ctypes.c_uint32,
            ctypes.c_bool,
            ctypes.c_uint32,
        ]
        kernel32.OpenProcess.restype = ctypes.c_void_p
        kernel32.WaitForSingleObject.argtypes = [ctypes.c_void_p, ctypes.c_uint32]
        kernel32.WaitForSingleObject.restype = ctypes.c_uint32
        kernel32.CloseHandle.argtypes = [ctypes.c_void_p]
        handle = kernel32.OpenProcess(synchronize, False, pid)
        if not handle:
            return False
        try:
            return kernel32.WaitForSingleObject(handle, 0) == wait_timeout
        finally:
            kernel32.CloseHandle(handle)
    try:
        os.kill(pid, 0)
        return True
    except ProcessLookupError:
        return False
    except PermissionError:
        return True


def acquire_process_lock():
    """Prevent two recorder loops from fighting over the single microphone."""
    for _ in range(2):
        try:
            descriptor = os.open(PROCESS_LOCK, os.O_CREAT | os.O_EXCL | os.O_WRONLY)
        except FileExistsError:
            try:
                with open(PROCESS_LOCK, "r", encoding="ascii") as handle:
                    existing_pid = int(handle.read().strip())
            except (OSError, ValueError):
                existing_pid = -1
            if existing_pid > 0 and process_is_alive(existing_pid):
                return False
            try:
                os.remove(PROCESS_LOCK)
            except FileNotFoundError:
                pass
            continue
        with os.fdopen(descriptor, "w", encoding="ascii") as handle:
            handle.write(str(os.getpid()))
        return True
    return False


def release_process_lock():
    try:
        with open(PROCESS_LOCK, "r", encoding="ascii") as handle:
            owner = int(handle.read().strip())
        if owner == os.getpid():
            os.remove(PROCESS_LOCK)
    except (FileNotFoundError, OSError, ValueError):
        pass


def wait_until_file_stable(path, timeout=12):
    started = time.monotonic()
    last_size = -1
    stable_reads = 0
    while time.monotonic() - started < timeout:
        try:
            size = os.path.getsize(path)
        except OSError:
            size = -1
        if size > 1000 and size == last_size:
            stable_reads += 1
            if stable_reads >= 2:
                return True
        else:
            stable_reads = 0
        last_size = size
        time.sleep(0.25)
    return os.path.isfile(path) and os.path.getsize(path) > 1000


def write_ready_metadata(path, metadata):
    temporary = path + ".tmp"
    with open(temporary, "w", encoding="utf-8") as handle:
        json.dump(metadata, handle, sort_keys=True)
        handle.flush()
        os.fsync(handle.fileno())
    os.replace(temporary, path)


def recover_orphan_audio():
    """Make finalized audio from an interrupted run eligible for retry."""
    for filename in sorted(os.listdir(QUEUE_DIR)):
        if not filename.endswith(".aac"):
            continue
        audio_path = os.path.join(QUEUE_DIR, filename)
        metadata_path = audio_path[:-4] + ".json"
        if os.path.exists(metadata_path) or os.path.getsize(audio_path) <= 1000:
            continue
        capture_id = filename[:-4]
        captured_at = capture_id[:19]
        try:
            datetime.strptime(captured_at, "%Y-%m-%d_%H-%M-%S")
        except ValueError:
            captured_at = timestamp_now()
        write_ready_metadata(
            metadata_path,
            {
                "capture_id": capture_id,
                "captured_at": captured_at,
                "speaker": SPEAKER_LABEL,
                "recovered": True,
            },
        )
        record_log(f"Recovered queued capture {capture_id}")


def ready_items():
    try:
        names = sorted(name for name in os.listdir(QUEUE_DIR) if name.endswith(".json"))
    except OSError:
        return []
    return [os.path.join(QUEUE_DIR, name) for name in names]


def post_status():
    try:
        requests.post(
            STATUS_URL,
            json={
                "reported_at": timestamp_now(),
                "recorder": "record_v2",
                "queue_depth": len(ready_items()),
                "cycle_seconds": LAST_CYCLE_SECONDS,
            },
            timeout=8,
        )
    except Exception:
        # Status is advisory. Audio delivery owns the useful outage log and
        # retry behavior, so a heartbeat failure stays quiet.
        pass


def deliver(metadata_path):
    try:
        with open(metadata_path, "r", encoding="utf-8") as handle:
            metadata = json.load(handle)
        audio_path = metadata_path[:-5] + ".aac"
        if not os.path.isfile(audio_path):
            record_log(f"Queue metadata has no audio: {metadata_path}")
            return False
        filename = os.path.basename(audio_path)
        with open(audio_path, "rb") as audio:
            response = requests.post(
                TRANSCRIBE_URL,
                files={"audio": (filename, audio, "audio/aac")},
                data={
                    "capture_id": metadata["capture_id"],
                    "captured_at": metadata["captured_at"],
                    "speaker": metadata.get("speaker", SPEAKER_LABEL),
                },
                timeout=(CONNECT_TIMEOUT, UPLOAD_TIMEOUT),
            )
        if not 200 <= response.status_code < 300:
            record_log(
                f"Delivery HTTP {response.status_code} for {metadata['capture_id']}: "
                f"{response.text[:120]}"
            )
            return False
        os.remove(audio_path)
        os.remove(metadata_path)
        record_log(f"Delivered {metadata['capture_id']} (HTTP {response.status_code})")
        return True
    except Exception as exc:
        record_log(f"Delivery deferred: {exc}")
        return False


def uploader_loop():
    backoff = 2
    last_status = 0.0
    while not STOP.is_set() or ready_items():
        if time.monotonic() - last_status >= 10:
            post_status()
            last_status = time.monotonic()
        items = ready_items()
        if not items:
            STOP.wait(1)
            continue
        if deliver(items[0]):
            backoff = 2
            post_status()
            last_status = time.monotonic()
        else:
            STOP.wait(backoff)
            backoff = min(backoff * 2, 60)


def record_one():
    global LAST_CYCLE_SECONDS
    captured_at = timestamp_now()
    capture_id = f"{captured_at}_{uuid.uuid4().hex[:8]}"
    audio_path = os.path.join(QUEUE_DIR, capture_id + ".aac")
    metadata_path = os.path.join(QUEUE_DIR, capture_id + ".json")
    started = time.monotonic()
    record_log(f"Capture started {capture_id} ({CHUNK_SECONDS}s)")
    result = subprocess.run(
        [
            "termux-microphone-record", "-l", str(CHUNK_SECONDS), "-f", audio_path,
            "-e", "aac", "-r", SAMPLE_RATE, "-c", "1", "-b", BITRATE,
        ],
        capture_output=True,
        text=True,
    )
    if result.returncode != 0:
        record_log(
            "Recorder command failed: "
            + (result.stderr.strip() or result.stdout.strip() or "unknown error")
        )
        return False

    STOP.wait(CHUNK_SECONDS)
    stop_recording()
    if not wait_until_file_stable(audio_path):
        record_log(f"Capture did not finalize cleanly: {capture_id}")
        return False

    write_ready_metadata(
        metadata_path,
        {
            "capture_id": capture_id,
            "captured_at": captured_at,
            "speaker": SPEAKER_LABEL,
            "duration_target_seconds": CHUNK_SECONDS,
        },
    )
    LAST_CYCLE_SECONDS = round(time.monotonic() - started, 2)
    record_log(f"Capture queued {capture_id}; cycle took {LAST_CYCLE_SECONDS:.2f}s")
    return True


def check_connection():
    try:
        response = requests.get(PING_URL, timeout=8)
        record_log(f"Receiver ping: HTTP {response.status_code}")
    except Exception as exc:
        record_log(f"Receiver offline; local queue will retain audio: {exc}")


def main():
    signal.signal(signal.SIGINT, request_stop)
    signal.signal(signal.SIGTERM, request_stop)
    os.makedirs(QUEUE_DIR, exist_ok=True)
    if not acquire_process_lock():
        record_log("Another record_v2.py process is already running; refusing to start")
        return 2
    try:
        recover_orphan_audio()
        check_connection()
        uploader = threading.Thread(
            target=uploader_loop, name="motox-uploader", daemon=True
        )
        uploader.start()
        record_log("record_v2.py started; capture and delivery are independent")
        while not STOP.is_set():
            record_one()
        uploader.join(timeout=5)
        record_log("Recorder stopped; any undelivered audio remains in the queue")
        return 0
    finally:
        release_process_lock()


if __name__ == "__main__":
    sys.exit(main())
