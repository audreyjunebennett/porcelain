#!/usr/bin/env bash
# make chimera-start — run gateway serve in background; log + pid paths from scripts/chimera-names.sh.
# Makefile passes --stack unless UP_STACK=0 (then BiFrost only, no Qdrant).
# Usage: scripts/chimera-start.sh [--stack]   (--stack adds -qdrant-bin when qdrant binary exists)
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
# shellcheck source=scripts/chimera-names.sh
source "$ROOT/scripts/chimera-names.sh"

prog="$(basename "$0")"
mkdir -p "${CHIMERA_LOG_DIR}" "${CHIMERA_RUN_DIR}"

STACK=0
[[ "${1:-}" == "--stack" ]] && STACK=1

pidf="$(chimera_pid_path)"
logf="$(chimera_log_path)"

if [[ -f "$pidf" ]]; then
	pid="$(cat "$pidf")"
	if kill -0 "$pid" 2>/dev/null; then
		echo "${prog}: already running (pid $pid). Stop with: make ${CHIMERA_MAKE_STOP_TARGET}"
		exit 1
	fi
	rm -f "$pidf"
fi

if ! BIN="$(chimera_resolve_gateway_binary)"; then
	echo "${prog}: no ./${CHIMERA_GATEWAY_BIN_BASE} binary — run: make ${CHIMERA_MAKE_BUILD_TARGET}" >&2
	exit 1
fi

BF=bin/bifrost-http
[[ -f bin/bifrost-http.exe ]] && BF=bin/bifrost-http.exe
if [[ ! -f "$BF" ]]; then
	echo "${prog}: missing $BF — run: make ${CHIMERA_MAKE_INSTALL_TARGET}" >&2
	exit 1
fi

ARGS=(serve -bifrost-bin "$BF")
if [[ "$STACK" -eq 1 ]]; then
	QT=bin/qdrant
	[[ -f bin/qdrant.exe ]] && QT=bin/qdrant.exe
	if [[ -f "$QT" ]]; then
		ARGS+=(-qdrant-bin "$QT")
	else
		echo "${prog}: --stack requested but no $QT — run: make ${CHIMERA_MAKE_INSTALL_TARGET}" >&2
		exit 1
	fi
fi

nohup "$BIN" "${ARGS[@]}" >>"$logf" 2>&1 &
echo $! >"$pidf"
echo "${prog}: pid $(cat "$pidf")  log: $logf"
echo "  Gateway   http://127.0.0.1:3000/health"
echo "  BiFrost   http://127.0.0.1:8080"
[[ "$STACK" -eq 1 ]] && echo "  Qdrant    http://127.0.0.1:6333"
