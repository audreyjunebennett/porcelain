#!/usr/bin/env bash
# Install a C compiler for CGO (BiFrost) using the OS package manager when possible.
# Idempotent: no-op if gcc or clang is already on PATH.
# Set SKIP_AUTO_GCC=1 to skip this script (install.sh will fail if still no compiler).
# Windows: winget/chocolatey installs use a UAC elevation prompt when not already Administrator
# unless SKIP_WIN_ELEVATE=1 (then run Git Bash / make chimera-install as Administrator yourself).
# Sourced by install.sh so PATH shims (e.g. WinGet WinLibs) apply to the same shell.
# On Windows, discovered compiler bin dirs are also appended to the User PATH (registry) when missing.
set -euo pipefail

_SCRIPTS="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=scripts/chimera-names.sh
source "$_SCRIPTS/chimera-names.sh"
# shellcheck source=compiler-detect.sh
source "$_SCRIPTS/compiler-detect.sh"
# shellcheck source=win-persist-user-path.sh
source "$_SCRIPTS/win-persist-user-path.sh"

main() {
	if has_cc; then
		return 0
	fi

	if [[ "${SKIP_AUTO_GCC:-}" == "1" ]]; then
		echo "install-gcc: SKIP_AUTO_GCC=1 — not installing; add gcc or clang to PATH." >&2
		return 1
	fi

	echo "install-gcc: no gcc/clang on PATH — trying platform installers…"

	run() {
		echo "install-gcc: $*"
		"$@" || return 1
	}

	# --- Linux (not WSL vs native: both report Linux) ---
	local kernel
	kernel="$(uname -s 2>/dev/null || echo unknown)"
	if [[ "$kernel" == Linux ]]; then
		if [[ -f /etc/debian_version ]] && command -v apt-get >/dev/null 2>&1; then
			if command -v sudo >/dev/null 2>&1; then
				run sudo apt-get update -qq
				run sudo apt-get install -y build-essential
			else
				run apt-get update -qq
				run apt-get install -y build-essential
			fi
			has_cc && return 0
			echo "install-gcc: build-essential installed but gcc/clang not on PATH" >&2
			return 1
		fi
		if command -v dnf >/dev/null 2>&1; then
			if command -v sudo >/dev/null 2>&1; then
				run sudo dnf install -y gcc gcc-c++ make
			else
				run dnf install -y gcc gcc-c++ make
			fi
			has_cc && return 0
			echo "install-gcc: dnf install ran but gcc/clang not on PATH" >&2
			return 1
		fi
		if command -v yum >/dev/null 2>&1 && ! command -v dnf >/dev/null 2>&1; then
			if command -v sudo >/dev/null 2>&1; then
				run sudo yum install -y gcc gcc-c++ make
			else
				run yum install -y gcc gcc-c++ make
			fi
			has_cc && return 0
			echo "install-gcc: yum install ran but gcc/clang not on PATH" >&2
			return 1
		fi
		if command -v pacman >/dev/null 2>&1; then
			if command -v sudo >/dev/null 2>&1; then
				run sudo pacman -S --needed --noconfirm base-devel
			else
				run pacman -S --needed --noconfirm base-devel
			fi
			has_cc && return 0
			echo "install-gcc: pacman base-devel installed but gcc/clang not on PATH" >&2
			return 1
		fi
		if command -v zypper >/dev/null 2>&1; then
			if command -v sudo >/dev/null 2>&1; then
				run sudo zypper --non-interactive install gcc gcc-c++ make
			else
				run zypper --non-interactive install gcc gcc-c++ make
			fi
			has_cc && return 0
			echo "install-gcc: zypper install ran but gcc/clang not on PATH" >&2
			return 1
		fi
		if command -v apk >/dev/null 2>&1; then
			run apk add --no-cache build-base
			has_cc && return 0
			echo "install-gcc: apk build-base installed but gcc/clang not on PATH" >&2
			return 1
		fi
		echo "install-gcc: unsupported Linux distro for auto-install — install gcc/clang (e.g. build-essential)." >&2
		return 1
	fi

	# --- macOS ---
	if [[ "$kernel" == Darwin ]]; then
		if command -v brew >/dev/null 2>&1; then
			run brew install gcc
			has_cc && return 0
			echo "install-gcc: brew install gcc ran but gcc/clang not on PATH" >&2
			return 1
		fi
		echo "install-gcc: install Homebrew (https://brew.sh) then: brew install gcc" >&2
		echo "install-gcc: or run: xcode-select --install   (Xcode Command Line Tools)" >&2
		return 1
	fi

	# --- Windows: Git Bash / MSYS / MinGW ---
	case "$kernel" in
	MINGW* | MSYS* | CYGWIN*)
		_win_is_admin() {
			powershell.exe -NoProfile -Command "([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)" 2>/dev/null | grep -qi true
		}

		# True if winget reports the package id is already installed (skip install / UAC).
		_win_winget_pkg_installed() {
			local id="$1" out
			if command -v winget >/dev/null 2>&1; then
				out="$(winget list -e --id "$id" --disable-interactivity 2>/dev/null)" || true
			else
				local cmdexe="${WINDIR:-C:/Windows}/System32/cmd.exe"
				cmdexe="${cmdexe//\\//}"
				[[ -f "$cmdexe" ]] || cmdexe="/c/Windows/System32/cmd.exe"
				[[ -f "$cmdexe" ]] || return 1
				out="$("$cmdexe" //c "winget list -e --id \"$id\" --disable-interactivity 2>nul")" || true
			fi
			[[ -n "${out//[[:space:]]/}" ]] || return 1
			echo "$out" | grep -qiE 'no installed package found|no packages found|no matching package|no installed package matches' && return 1
			echo "$out" | grep -qF "$id" || return 1
			echo "$out" | tail -n +3 | grep -q '[[:alnum:]_]' || return 1
			return 0
		}

		# True if Chocolatey lists the package locally (skip install / UAC).
		_win_choco_pkg_installed() {
			local pkg="$1"
			command -v choco >/dev/null 2>&1 || return 1
			choco list -e "$pkg" --local-only -r --no-progress 2>/dev/null | grep -qiE "^${pkg}\\|" && return 0
			return 1
		}

		# Run winget elevated via UAC (user must approve). Cannot silently become Administrator on Windows.
		_win_winget_elevated() {
			local id="$1"
			echo "install-gcc: not Administrator — UAC prompt for winget install ($id)" >&2
			powershell.exe -NoProfile -ExecutionPolicy Bypass -Command "\$w = (Get-Command winget -ErrorAction SilentlyContinue).Source; if (-not \$w) { exit 2 }; \$p = Start-Process -FilePath \$w -Verb RunAs -Wait -PassThru -ArgumentList 'install','-e','--id','$id','--accept-package-agreements','--accept-source-agreements','--disable-interactivity'; if (\$null -ne \$p.ExitCode) { exit \$p.ExitCode }; exit 1" && return 0
			return 1
		}

		_win_choco_install_mingw_elevated() {
			echo "install-gcc: not Administrator — UAC prompt for choco install mingw" >&2
			powershell.exe -NoProfile -ExecutionPolicy Bypass -Command "\$c = (Get-Command choco -ErrorAction SilentlyContinue).Source; if (-not \$c) { exit 2 }; \$p = Start-Process -FilePath \$c -Verb RunAs -Wait -PassThru -ArgumentList 'install','mingw','-y'; if (\$null -ne \$p.ExitCode) { exit \$p.ExitCode }; exit 1" && return 0
			return 1
		}

		_win_winget() {
			local id="$1"
			if _win_winget_pkg_installed "$id"; then
				echo "install-gcc: winget package already installed ($id)" >&2
				return 0
			fi
			_win_try_winget_normal() {
				if command -v winget >/dev/null 2>&1; then
					if run winget install -e --id "$id" --accept-package-agreements --accept-source-agreements --disable-interactivity; then
						return 0
					fi
					return 1
				fi
				local cmdexe="${WINDIR:-C:/Windows}/System32/cmd.exe"
				cmdexe="${cmdexe//\\//}"
				[[ -f "$cmdexe" ]] || cmdexe="/c/Windows/System32/cmd.exe"
				if [[ -f "$cmdexe" ]]; then
					if run "$cmdexe" //c "winget install -e --id \"$id\" --accept-package-agreements --accept-source-agreements --disable-interactivity"; then
						return 0
					fi
				fi
				return 1
			}

			if _win_is_admin; then
				_win_try_winget_normal
				return $?
			fi
			if [[ "${SKIP_WIN_ELEVATE:-}" != "1" ]]; then
				if _win_winget_elevated "$id"; then
					return 0
				fi
			fi
			_win_try_winget_normal
			return $?
		}

		# WinLibs MinGW (winget community repo) — adds GCC; may require new shell for PATH.
		if _win_winget "BrechtSanders.WinLibs.POSIX.UCRT" || _win_winget "BrechtSanders.WinLibs.POSIX.MSVCRT"; then
			# Best-effort: pick up gcc if WinGet laid it down where we can see it.
			_shim_winlibs_path() {
				local base="${LOCALAPPDATA:-}"
				[[ -n "$base" ]] || return 0
				base="${base//\\//}"
				[[ -d "$base/Microsoft/WinGet/Packages" ]] || return 0
				local g=""
				# Prefer UCRT WinLibs when both are installed (find order is arbitrary; MSVCRT often sorts first).
				g=$(find "$base/Microsoft/WinGet/Packages" -type f -path '*WinLibs*UCRT*' -name gcc.exe 2>/dev/null | head -1) || true
				[[ -n "${g:-}" ]] || g=$(find "$base/Microsoft/WinGet/Packages" -type f -path '*WinLibs*MSVCRT*' -name gcc.exe 2>/dev/null | head -1) || true
				[[ -n "${g:-}" ]] || g=$(find "$base/Microsoft/WinGet/Packages" -type f -name gcc.exe 2>/dev/null | head -1) || true
				[[ -n "${g:-}" ]] || return 0
				g="${g//$'\r'/}"
				local bindir msysdir
				bindir="$(dirname "$g")"
				bindir="${bindir//\\//}"
				msysdir="$(win_msys_path "$bindir")"
				# Prepend both /c/... and C:/... so command -v and CGO agree with Git Bash.
				export PATH="$msysdir:$bindir:$PATH"
				hash -r 2>/dev/null || true
				echo "install-gcc: prepended to PATH: $msysdir"
				win_persist_user_path_dir "$msysdir"
				win_persist_user_path_dir "$bindir"
			}
			_shim_winlibs_path
			has_cc && return 0
		fi

		if command -v choco >/dev/null 2>&1; then
			_win_choco_install_mingw() {
				if _win_choco_pkg_installed mingw; then
					echo "install-gcc: Chocolatey package mingw already installed" >&2
					return 0
				fi
				if _win_is_admin; then
					run choco install mingw -y
					return $?
				fi
				if [[ "${SKIP_WIN_ELEVATE:-}" != "1" ]]; then
					if _win_choco_install_mingw_elevated; then
						return 0
					fi
				fi
				run choco install mingw -y
				return $?
			}
			if _win_choco_install_mingw; then
				# Chocolatey mingw64 layout (common). Use -f: Git Bash often reports -x false for PE gcc.exe.
				for d in "/c/ProgramData/chocolatey/lib/mingw/tools/install/mingw64/bin" "/c/tools/mingw64/bin"; do
					if [[ -f "${d}/gcc.exe" ]]; then
						export PATH="${d}:$PATH"
						hash -r 2>/dev/null || true
						echo "install-gcc: prepended to PATH: $d"
						win_persist_user_path_dir "$d"
						break
					fi
				done
				has_cc && return 0
			fi
		fi

		if command -v scoop >/dev/null 2>&1; then
			if run scoop install mingw-winlibs; then
				has_cc && return 0
			fi
		fi

		echo "install-gcc: install WinLibs or MSYS2 manually — see docs/installation.md#c-compiler-cgo" >&2
		echo "install-gcc: tried: winget (BrechtSanders.WinLibs.*), chocolatey mingw, scoop mingw-winlibs" >&2
		return 1
		;;
	*)
		echo "install-gcc: unknown OS ($kernel) — install gcc or clang, then re-run make ${CHIMERA_MAKE_INSTALL_TARGET}" >&2
		return 1
		;;
	esac
}

_rc=0
main "$@" || _rc=$?
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
	exit "$_rc"
fi
return "$_rc"
