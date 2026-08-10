"""Windows tray supervisor for the Moto X receiver.

Run ``python receiver_manager.py --install-startup`` once to register the
manager for the current Windows user. Normal operation uses pythonw.exe so the
manager and receiver stay hidden.
"""

from __future__ import annotations

import argparse
import ctypes
import json
import os
import subprocess
import sys
import threading
import time
import urllib.error
import urllib.request
import webbrowser
import winreg
from pathlib import Path

import psutil
import pystray
from PIL import Image


APP_NAME = "Moto X Receiver Manager"
MUTEX_NAME = r"Local\PorcelainMotoXReceiverManager"
DASHBOARD_URL = "https://sunset.tailfa86ac.ts.net/dashboard"
SCRIPTS_DIR = Path(__file__).resolve().parent
RECEIVER_PATH = SCRIPTS_DIR / "receiver.py"
RECEIVER_LOG = SCRIPTS_DIR / "receiver_log.md"
PYTHON_EXE = Path(sys.executable).resolve().with_name("python.exe")
PYTHONW_EXE = Path(sys.executable).resolve().with_name("pythonw.exe")
MANAGER_PATH = Path(__file__).resolve()
STATUS_URL = "http://127.0.0.1:8765/api/motox/status"
STARTUP_KEY = r"Software\Microsoft\Windows\CurrentVersion\Run"

LOCAL_STATE_DIR = (
    Path(os.environ.get("LOCALAPPDATA", str(Path.home() / "AppData" / "Local")))
    / "Porcelain"
    / "MotoX"
)
MANAGER_LOG = LOCAL_STATE_DIR / "receiver_manager.log"
CONSOLE_LOG = LOCAL_STATE_DIR / "receiver_console.log"


def append_manager_log(message: str) -> None:
    LOCAL_STATE_DIR.mkdir(parents=True, exist_ok=True)
    timestamp = time.strftime("%Y-%m-%d %H:%M:%S")
    with MANAGER_LOG.open("a", encoding="utf-8") as handle:
        handle.write(f"[{timestamp}] {message}\n")


def startup_command() -> str:
    return f'"{PYTHONW_EXE}" "{MANAGER_PATH}" --autostart'


def install_startup() -> None:
    with winreg.OpenKey(
        winreg.HKEY_CURRENT_USER, STARTUP_KEY, 0, winreg.KEY_SET_VALUE
    ) as key:
        winreg.SetValueEx(key, APP_NAME, 0, winreg.REG_SZ, startup_command())


def uninstall_startup() -> None:
    with winreg.OpenKey(
        winreg.HKEY_CURRENT_USER, STARTUP_KEY, 0, winreg.KEY_SET_VALUE
    ) as key:
        try:
            winreg.DeleteValue(key, APP_NAME)
        except FileNotFoundError:
            pass


def registered_startup_command() -> str | None:
    try:
        with winreg.OpenKey(winreg.HKEY_CURRENT_USER, STARTUP_KEY) as key:
            value, _ = winreg.QueryValueEx(key, APP_NAME)
            return str(value)
    except FileNotFoundError:
        return None


def acquire_single_instance_mutex() -> int | None:
    kernel32 = ctypes.WinDLL("kernel32", use_last_error=True)
    kernel32.CreateMutexW.argtypes = [ctypes.c_void_p, ctypes.c_bool, ctypes.c_wchar_p]
    kernel32.CreateMutexW.restype = ctypes.c_void_p
    kernel32.CloseHandle.argtypes = [ctypes.c_void_p]
    kernel32.CloseHandle.restype = ctypes.c_bool
    handle = kernel32.CreateMutexW(None, False, MUTEX_NAME)
    if not handle:
        raise ctypes.WinError(ctypes.get_last_error())
    if ctypes.get_last_error() == 183:  # ERROR_ALREADY_EXISTS
        kernel32.CloseHandle(handle)
        return None
    return handle


def normalized_path(value: str | os.PathLike[str]) -> str:
    return os.path.normcase(os.path.abspath(os.fspath(value)))


def receiver_processes() -> list[psutil.Process]:
    expected = normalized_path(RECEIVER_PATH)
    matches: list[psutil.Process] = []
    for process in psutil.process_iter(["pid", "ppid", "cmdline", "name"]):
        try:
            command = process.info.get("cmdline") or []
            cwd = process.cwd()
            command_paths = [
                normalized_path(
                    argument
                    if os.path.isabs(argument)
                    else os.path.join(cwd, argument)
                )
                for argument in command
                if argument.lower().endswith(".py")
            ]
            if expected in command_paths:
                matches.append(process)
        except (OSError, psutil.Error):
            continue

    # Windows venv executables are launchers: python.exe remains as the parent
    # of the base interpreter and both expose the same command line. Count the
    # leaf runtime only, otherwise one receiver looks like a duplicate pair.
    parent_pids = {
        process.info.get("ppid")
        for process in matches
        if process.info.get("ppid") is not None
    }
    return [process for process in matches if process.pid not in parent_pids]


def receiver_api_status(timeout: float = 0.7) -> dict | None:
    try:
        with urllib.request.urlopen(STATUS_URL, timeout=timeout) as response:
            if response.status != 200:
                return None
            return json.loads(response.read().decode("utf-8"))
    except (OSError, ValueError, urllib.error.URLError):
        return None


class ReceiverManager:
    def __init__(self) -> None:
        self.desired_running = True
        self.stopping = False
        self.child: subprocess.Popen | None = None
        self.child_log = None
        self.last_start = 0.0
        self.restart_delay = 2.0
        self.next_start = 0.0
        self.state_lock = threading.RLock()
        self.stop_event = threading.Event()
        self.status_text = "Starting receiver…"
        self.icon = pystray.Icon(
            "moto_x_receiver",
            self._load_icon(),
            APP_NAME,
            menu=pystray.Menu(
                pystray.MenuItem(lambda _: self.status_text, None, enabled=False),
                pystray.Menu.SEPARATOR,
                pystray.MenuItem(
                    "Start",
                    self._menu_start,
                    enabled=lambda _: not self.desired_running,
                ),
                pystray.MenuItem(
                    "Stop",
                    self._menu_stop,
                    enabled=lambda _: self.desired_running,
                ),
                pystray.MenuItem("Restart", self._menu_restart),
                pystray.Menu.SEPARATOR,
                pystray.MenuItem(
                    "Open dashboard",
                    lambda *_: webbrowser.open(DASHBOARD_URL),
                    default=True,
                ),
                pystray.MenuItem("Open receiver log", self._open_receiver_log),
            ),
        )

    @staticmethod
    def _load_icon() -> Image.Image:
        icon_path = SCRIPTS_DIR / "dashboard_assets" / "claudia-listening-192.png"
        return Image.open(icon_path).convert("RGBA")

    def _set_status(self, value: str) -> None:
        if value == self.status_text:
            return
        self.status_text = value
        self.icon.title = f"{APP_NAME}: {value}"
        self.icon.update_menu()

    @staticmethod
    def _open_receiver_log(*_) -> None:
        if not RECEIVER_LOG.exists():
            RECEIVER_LOG.touch()
        os.startfile(RECEIVER_LOG)

    def _menu_start(self, *_) -> None:
        with self.state_lock:
            self.desired_running = True
            self.next_start = 0.0
        append_manager_log("Start requested from tray")

    def _menu_stop(self, *_) -> None:
        threading.Thread(target=self.stop_receiver, daemon=True).start()

    def _menu_restart(self, *_) -> None:
        threading.Thread(target=self.restart_receiver, daemon=True).start()

    def start_receiver(self) -> bool:
        with self.state_lock:
            existing = receiver_processes()
            if existing:
                self._set_status(
                    "Duplicate receivers detected"
                    if len(existing) > 1
                    else f"Running (PID {existing[0].pid})"
                )
                return False
            if receiver_api_status() is not None:
                self._set_status("Receiver API already running")
                return False

            LOCAL_STATE_DIR.mkdir(parents=True, exist_ok=True)
            self.child_log = CONSOLE_LOG.open("a", encoding="utf-8", buffering=1)
            env = os.environ.copy()
            env["PYTHONUNBUFFERED"] = "1"
            self.child = subprocess.Popen(
                [str(PYTHON_EXE), "-u", str(RECEIVER_PATH)],
                cwd=SCRIPTS_DIR,
                env=env,
                stdin=subprocess.DEVNULL,
                stdout=self.child_log,
                stderr=subprocess.STDOUT,
                creationflags=subprocess.CREATE_NO_WINDOW,
            )
            self.last_start = time.monotonic()
            self._set_status(f"Starting (PID {self.child.pid})")
            append_manager_log(f"Started receiver PID {self.child.pid}")
            return True

    def stop_receiver(self) -> None:
        with self.state_lock:
            self.desired_running = False
            self.stopping = True
        processes = receiver_processes()
        if not processes:
            self._set_status("Stopped")
        for process in processes:
            try:
                append_manager_log(f"Stopping receiver PID {process.pid}")
                process.terminate()
            except psutil.Error as exc:
                append_manager_log(f"Could not stop PID {process.pid}: {exc}")
        _, alive = psutil.wait_procs(processes, timeout=10)
        for process in alive:
            try:
                append_manager_log(f"Killing unresponsive receiver PID {process.pid}")
                process.kill()
            except psutil.Error:
                pass
        with self.state_lock:
            self.child = None
            if self.child_log is not None:
                self.child_log.close()
                self.child_log = None
            self.stopping = False
        self._set_status("Stopped")

    def restart_receiver(self) -> None:
        append_manager_log("Restart requested from tray")
        self.stop_receiver()
        with self.state_lock:
            self.desired_running = True
            self.next_start = 0.0

    def supervise(self) -> None:
        while not self.stop_event.wait(1.0):
            with self.state_lock:
                desired = self.desired_running
                stopping = self.stopping
            processes = receiver_processes()

            if len(processes) > 1:
                self._set_status("Duplicate receivers detected")
                continue
            if not desired:
                if not stopping:
                    self._set_status("Stopped")
                continue
            if processes:
                process = processes[0]
                api_status = receiver_api_status()
                if api_status is not None:
                    health = api_status.get("capture_health", "unknown")
                    self._set_status(f"Running · capture {health} (PID {process.pid})")
                    if time.monotonic() - self.last_start >= 60:
                        self.restart_delay = 2.0
                else:
                    self._set_status(f"Starting (PID {process.pid})")
                continue

            now = time.monotonic()
            if self.child is not None:
                exit_code = self.child.poll()
                if exit_code is not None:
                    append_manager_log(
                        f"Receiver exited with code {exit_code}; restart in "
                        f"{self.restart_delay:.0f}s"
                    )
                    if self.child_log is not None:
                        self.child_log.close()
                        self.child_log = None
                    self.child = None
                    self.next_start = now + self.restart_delay
                    self.restart_delay = min(self.restart_delay * 2, 60.0)
            if now >= self.next_start:
                try:
                    self.start_receiver()
                except Exception as exc:
                    append_manager_log(f"Receiver start failed: {exc!r}")
                    self._set_status("Start failed; retrying")
                    self.next_start = now + self.restart_delay
                    self.restart_delay = min(self.restart_delay * 2, 60.0)
            else:
                wait = max(1, round(self.next_start - now))
                self._set_status(f"Restarting in {wait}s")

    def run(self) -> None:
        append_manager_log("Manager started")
        threading.Thread(target=self.supervise, daemon=True).start()
        self.icon.run()


def print_status() -> None:
    processes = receiver_processes()
    print(
        json.dumps(
            {
                "startup_registered": registered_startup_command()
                == startup_command(),
                "startup_command": registered_startup_command(),
                "receiver_pids": [process.pid for process in processes],
                "api_healthy": receiver_api_status() is not None,
                "dashboard": DASHBOARD_URL,
                "receiver_log": str(RECEIVER_LOG),
                "manager_log": str(MANAGER_LOG),
            },
            indent=2,
        )
    )


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    actions = parser.add_mutually_exclusive_group()
    actions.add_argument("--install-startup", action="store_true")
    actions.add_argument("--uninstall-startup", action="store_true")
    actions.add_argument("--status", action="store_true")
    parser.add_argument("--autostart", action="store_true", help=argparse.SUPPRESS)
    args = parser.parse_args()

    if args.install_startup:
        install_startup()
        print(f"Installed startup entry: {startup_command()}")
        return 0
    if args.uninstall_startup:
        uninstall_startup()
        print("Removed startup entry")
        return 0
    if args.status:
        print_status()
        return 0

    mutex = acquire_single_instance_mutex()
    if mutex is None:
        return 0
    ReceiverManager().run()
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
