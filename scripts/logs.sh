#!/usr/bin/env bash
# make logs — tail gateway supervisor log (path from chimera-names.sh). Usage: scripts/logs.sh [lines]
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
# shellcheck source=scripts/chimera-names.sh
source "$ROOT/scripts/chimera-names.sh"

n="${1:-80}"
logf="$(chimera_log_path)"
if [[ -f "$logf" ]]; then
	tail -n "$n" "$logf"
else
	echo "logs: no $logf — run make ${CHIMERA_MAKE_START_TARGET} or make up first"
fi
