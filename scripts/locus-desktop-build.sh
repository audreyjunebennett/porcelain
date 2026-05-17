#!/usr/bin/env bash
# make locus-desktop-build — go build -tags desktop → $(LOCUS_DESKTOP_BIN_BASE)[.exe] (arg: output name).
set -euo pipefail
root=$(cd "$(dirname "$0")/.." && pwd)
# shellcheck source=scripts/chimera-names.sh
source "$root/scripts/chimera-names.sh"
# shellcheck source=scripts/msys2-gcc-path.sh
source "$root/scripts/msys2-gcc-path.sh"
msys2_prepend_gcc_path || true
bin="${1:?locus-desktop-build.sh: missing output binary name (e.g. ${LOCUS_DESKTOP_BIN_BASE} or ${LOCUS_DESKTOP_BIN_BASE}.exe)}"
cd "$root"
export CGO_ENABLED=1
# Windows: GUI subsystem so double-click / Explorer launch does not open a console host (logs → /ui/logs).
target_os="${GOOS:-$(go env GOOS)}"
# Flags before package args only (-ldflags after ./cmd/... is parsed as a package path).
args=("-tags" "desktop")
if [[ "$target_os" == "windows" ]]; then
	args+=(-ldflags "-H=windowsgui")
fi
args+=("-o" "$root/locus/bin/$bin" "./${CHIMERA_CMD_DESKTOP}")
if ! go build "${args[@]}"; then
  echo "" >&2
  echo "${CHIMERA_MAKE_DESKTOP_BUILD_TARGET}: needs CGO and native WebView deps (WebKitGTK on Linux, WebView2 on Windows)." >&2
  echo "  Run:  make ${CHIMERA_MAKE_DESKTOP_INSTALL_TARGET}" >&2
  exit 1
fi
echo "Built $root/locus/bin/$bin — run:  make ${CHIMERA_MAKE_DESKTOP_RUN_TARGET}   or  ./locus/bin/$bin   (supervisor+UI) / ./locus/bin/$bin --headless"
