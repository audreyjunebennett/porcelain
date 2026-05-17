#!/usr/bin/env bash
# Remove local build artifacts only (see Makefile clean).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
# shellcheck source=scripts/chimera-names.sh
source "$ROOT/scripts/chimera-names.sh"

rm -f \
	"chimera/bin/${CHIMERA_GATEWAY_BIN_BASE}" "chimera/bin/${CHIMERA_GATEWAY_BIN_BASE}.exe" \
	"chimera/bin/${CHIMERA_SUPERVISOR_BIN_BASE}" "chimera/bin/${CHIMERA_SUPERVISOR_BIN_BASE}.exe" \
	"chimera/bin/${CHIMERA_INDEX_BIN_BASE}" "chimera/bin/${CHIMERA_INDEX_BIN_BASE}.exe"
rm -f \
	"locus/bin/${LOCUS_DESKTOP_BIN_BASE}" "locus/bin/${LOCUS_DESKTOP_BIN_BASE}.exe"
rm -rf dist
echo "clean: removed chimera/bin/${CHIMERA_GATEWAY_BIN_BASE}[.exe], chimera/bin/${CHIMERA_SUPERVISOR_BIN_BASE}[.exe], chimera/bin/${CHIMERA_INDEX_BIN_BASE}[.exe], locus/bin/${LOCUS_DESKTOP_BIN_BASE}[.exe], dist/"
