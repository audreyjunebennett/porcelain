# Feature: Moto X receiver manager

| Field | Value |
|-------|-------|
| **Doc kind** | `feature-record` |
| **Areas** | Moto X receiver, Windows startup, tray controls |
| **Status** | `current` |
| **Introduced** | Moto X roadmap Phase 3 |
| **Originated from** | [`plans/moto-x-roadmap.md`](../plans/moto-x-roadmap.md) |
| **Related features** | Moto X dashboard and deterministic journal |
| **Depends on** | `.venv-motox`, `receiver.py`, Tailscale Serve |
| **Last updated** | See git history |

## At a glance

The Moto X receiver starts hidden after Windows login and is supervised by a
small Claudia tray application. The tray reports receiver and capture health,
provides start/stop/restart controls, and opens the private dashboard or
receiver log without requiring a terminal.

## Operator-visible behavior

- A purple Claudia icon appears in the Windows notification area, possibly
  under the notification-area overflow arrow.
- Its menu shows current receiver/capture status and offers **Start**,
  **Stop**, **Restart**, **Open dashboard**, and **Open receiver log**.
- Double-clicking the icon opens the private dashboard at
  `https://sunset.tailfa86ac.ts.net/dashboard`.
- The manager and receiver run without console windows during normal use.
- The receiver starts for the current user at login and restarts after an
  unexpected exit with bounded exponential backoff.

## System behavior and contracts

**Invariants**

- `.venv-motox` supplies both the manager and receiver Python runtimes.
- Only one tray manager may supervise the receiver at a time.
- Process discovery treats the Windows virtual-environment launcher and base
  interpreter as one receiver, and detects both absolute and relative
  `receiver.py` launches.
- A manager never launches another receiver while an existing receiver process
  or healthy Moto X API is present.
- Intentional **Stop** disables automatic restart until **Start** or **Restart**
  is selected.
- The manager does not change the recorder's canonical 30-second capture unit.

| Topic | Decision |
|-------|----------|
| Login startup | Per-user `HKCU\Software\Microsoft\Windows\CurrentVersion\Run` entry |
| Hidden execution | `pythonw.exe` manager plus `CREATE_NO_WINDOW` receiver |
| Failure recovery | Manager process supervision with 2-to-60-second backoff |
| Duplicate protection | Named manager mutex plus receiver process/API discovery |
| Receiver log | Existing Syncthing-visible `phone-scripts/receiver_log.md` |
| Supervisor logs | `%LOCALAPPDATA%\Porcelain\MotoX\` |

## Interfaces

| Surface | Detail |
|---------|--------|
| Install startup | `receiver_manager.py --install-startup` |
| Remove startup | `receiver_manager.py --uninstall-startup` |
| Inspect status | `receiver_manager.py --status` emits JSON |
| Dashboard | `https://sunset.tailfa86ac.ts.net/dashboard` |
| Receiver health | `http://127.0.0.1:8765/api/motox/status` |

## Code map

| Concern | Location |
|---------|----------|
| Tray and supervision | `Moto X/phone-scripts/receiver_manager.py` |
| Manager dependencies | `Moto X/phone-scripts/receiver-manager-requirements.txt` |
| Receiver | `Moto X/phone-scripts/receiver.py` |
| Recorder lock portability | `Moto X/phone-scripts/record_v2.py` |

## Verification

- Run `receiver_manager.py --status`; expect one receiver PID, a registered
  startup command, and a healthy API.
- Start a second manager; it must exit successfully without changing the
  receiver PID.
- Terminate the receiver runtime once; expect a new PID and restored API
  without starting a console window.
- Request the private dashboard and expect HTTP 200.
- Run `test_motox_v1.py` and `test_record_v2.py`.

## Out of scope and known gaps

- This is a notification-area application, not a Windows service or a separate
  desktop status window.
- Capture, transcription, and dashboard feature work remain tracked in the
  Moto X roadmap.

## References

- Delivery plan: [`plans/moto-x-roadmap.md`](../plans/moto-x-roadmap.md)
