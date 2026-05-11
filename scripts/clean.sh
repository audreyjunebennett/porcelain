#!/usr/bin/env bash
# Remove local build artifacts only (see Makefile clean).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
# shellcheck source=scripts/chimera-names.sh
source "$ROOT/scripts/chimera-names.sh"

rm -f \
	"${CHIMERA_GATEWAY_BIN_BASE}" "${CHIMERA_GATEWAY_BIN_BASE}.exe" \
	"${CHIMERA_INDEX_BIN_BASE}" "${CHIMERA_INDEX_BIN_BASE}.exe" \
	"${CHIMERA_DESKTOP_BIN_BASE}" "${CHIMERA_DESKTOP_BIN_BASE}.exe" \
	claudia claudia.exe claudia-desktop claudia-desktop.exe claudia-index claudia-index.exe
rm -rf dist
echo "clean: removed ${CHIMERA_GATEWAY_BIN_BASE}[.exe], ${CHIMERA_DESKTOP_BIN_BASE}[.exe], ${CHIMERA_INDEX_BIN_BASE}[.exe], dist/"
