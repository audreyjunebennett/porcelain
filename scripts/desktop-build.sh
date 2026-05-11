#!/usr/bin/env bash
# make desktop-build — go build -tags desktop → $(CHIMERA_DESKTOP_BIN_BASE)[.exe] (arg: output name).
set -euo pipefail
root=$(cd "$(dirname "$0")/.." && pwd)
# shellcheck source=scripts/chimera-names.sh
source "$root/scripts/chimera-names.sh"
# shellcheck source=scripts/msys2-gcc-path.sh
source "$root/scripts/msys2-gcc-path.sh"
msys2_prepend_gcc_path || true
bin="${1:?desktop-build.sh: missing output binary name (e.g. ${CHIMERA_DESKTOP_BIN_BASE} or ${CHIMERA_DESKTOP_BIN_BASE}.exe)}"
cd "$root"
export CGO_ENABLED=1
# Windows: GUI subsystem so double-click / Explorer launch does not open a console host (logs → /ui/logs).
target_os="${GOOS:-$(go env GOOS)}"
# Flags before package args only (-ldflags after ./cmd/... is parsed as a package path).
args=("-tags" "desktop")
if [[ "$target_os" == "windows" ]]; then
	args+=(-ldflags "-H=windowsgui")
fi
args+=("-o" "$root/$bin" "./${CHIMERA_CMD_GATEWAY}")
if ! go build "${args[@]}"; then
  echo "" >&2
  echo "desktop-build: needs CGO and native WebView deps (WebKitGTK on Linux, WebView2 on Windows)." >&2
  echo "  Run:  make desktop-install" >&2
  exit 1
fi
echo "Built $root/$bin — run:  make desktop-run   or  ./$bin   (supervisor+UI) / ./$bin --headless"
