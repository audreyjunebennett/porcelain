# Installation

This document covers **installing toolchains and third-party binaries** so you can build and run Claudia Gateway from source. It does **not** cover gateway configuration (tokens, `gateway.yaml`, provider keys) or verifying that the server is healthy — see [configuration.md](configuration.md) and the **Execution** section in the repo [README.md](../README.md).

## What gets installed

From a clean clone you typically need:

1. **Language runtimes and build driver** on your machine — **Go** (to build this repo and BiFrost’s Go code), **Node.js** (BiFrost’s UI is built with `npm`; `make claudia-install` runs that step), **Git** (clone BiFrost), **GNU Make**, and a **C compiler for CGO** (`gcc` or `clang` on `PATH`) because BiFrost’s `bifrost-http` binary is built with **CGO** enabled.
2. **BiFrost** — a checkout under `.deps/bifrost` and a compiled `bifrost-http` binary copied to `./bin/bifrost-http`. The gateway talks to BiFrost over HTTP (upstream URL in `gateway.yaml` when you configure it later).
3. **Qdrant** (optional for the full local stack) — a prebuilt `./bin/qdrant` binary downloaded from GitHub releases, matching the version pinned in `deps.lock`.

Pinned versions live in repo-root `deps.lock` (single place to bump them). The important keys are `BIFROST_GIT_URL`, `BIFROST_GIT_REF` (commit, tag, or branch), and `QDRANT_RELEASE`. `scripts/install.sh` (via `make claudia-install`) and `scripts/deps-lock.sh` read that file; always treat `deps.lock` as the source of truth for exact pins.

## Prerequisites

You need **Go 1.22+**, **Node.js 20+**, **Git**, **GNU Make**, `gcc` or `clang` (CGO), plus `bash`, `curl`, and `tar` (and `unzip` on Windows/Git Bash for the Qdrant zip) for `make claudia-install` (or `make install`, which also runs `desktop-install`). Below: **Ubuntu**, **macOS**, and **Windows** for each.

Go drives `go build` / `make claudia-build` here and inside BiFrost’s `make build`. Node is only for building BiFrost (during `make claudia-install`); the `claudia` binary does not embed Node.

### C compiler (CGO) {#c-compiler-cgo}

BiFrost’s `bifrost-http` target is linked with **CGO**. If `gcc` is missing, Go prints `cgo: C compiler "gcc" not found` and `tmp/bifrost-http` may not be produced.

`make claudia-install` runs `scripts/install.sh`, which **sources** `scripts/install-gcc.sh` when neither `gcc` nor `clang` is on `PATH` (so any `PATH` fixups in that script apply to the same shell as BiFrost bootstrap). The helper uses the OS package manager when it can (`apt`, `dnf`/`yum`, `pacman`, `zypper`, `apk`, **Homebrew** on macOS, and on Windows **Git Bash / MSYS**: `winget` WinLibs packages, then **Chocolatey** `mingw`, then **Scoop** `mingw-winlibs`).

**Git Bash** sometimes fails `command -v gcc` even when `gcc.exe` is on `PATH`; `make claudia-install` uses a shared detector that also checks `gcc.exe` and common MinGW names, prefers the **UCRT** WinLibs layout over **MSVCRT** when both are present, and treats Chocolatey’s `gcc.exe` as present with `test -f` (not `-x`, which is unreliable for Windows binaries in MSYS).

On **Windows**, `winget` and **Chocolatey** installs often need **Administrator** rights. The script cannot become Administrator silently; when your shell is not elevated it will trigger a **UAC** prompt and re-run only that package command elevated (via PowerShell `Start-Process -Verb RunAs`). It first asks `winget list` / `choco list` whether the package is already installed so it does not elevate or reinstall unnecessarily. Approve the prompt, or run **Git Bash** / your terminal **as Administrator** so installs run in-session. Set `SKIP_WIN_ELEVATE=1` to skip that behavior and install `gcc` yourself (e.g. **Scoop** `mingw-winlibs`, which is usually per-user). You may still need a **new terminal** if `PATH` was updated for the machine or your profile.

On **Linux**, the helper runs `apt` / `dnf` / etc. with `sudo` when `sudo` is on `PATH`. Set `SKIP_AUTO_GCC=1` when running `make claudia-install` if you want to install a compiler yourself and avoid the helper (the install will fail until `gcc` or `clang` is available).

Manual options if auto-install is not suitable:

- **Ubuntu / Debian:** `sudo apt install build-essential` (includes `gcc`).
- **macOS:** **Xcode Command Line Tools** (`xcode-select --install`) or `brew install gcc` if `gcc` is not on `PATH`.
- **Windows:** Install a **MinGW-w64** toolchain so `gcc` is on `PATH` in the same shell you use for `make claudia-install` (e.g. **MSYS2**: `pacman -S mingw-w64-ucrt-x86_64-gcc`, then use that environment’s `bash` / `make`), or run `make claudia-install` from **WSL (Ubuntu)** with `build-essential`.

### Go

- **Ubuntu:** Default `golang-go` in the archive can be older than 1.22. Prefer: `sudo snap install go --classic` (tracks current stable), or install from [go.dev/dl](https://go.dev/dl/) (Linux tarball: extract to `/usr/local/go`, then add `export PATH=$PATH:/usr/local/go/bin` to `~/.profile` and open a new shell). Verify: `go version` (need **1.22+**).
- **macOS:** `brew install go`, or install the **macOS** package from [go.dev/dl](https://go.dev/dl/). Verify: `go version`.
- **Windows:** MSI from [go.dev/dl](https://go.dev/dl/) or `winget install GoLang.Go`. If `go` is missing in a new terminal: **Settings → System → About → Advanced system settings → Environment Variables** → add `C:\Program Files\Go\bin` (or your install location) to **Path**. Verify: `go version`.

### Node.js (20 or later)

- **Ubuntu:** The stock `nodejs` package may be too old. Use [NodeSource’s Node 20 setup](https://github.com/nodesource/distributions#installation-instructions) (e.g. their `setup_20.x` script then `sudo apt-get install -y nodejs`), or install [nvm](https://github.com/nvm-sh/nvm) and run `nvm install 20`. Verify: `node -v` (major version **≥ 20**).
- **macOS:** `brew install node` (upgrade with `brew upgrade node` if `node -v` is below **20**), or install **LTS** from [nodejs.org](https://nodejs.org/). Verify: `node -v` (major **≥ 20**).
- **Windows:** LTS installer from [nodejs.org](https://nodejs.org/) or `winget install OpenJS.NodeJS.LTS`. Verify: `node -v`.

### Git

- **Ubuntu:** `sudo apt update && sudo apt install git`. Verify: `git --version`.
- **macOS:** `xcode-select --install` ([Xcode Command Line Tools](https://developer.apple.com/xcode/resources/)) includes `git`, or `brew install git`. Verify: `git --version`.
- **Windows:** [git-scm.com/download/win](https://git-scm.com/download/win) or `winget install Git.Git`. Open a **new** terminal; verify: `git --version`.

### Make

Use **GNU Make**, not MSVC `nmake`. Verify: `make --version` (look for *GNU Make*).

**Auto-install helpers** (from the repo root): `bash scripts/install-make.sh` on Git Bash / Linux / macOS, or `pwsh -ExecutionPolicy Bypass -File scripts/install-make.ps1` on Windows. They try `apt` / `dnf` / `brew` / `winget` (`GnuWin32.Make`) / **Chocolatey** / **Scoop** as available. Set `SKIP_AUTO_MAKE=1` to skip. You may need a **new terminal** after `winget` or **Chocolatey** updates `PATH`.

Manual options:

- **Ubuntu:** `sudo apt update && sudo apt install build-essential` ( `make` plus a C toolchain BiFrost may need) or `sudo apt install make`. Verify: `make --version`.
- **macOS:** **Xcode Command Line Tools** (`xcode-select --install`) provide `make`. Or `brew install make` — if the command is `gmake`, run `gmake` wherever this doc says `make`, or put GNU `make` first on `PATH`. Verify: `make --version` or `gmake --version`.
- **Windows:** [Git for Windows](https://git-scm.com/download/win) does **not** ship `make`. Options (use the same environment you run `make` in): **WSL (Ubuntu):** `sudo apt update && sudo apt install make`; **Scoop:** `scoop install make`; **Chocolatey:** `choco install make`; **MSYS2:** `pacman -S make`. Verify inside that environment: `make --version`.

### `bash`, `curl`, and `tar` (for `make claudia-install`)

`scripts/install.sh` runs `scripts/install-bootstrap.sh`, which uses `curl` and `tar` (or `unzip` for the Windows Qdrant asset).

- **Ubuntu:** `sudo apt install bash curl tar` (`bash` is usually already present). Verify: `bash --version`, `curl --version`, `tar --version`.
- **macOS:** **bash**, **curl**, and **tar** ship with the OS; Xcode CLT is enough if prompted. Verify the same three commands.
- **Windows:** **Git Bash** or **WSL** — install **GNU Make** (e.g. Chocolatey `choco install make`) and use `make claudia-install` from the same environment. **Git Bash** provides `bash` and `unzip` for the Qdrant zip path in `qdrant-from-release.sh`. **cmd.exe** alone is not supported for the bootstrap scripts.

### Windows and `make claudia-install`

Prefer **Git Bash** (or **WSL**) so `bash`, `curl`, `make`, and `unzip` line up with `scripts/install-bootstrap.sh`. Native **Go** and **Node** on Windows are fine if `make` invokes **Git’s bash** for `install` (see root `Makefile` `GITBASH` on `OS=Windows_NT`).

## Clone the repository

```bash
git clone <your-fork-or-upstream-url> claudia-gateway
cd claudia-gateway
```

Use whichever remote you develop against; the install steps are the same.

## Install BiFrost and Qdrant (`make claudia-install`)

`make install` runs `make claudia-install` and then `make desktop-install` (OS packages for the desktop WebView build). This section describes `claudia-install` only — use it alone on headless hosts when you only need the toolchain and `./bin/` binaries.

Run from the repository root (idempotent — safe to repeat after `deps.lock` changes or partial failures):

```bash
make claudia-install
```

For a full local desktop dev environment (same as `make up` prerequisites), use `make install` instead.

### What this does

1. **Reports toolchain status** — `go`, `git`, `make`, `node` (warns if Node major version is below 20); exits if required tools are missing.
2. Runs `scripts/install-bootstrap.sh`, which:
   - Creates `.deps/bifrost` if needed, **clones** BiFrost from `BIFROST_GIT_URL`, **checks out** `BIFROST_GIT_REF`, runs BiFrost’s `make setup-workspace` and `make build LOCAL=1` (may run `npm ci` in BiFrost’s UI — several minutes first time).
   - Copies `tmp/bifrost-http` (or `.exe` on Windows) into `./bin/`.
   - Runs `scripts/qdrant-from-release.sh` for your OS (Linux **tar.gz**, macOS **tar.gz**, Windows **zip** under Git Bash / MSYS).

### Re-runs and disk layout

- If `.deps/bifrost` already exists, the script **reuses** it, fetches updates, and checks out the pinned ref again.
- Expect a **large** `.deps/bifrost` directory. `./bin` holds `bifrost-http` and `qdrant` (with `.exe` suffixes on Windows where applicable).

### Common failures

| Symptom | Likely cause |
|--------|----------------|
| `install: install missing tools` / Node warnings | Install **Go**, **Node 20+**, **Git**, **Make**, **gcc** or **clang**, **bash**; open a new shell. |
| `cgo: C compiler "gcc" not found` / no `tmp/bifrost-http` | Install **gcc** (or **clang**) on `PATH`; see **C compiler (CGO)** above. |
| `git` / `curl` / `make` / `bash` not found | See prerequisites; on Windows use **GNU Make** + **Git Bash** for `make claudia-install`. |
| BiFrost `make` errors | Network or incomplete clone; try `make clean-all CONFIRM=1` then `make claudia-install`, or check BiFrost docs. |
| Qdrant fetch fails | Ensure `unzip` (Windows) or `tar` (Unix) on PATH; confirm `QDRANT_RELEASE` in `deps.lock`. |

To refresh **Qdrant only** (no BiFrost): `bash scripts/qdrant-from-release.sh` from the repo root (same script `make claudia-install` runs).

## Build the `claudia` binary

After dependencies are in place (BiFrost is only required when you run a supervised stack or point the gateway at a live upstream — not strictly required **only** to compile `claudia`):

```bash
make claudia-build
```

This produces the `./claudia` executable in the repo root (or use `go build -o claudia ./cmd/claudia` if you prefer invoking `go` directly).

## Next steps

- **Configuration** (environment file, `config/tokens.yaml`, `config/gateway.yaml`, `config/bifrost.config.json`): [configuration.md](configuration.md).
- **Running** BiFrost and the gateway together (`claudia serve`, local binaries): [supervisor.md](supervisor.md).
