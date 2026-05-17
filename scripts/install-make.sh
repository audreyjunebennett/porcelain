#!/usr/bin/env bash
# Install GNU Make when missing (repo bootstrap + BiFrost build need it).
# Set SKIP_AUTO_MAKE=1 to skip. Idempotent if make is already GNU Make.
# Safe to source from installer scripts / install-toolchain-deps.sh (updates PATH in caller).
set -euo pipefail

have_gnu_make() {
	if command -v make >/dev/null 2>&1 && make --version 2>/dev/null | grep -qi 'GNU Make'; then
		echo "install-make: OK  $(command -v make)"
		return 0
	fi
	if command -v gmake >/dev/null 2>&1 && gmake --version 2>/dev/null | grep -qi 'GNU Make'; then
		echo "install-make: OK  $(command -v gmake)"
		return 0
	fi
	if command -v mingw32-make >/dev/null 2>&1 && mingw32-make --version 2>/dev/null | grep -qi 'GNU Make'; then
		echo "install-make: OK  $(command -v mingw32-make)  (set MAKE=mingw32-make for BiFrost if needed)"
		return 0
	fi
	return 1
}

run() {
	echo "install-make: $*"
	"$@" || return 1
}

_shim_gnuwin32() {
	local p
	for p in "/c/Program Files (x86)/GnuWin32/bin" "/c/Program Files/GnuWin32/bin"; do
		if [[ -f "$p/make.exe" ]]; then
			export PATH="$p:$PATH"
			hash -r 2>/dev/null || true
			echo "install-make: prepended to PATH: $p"
			return 0
		fi
	done
	return 1
}

install_make_main() {
	if have_gnu_make; then
		return 0
	fi

	if [[ "${SKIP_AUTO_MAKE:-}" == "1" ]]; then
		echo "install-make: SKIP_AUTO_MAKE=1 — install GNU Make and put it on PATH." >&2
		return 1
	fi

	echo "install-make: GNU Make not found — trying platform installers…"

	kernel="$(uname -s 2>/dev/null || echo unknown)"

	if [[ "$kernel" == Linux ]]; then
		if [[ -f /etc/debian_version ]] && command -v apt-get >/dev/null 2>&1; then
			if command -v sudo >/dev/null 2>&1; then
				run sudo apt-get update -qq
				run sudo apt-get install -y make
			else
				run apt-get update -qq
				run apt-get install -y make
			fi
			have_gnu_make && return 0
		fi
		if command -v dnf >/dev/null 2>&1; then
			command -v sudo >/dev/null 2>&1 && run sudo dnf install -y make || run dnf install -y make
			have_gnu_make && return 0
		fi
		if command -v yum >/dev/null 2>&1 && ! command -v dnf >/dev/null 2>&1; then
			command -v sudo >/dev/null 2>&1 && run sudo yum install -y make || run yum install -y make
			have_gnu_make && return 0
		fi
		if command -v pacman >/dev/null 2>&1; then
			command -v sudo >/dev/null 2>&1 && run sudo pacman -S --needed --noconfirm make || run pacman -S --needed --noconfirm make
			have_gnu_make && return 0
		fi
		if command -v zypper >/dev/null 2>&1; then
			command -v sudo >/dev/null 2>&1 && run sudo zypper --non-interactive install make || run zypper --non-interactive install make
			have_gnu_make && return 0
		fi
		if command -v apk >/dev/null 2>&1; then
			run apk add --no-cache make
			have_gnu_make && return 0
		fi
		echo "install-make: unsupported Linux distro — sudo apt install make (or equivalent)." >&2
		return 1
	fi

	if [[ "$kernel" == Darwin ]]; then
		if command -v brew >/dev/null 2>&1; then
			run brew install make
			for d in /opt/homebrew/opt/make/libexec/gnubin /usr/local/opt/make/libexec/gnubin; do
				if [[ -f "$d/make" ]]; then
					export PATH="$d:$PATH"
					hash -r 2>/dev/null || true
					echo "install-make: prepended to PATH: $d"
					break
				fi
			done
			have_gnu_make && return 0
		fi
		echo "install-make: install Xcode CLT (includes make) or: brew install make" >&2
		return 1
	fi

	case "$kernel" in
	MINGW* | MSYS* | CYGWIN*)
		winget_ok=0
		if command -v winget >/dev/null 2>&1; then
			if run winget install -e --id GnuWin32.Make --accept-package-agreements --accept-source-agreements --disable-interactivity; then
				winget_ok=1
			fi
		fi
		if [[ "$winget_ok" -eq 0 ]]; then
			cmdexe="${WINDIR:-C:/Windows}/System32/cmd.exe"
			cmdexe="${cmdexe//\\//}"
			[[ -f "$cmdexe" ]] || cmdexe="/c/Windows/System32/cmd.exe"
			if [[ -f "$cmdexe" ]] && run "$cmdexe" //c "winget install -e --id \"GnuWin32.Make\" --accept-package-agreements --accept-source-agreements --disable-interactivity"; then
				winget_ok=1
			fi
		fi
		if [[ "$winget_ok" -eq 1 ]]; then
			_shim_gnuwin32 || true
			have_gnu_make && return 0
		fi
		if command -v choco >/dev/null 2>&1; then
			if run choco install make -y; then
				hash -r 2>/dev/null || true
				have_gnu_make && return 0
			fi
		fi
		if command -v scoop >/dev/null 2>&1; then
			if run scoop install make; then
				hash -r 2>/dev/null || true
				have_gnu_make && return 0
			fi
		fi
		echo "install-make: try winget install GnuWin32.Make, choco install make, or scoop install make" >&2
		echo "install-make: or run:  pwsh -ExecutionPolicy Bypass -File scripts/install-make.ps1" >&2
		return 1
		;;
	*)
		echo "install-make: install GNU Make for $(uname -s), then re-run." >&2
		return 1
		;;
	esac
}

if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
	_rc=0
	install_make_main "$@" || _rc=$?
	exit "$_rc"
fi
