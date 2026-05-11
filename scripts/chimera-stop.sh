#!/usr/bin/env bash
# Stop background supervisor started by scripts/chimera-start.sh (PID file from chimera-names.sh).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
# shellcheck source=scripts/chimera-names.sh
source "$ROOT/scripts/chimera-names.sh"

prog="$(basename "$0")"
pidf="$(chimera_pid_path)"

if [[ ! -f "$pidf" ]]; then
	echo "${prog}: no $pidf — nothing to stop"
	exit 0
fi

pid="$(cat "$pidf")"
if kill -0 "$pid" 2>/dev/null; then
	kill "$pid" 2>/dev/null || true
	echo "${prog}: sent SIGTERM to pid $pid (supervisor; child processes may exit with it)"
else
	echo "${prog}: stale pid file (process $pid not running)"
fi
rm -f "$pidf"
