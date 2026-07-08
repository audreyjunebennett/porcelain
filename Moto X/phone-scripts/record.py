import os
import signal
import subprocess
import sys
import time
from datetime import datetime

import requests


SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))

PC_BASE_URL = os.environ.get("MOTOX_PC_URL", "http://192.168.8.170:8765")
TRANSCRIBE_URL = f"{PC_BASE_URL.rstrip('/')}/transcribe"
PING_URL = f"{PC_BASE_URL.rstrip('/')}/ping"

CHUNK_SECONDS = int(os.environ.get("MOTOX_CHUNK_SECONDS", "30"))
CHUNK_FILE = os.environ.get("MOTOX_CHUNK_FILE", os.path.join(SCRIPT_DIR, "motox_chunk.aac"))
UNSENT_DIR = os.environ.get("MOTOX_UNSENT_DIR", os.path.join(SCRIPT_DIR, "motox_unsent"))
# Syncthing-visible log (default: inside phone-scripts). Override with MOTOX_RECORD_LOG.
RECORD_LOG = os.environ.get(
    "MOTOX_RECORD_LOG",
    os.path.join(SCRIPT_DIR, "motox_record_log.md"),
)
SAMPLE_RATE = os.environ.get("MOTOX_SAMPLE_RATE", "44100")
BITRATE = os.environ.get("MOTOX_AUDIO_BITRATE", "128000")
SPEAKER_LABEL = os.environ.get("MOTOX_SPEAKER", "Ruby")


def record_log(msg: str) -> None:
    """Append one line to RECORD_LOG (syncs to PC via Syncthing) and echo to stdout."""
    ts = datetime.now().strftime("%Y-%m-%d %H:%M:%S")
    line = f"[{ts}] {msg}"
    print(line)
    try:
        os.makedirs(os.path.dirname(RECORD_LOG) or ".", exist_ok=True)
        with open(RECORD_LOG, "a", encoding="utf-8") as f:
            f.write(line + "\n")
    except OSError:
        pass


def stop_recording():
    subprocess.run(["termux-microphone-record", "-q"], capture_output=True)


def cleanup(sig=None, frame=None):
    record_log("Stopping recording (signal)...")
    stop_recording()
    if os.path.exists(CHUNK_FILE):
        os.remove(CHUNK_FILE)
    record_log("Goodbye.")
    sys.exit(0)


def wait_until_file_stable(path, timeout=15):
    start = time.time()
    last_size = -1
    stable_reads = 0

    while time.time() - start < timeout:
        if not os.path.exists(path):
            time.sleep(0.5)
            continue

        size = os.path.getsize(path)
        if size > 1000 and size == last_size:
            stable_reads += 1
            if stable_reads >= 2:
                return True
        else:
            stable_reads = 0

        last_size = size
        time.sleep(1)

    return os.path.exists(path) and os.path.getsize(path) > 1000


def check_connection():
    record_log("Checking connection to PC...")
    try:
        response = requests.get(PING_URL, timeout=8)
        if response.status_code == 200 and response.text.strip() == "ok":
            record_log("OK ping - receiver reachable.")
            return True
        record_log(f"ERROR ping unexpected: HTTP {response.status_code} {response.text[:120]}")
    except Exception as exc:
        record_log(f"ERROR ping failed: {exc} (is receiver.py running? Wi-Fi OK?)")
    return False


def record_chunk():
    if os.path.exists(CHUNK_FILE):
        os.remove(CHUNK_FILE)

    stop_recording()
    time.sleep(1)
    record_log(f"Recording {CHUNK_SECONDS}s chunk...")
    result = subprocess.run(
        [
            "termux-microphone-record",
            "-l",
            str(CHUNK_SECONDS),
            "-f",
            CHUNK_FILE,
            "-e",
            "aac",
            "-r",
            SAMPLE_RATE,
            "-c",
            "1",
            "-b",
            BITRATE,
        ],
        capture_output=True,
        text=True,
    )

    # termux-microphone-record starts recording and returns immediately on some
    # devices, so wait here before asking it to finalize the audio file.
    if result.returncode == 0:
        for seconds_left in range(CHUNK_SECONDS, 0, -1):
            if seconds_left == CHUNK_SECONDS or seconds_left % 5 == 0 or seconds_left <= 3:
                print(f"  {seconds_left}s left...", end="\r", flush=True)
            time.sleep(1)
        print(" " * 24, end="\r")

    stop_recording()
    time.sleep(3)
    if result.returncode != 0:
        record_log(
            "ERROR termux-microphone-record failed: "
            + (result.stderr.strip() or result.stdout.strip() or "unknown")
        )
        return False

    if not wait_until_file_stable(CHUNK_FILE):
        record_log("ERROR chunk file unstable or missing; skipping send.")
        return False

    return True


def send_file(path, filename="motox_chunk.aac"):
    try:
        with open(path, "rb") as file:
            response = requests.post(
                TRANSCRIBE_URL,
                files={"audio": (filename, file, "audio/aac")},
                data={"speaker": SPEAKER_LABEL},
                timeout=120,
            )
        if 200 <= response.status_code < 300:
            record_log(f"OK POST /transcribe -> HTTP {response.status_code}")
        else:
            record_log(f"ERROR POST /transcribe -> HTTP {response.status_code} {response.text[:160]}")
        return 200 <= response.status_code < 300
    except Exception as exc:
        record_log(f"ERROR POST /transcribe failed: {exc}")
        return False


def send_chunk():
    record_log("Sending chunk to PC...")
    if send_file(CHUNK_FILE):
        os.remove(CHUNK_FILE)
    else:
        queued_path = os.path.join(UNSENT_DIR, f"{int(time.time())}.aac")
        try:
            os.rename(CHUNK_FILE, queued_path)
            record_log(f"ERROR offline - queued chunk: {queued_path}")
        except Exception as exc:
            record_log(f"ERROR could not queue chunk: {exc}")
            if os.path.exists(CHUNK_FILE):
                os.remove(CHUNK_FILE)


def drain_unsent():
    if not os.path.isdir(UNSENT_DIR):
        return
    queued = sorted(
        f for f in os.listdir(UNSENT_DIR)
        if os.path.isfile(os.path.join(UNSENT_DIR, f))
    )
    if not queued:
        return
    record_log(f"Retrying {len(queued)} queued chunk(s)...")
    for filename in queued:
        path = os.path.join(UNSENT_DIR, filename)
        if send_file(path, filename):
            os.remove(path)
            record_log(f"OK sent queued chunk: {filename}")
        else:
            record_log("ERROR still offline - will retry next cycle.")
            break


def main():
    signal.signal(signal.SIGINT, cleanup)
    signal.signal(signal.SIGTERM, cleanup)

    os.makedirs(UNSENT_DIR, exist_ok=True)
    record_log(f"record.py starting - log file: {RECORD_LOG}")
    check_connection()
    record_log("Recording loop started (Ctrl+C to stop).")

    while True:
        drain_unsent()
        if record_chunk():
            send_chunk()
        time.sleep(1)


if __name__ == "__main__":
    main()
