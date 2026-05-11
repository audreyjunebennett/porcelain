#!/usr/bin/env bash
# make desktop-build — go build -tags desktop → claudia-desktop[.exe] (arg: output name).
set -euo pipefail
root=$(cd "$(dirname "$0")/.." && pwd)
# shellcheck source=scripts/msys2-gcc-path.sh
source "$root/scripts/msys2-gcc-path.sh"
msys2_prepend_gcc_path || true
bin="${1:?desktop-build.sh: missing output binary name (e.g. claudia-desktop or claudia-desktop.exe)}"
cd "$root"
export CGO_ENABLED=1
# Windows: GUI subsystem so double-click / Explorer launch does not open a console host (logs → /ui/logs).
target_os="${GOOS:-$(go env GOOS)}"
target_arch="${GOARCH:-$(go env GOARCH)}"
# Flags before package args only (-ldflags after ./cmd/claudia is parsed as a package path).
args=("-tags" "desktop")
if [[ "$target_os" == "windows" ]]; then
	# Embed Explorer file icon from assets/icon.ico via a COFF resource object.
	# This keeps the .exe icon consistent with the in-app window icon.
	rc_file="$root/cmd/claudia/icon_windows.rc"
	syso_file="$root/cmd/claudia/icon_windows_${target_arch}.syso"
	if command -v windres >/dev/null 2>&1; then
		windres "$rc_file" -O coff -o "$syso_file"
	elif command -v x86_64-w64-mingw32-windres >/dev/null 2>&1; then
		x86_64-w64-mingw32-windres "$rc_file" -O coff -o "$syso_file"
	else
		echo "desktop-build: warning: windres not found; .exe will use default file icon in Explorer." >&2
	fi
	args+=(-ldflags "-H=windowsgui")
fi
args+=("-o" "$root/$bin" "./cmd/claudia")
if ! go build "${args[@]}"; then
  echo "" >&2
  echo "desktop-build: needs CGO and native WebView deps (WebKitGTK on Linux, WebView2 on Windows)." >&2
  echo "  Run:  make desktop-install" >&2
  exit 1
fi
echo "Built $root/$bin — run:  make desktop-run   or  ./$bin   (supervisor+UI) / ./$bin --headless"
