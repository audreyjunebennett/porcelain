#!/usr/bin/env bash
# Used by Makefile clean-all on Windows (POSIX test in the Makefile runs under cmd.exe and fails).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck source=scripts/chimera-names.sh
source "$ROOT/scripts/chimera-names.sh"
if [[ "${1:-}" != "1" ]]; then
	echo "clean-all: removes bin/, packaging/qdrant-bundles/, packages/, node_modules/, .deps/, run/, logs/ (+ make clean: ${CHIMERA_GATEWAY_BIN_BASE}*, ${CHIMERA_SUPERVISOR_BIN_BASE}*, ${CHIMERA_INDEX_BIN_BASE}*, ${LOCUS_DESKTOP_BIN_BASE}*, dist/) — re-run with CONFIRM=1" >&2
	exit 1
fi
