#!/usr/bin/env bash
# Sourced by Chimera installer scripts: auto-install git, GNU make, Go, Node when missing (session PATH + User PATH on Windows).
# Opt out: SKIP_AUTO_GIT, SKIP_AUTO_MAKE, SKIP_AUTO_GO, SKIP_AUTO_NODE (same spirit as SKIP_AUTO_GCC).
# Requires REPO_ROOT and is intended to run under set -euo pipefail from the parent.
set -euo pipefail

_toolchain_scripts="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=win-persist-user-path.sh
source "$_toolchain_scripts/win-persist-user-path.sh"

_toolchain_kernel() {
	uname -s 2>/dev/null || echo unknown
}

_toolchain_is_win_msys() {
	case "$(_toolchain_kernel)" in
	MINGW* | MSYS* | CYGWIN*) return 0 ;;
	*) return 1 ;;
	esac
}

# LOCALAPPDATA / APPDATA may be Windows-style; normalize to forward slashes for MSYS paths.
_toolchain_win_env_path() {
	local _n=$1
	local v="${!_n:-}"
	v="${v//$'\r'/}"
	v="${v//\\//}"
	echo "$v"
}

_toolchain_run() {
	echo "install-toolchain: $*"
	"$@" || return 1
}

# Prepend MSYS-style dir to PATH for this shell (e.g. /c/Program Files/Go/bin).
_toolchain_prepend_path_dir() {
	local d="${1//$'\r'/}"
	d="${d//\\//}"
	[[ -n "$d" && -d "$d" ]] || return 1
	export PATH="$d:$PATH"
	hash -r 2>/dev/null || true
	echo "install-toolchain: prepended to PATH: $d"
	return 0
}

_toolchain_win_is_admin() {
	powershell.exe -NoProfile -Command "([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)" 2>/dev/null | grep -qi true
}

_toolchain_win_winget_pkg_installed() {
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

_toolchain_win_winget_elevated() {
	local id="$1"
	echo "install-toolchain: not Administrator — UAC prompt for winget install ($id)" >&2
	powershell.exe -NoProfile -ExecutionPolicy Bypass -Command "\$w = (Get-Command winget -ErrorAction SilentlyContinue).Source; if (-not \$w) { exit 2 }; \$p = Start-Process -FilePath \$w -Verb RunAs -Wait -PassThru -ArgumentList 'install','-e','--id','$id','--accept-package-agreements','--accept-source-agreements','--disable-interactivity'; if (\$null -ne \$p.ExitCode) { exit \$p.ExitCode }; exit 1" && return 0
	return 1
}

# Install a winget package by id; mirrors install-gcc.sh elevation behavior.
_toolchain_win_winget_install() {
	local id="$1"
	if _toolchain_win_winget_pkg_installed "$id"; then
		echo "install-toolchain: winget package already installed ($id)" >&2
		return 0
	fi
	_win_try_normal() {
		if command -v winget >/dev/null 2>&1; then
			_toolchain_run winget install -e --id "$id" --accept-package-agreements --accept-source-agreements --disable-interactivity
			return $?
		fi
		local cmdexe="${WINDIR:-C:/Windows}/System32/cmd.exe"
		cmdexe="${cmdexe//\\//}"
		[[ -f "$cmdexe" ]] || cmdexe="/c/Windows/System32/cmd.exe"
		[[ -f "$cmdexe" ]] && _toolchain_run "$cmdexe" //c "winget install -e --id \"$id\" --accept-package-agreements --accept-source-agreements --disable-interactivity"
		return $?
	}
	if _toolchain_win_is_admin; then
		_win_try_normal
		return $?
	fi
	if [[ "${SKIP_WIN_ELEVATE:-}" != "1" ]]; then
		if _toolchain_win_winget_elevated "$id"; then
			return 0
		fi
	fi
	_win_try_normal
	return $?
}

_toolchain_shim_go_paths() {
	local d _la
	_la="$(_toolchain_win_env_path LOCALAPPDATA)"
	for d in \
		"/c/Program Files/Go/bin" \
		"/c/Program Files (x86)/Go/bin" \
		"${_la}/Programs/Go/bin"; do
		d="${d//\\//}"
		[[ -f "$d/go.exe" || -f "$d/go" ]] || continue
		_toolchain_prepend_path_dir "$d" || true
		win_persist_user_path_dir "$d" || true
		return 0
	done
	return 1
}

_toolchain_shim_git_paths() {
	local d _la
	_la="$(_toolchain_win_env_path LOCALAPPDATA)"
	for d in \
		"/c/Program Files/Git/cmd" \
		"/c/Program Files/Git/bin" \
		"${_la}/Programs/Git/cmd" \
		"${_la}/Programs/Git/bin"; do
		d="${d//\\//}"
		[[ -f "$d/git.exe" || -f "$d/git" ]] || continue
		_toolchain_prepend_path_dir "$d" || true
		win_persist_user_path_dir "$d" || true
		return 0
	done
	return 1
}

_toolchain_shim_node_paths() {
	local d _la _ad
	_la="$(_toolchain_win_env_path LOCALAPPDATA)"
	_ad="$(_toolchain_win_env_path APPDATA)"
	for d in \
		"/c/Program Files/nodejs" \
		"/c/Program Files (x86)/nodejs" \
		"${_la}/Programs/nodejs" \
		"${_ad}/npm"; do
		d="${d//\\//}"
		[[ -f "$d/node.exe" || -f "$d/node" ]] || continue
		_toolchain_prepend_path_dir "$d" || true
		win_persist_user_path_dir "$d" || true
		return 0
	done
	return 1
}

toolchain_ensure_git() {
	if command -v git >/dev/null 2>&1; then
		echo "    OK  git → $(command -v git)"
		return 0
	fi
	if [[ "${SKIP_AUTO_GIT:-}" == "1" ]]; then
		echo "    MISSING  git (SKIP_AUTO_GIT=1)" >&2
		return 1
	fi
	echo "    MISSING  git — attempting install…" >&2
	local k
	k="$(_toolchain_kernel)"
	if [[ "$k" == Linux ]]; then
		if [[ -f /etc/debian_version ]] && command -v apt-get >/dev/null 2>&1; then
			command -v sudo >/dev/null 2>&1 && _toolchain_run sudo apt-get update -qq && _toolchain_run sudo apt-get install -y git || {
				_toolchain_run apt-get update -qq && _toolchain_run apt-get install -y git
			}
		elif command -v dnf >/dev/null 2>&1; then
			command -v sudo >/dev/null 2>&1 && _toolchain_run sudo dnf install -y git || _toolchain_run dnf install -y git
		elif command -v yum >/dev/null 2>&1 && ! command -v dnf >/dev/null 2>&1; then
			command -v sudo >/dev/null 2>&1 && _toolchain_run sudo yum install -y git || _toolchain_run yum install -y git
		elif command -v pacman >/dev/null 2>&1; then
			command -v sudo >/dev/null 2>&1 && _toolchain_run sudo pacman -S --needed --noconfirm git || _toolchain_run pacman -S --needed --noconfirm git
		elif command -v zypper >/dev/null 2>&1; then
			command -v sudo >/dev/null 2>&1 && _toolchain_run sudo zypper --non-interactive install git || _toolchain_run zypper --non-interactive install git
		elif command -v apk >/dev/null 2>&1; then
			_toolchain_run apk add --no-cache git
		else
			echo "    install-toolchain: unsupported Linux — install git" >&2
			return 1
		fi
	elif [[ "$k" == Darwin ]]; then
		if command -v brew >/dev/null 2>&1; then
			_toolchain_run brew install git
		else
			echo "    install-toolchain: install Xcode CLT or Homebrew, then git" >&2
			return 1
		fi
	elif _toolchain_is_win_msys; then
		_toolchain_win_winget_install "Git.Git" || true
		_toolchain_shim_git_paths || true
	else
		echo "    install-toolchain: install git for $k" >&2
		return 1
	fi
	hash -r 2>/dev/null || true
	if command -v git >/dev/null 2>&1; then
		echo "    OK  git → $(command -v git)"
		return 0
	fi
	echo "    MISSING  git after auto-install — open a new terminal or add git to PATH" >&2
	return 1
}

toolchain_ensure_make() {
	if command -v make >/dev/null 2>&1 && make --version 2>/dev/null | grep -qi 'GNU Make'; then
		echo "    OK  make → $(command -v make)"
		return 0
	fi
	if command -v gmake >/dev/null 2>&1 && gmake --version 2>/dev/null | grep -qi 'GNU Make'; then
		echo "    OK  make (gmake) → $(command -v gmake)"
		return 0
	fi
	if command -v mingw32-make >/dev/null 2>&1 && mingw32-make --version 2>/dev/null | grep -qi 'GNU Make'; then
		echo "    OK  make (mingw32-make) → $(command -v mingw32-make)"
		return 0
	fi
	if [[ "${SKIP_AUTO_MAKE:-}" == "1" ]]; then
		echo "    MISSING  make (SKIP_AUTO_MAKE=1)" >&2
		return 1
	fi
	# shellcheck source=scripts/install-make.sh
	source "$REPO_ROOT/scripts/install-make.sh"
	if install_make_main; then
		hash -r 2>/dev/null || true
		if command -v make >/dev/null 2>&1 && make --version 2>/dev/null | grep -qi 'GNU Make'; then
			echo "    OK  make → $(command -v make)"
		elif command -v gmake >/dev/null 2>&1 && gmake --version 2>/dev/null | grep -qi 'GNU Make'; then
			echo "    OK  make (gmake) → $(command -v gmake)"
		elif command -v mingw32-make >/dev/null 2>&1 && mingw32-make --version 2>/dev/null | grep -qi 'GNU Make'; then
			echo "    OK  make (mingw32-make) → $(command -v mingw32-make)"
		else
			echo "    MISSING  GNU make after auto-install" >&2
			return 1
		fi
		for d in "/c/Program Files (x86)/GnuWin32/bin" "/c/Program Files/GnuWin32/bin"; do
			[[ -f "${d}/make.exe" ]] && win_persist_user_path_dir "$d" && break
		done
		return 0
	fi
	echo "    MISSING  GNU make after auto-install" >&2
	return 1
}

toolchain_ensure_go() {
	if command -v go >/dev/null 2>&1; then
		echo "    OK  go → $(command -v go)"
		echo "    go version: $(go version)"
		return 0
	fi
	if [[ "${SKIP_AUTO_GO:-}" == "1" ]]; then
		echo "    MISSING  go (SKIP_AUTO_GO=1)" >&2
		return 1
	fi
	echo "    MISSING  go — attempting install…" >&2
	local k
	k="$(_toolchain_kernel)"
	if [[ "$k" == Linux ]]; then
		if [[ -f /etc/debian_version ]] && command -v apt-get >/dev/null 2>&1; then
			command -v sudo >/dev/null 2>&1 && _toolchain_run sudo apt-get update -qq && _toolchain_run sudo apt-get install -y golang-go || {
				_toolchain_run apt-get update -qq && _toolchain_run apt-get install -y golang-go
			}
		elif command -v dnf >/dev/null 2>&1; then
			command -v sudo >/dev/null 2>&1 && _toolchain_run sudo dnf install -y golang || _toolchain_run dnf install -y golang
		elif command -v pacman >/dev/null 2>&1; then
			command -v sudo >/dev/null 2>&1 && _toolchain_run sudo pacman -S --needed --noconfirm go || _toolchain_run pacman -S --needed --noconfirm go
		else
			echo "    install-toolchain: install Go — https://go.dev/dl/" >&2
			return 1
		fi
	elif [[ "$k" == Darwin ]]; then
		if command -v brew >/dev/null 2>&1; then
			_toolchain_run brew install go
		else
			echo "    install-toolchain: install Homebrew then: brew install go" >&2
			return 1
		fi
	elif _toolchain_is_win_msys; then
		_toolchain_win_winget_install "GoLang.Go" || true
		_toolchain_shim_go_paths || true
	else
		echo "    install-toolchain: install Go for $k" >&2
		return 1
	fi
	hash -r 2>/dev/null || true
	if command -v go >/dev/null 2>&1; then
		echo "    OK  go → $(command -v go)"
		echo "    go version: $(go version)"
		return 0
	fi
	echo "    MISSING  go after auto-install — open a new terminal or see https://go.dev/dl/" >&2
	return 1
}

toolchain_ensure_node() {
	if command -v node >/dev/null 2>&1; then
		local ver major
		ver="$(node -v 2>/dev/null || true)"
		major="$(node -p "parseInt(process.versions.node.split('.')[0],10)" 2>/dev/null || echo 0)"
		if [ "$major" -lt 20 ]; then
			if [[ "${SKIP_AUTO_NODE:-}" == "1" ]]; then
				echo "    WARN  Node.js should be >= 20 (found $ver); SKIP_AUTO_NODE=1" >&2
				return 1
			fi
			echo "    WARN  Node.js should be >= 20 (found $ver) — attempting upgrade…" >&2
		else
			echo "    OK  node $ver"
			return 0
		fi
	else
		if [[ "${SKIP_AUTO_NODE:-}" == "1" ]]; then
			echo "    MISSING  node (SKIP_AUTO_NODE=1)" >&2
			return 1
		fi
		echo "    MISSING  node — attempting install…" >&2
	fi

	local k
	k="$(_toolchain_kernel)"
	if [[ "$k" == Linux ]]; then
		if command -v snap >/dev/null 2>&1; then
			{
				command -v sudo >/dev/null 2>&1 && sudo snap install node --classic --channel=20
			} || snap install node --classic --channel=20 || true
		fi
		if [[ -f /etc/debian_version ]] && command -v apt-get >/dev/null 2>&1; then
			command -v sudo >/dev/null 2>&1 && _toolchain_run sudo apt-get update -qq && _toolchain_run sudo apt-get install -y nodejs npm || {
				_toolchain_run apt-get update -qq && _toolchain_run apt-get install -y nodejs npm
			}
		elif command -v dnf >/dev/null 2>&1; then
			command -v sudo >/dev/null 2>&1 && _toolchain_run sudo dnf install -y nodejs npm || _toolchain_run dnf install -y nodejs npm
		elif command -v pacman >/dev/null 2>&1; then
			command -v sudo >/dev/null 2>&1 && _toolchain_run sudo pacman -S --needed --noconfirm nodejs npm || _toolchain_run pacman -S --needed --noconfirm nodejs npm
		fi
	elif [[ "$k" == Darwin ]]; then
		if command -v brew >/dev/null 2>&1; then
			_toolchain_run brew install node@20 || _toolchain_run brew install node
			for d in /opt/homebrew/opt/node@20/bin /usr/local/opt/node@20/bin /opt/homebrew/opt/node/bin /usr/local/opt/node/bin; do
				[[ -f "$d/node" ]] && _toolchain_prepend_path_dir "$d" && break
			done
		else
			echo "    install-toolchain: install Homebrew then: brew install node@20" >&2
			return 1
		fi
	elif _toolchain_is_win_msys; then
		_toolchain_win_winget_install "OpenJS.NodeJS.LTS" || true
		_toolchain_shim_node_paths || true
	else
		echo "    install-toolchain: install Node.js 20+ for $k" >&2
		return 1
	fi

	hash -r 2>/dev/null || true
	if ! command -v node >/dev/null 2>&1; then
		echo "    MISSING  node after auto-install" >&2
		return 1
	fi
	local ver major
	ver="$(node -v 2>/dev/null || true)"
	major="$(node -p "parseInt(process.versions.node.split('.')[0],10)" 2>/dev/null || echo 0)"
	if [ "$major" -lt 20 ]; then
		echo "    WARN  Node.js should be >= 20 for BiFrost UI (found $ver)" >&2
		return 1
	fi
	echo "    OK  node $ver"
	_la="$(_toolchain_win_env_path LOCALAPPDATA)"
	for d in "/c/Program Files/nodejs" "${_la}/Programs/nodejs"; do
		d="${d//\\//}"
		[[ -f "$d/node.exe" ]] && win_persist_user_path_dir "$d" && break
	done
	return 0
}
