#!/usr/bin/env bash
# chimera-names.sh — canonical binary / path / make-target names for shell scripts.
# Makefiles mirror the gateway/indexer basenames: see chimera/Makefile (CHIMERA_GATEWAY_BIN, CHIMERA_INDEX_BIN).
#
# Usage (from repo root after: ROOT="$(cd "$(dirname "$0")/.." && pwd)"; cd "$ROOT"):
#   # shellcheck source=scripts/chimera-names.sh
#   source "$ROOT/scripts/chimera-names.sh"
#
# Forks may export overrides before sourcing (e.g. CHIMERA_GATEWAY_BIN_BASE=mygw).

: "${CHIMERA_GATEWAY_BIN_BASE:=chimera}"
: "${CHIMERA_INDEX_BIN_BASE:=chimera-index}"
: "${CHIMERA_DESKTOP_BIN_BASE:=porcelain-desktop}"
: "${CHIMERA_RUN_DIR:=run}"
: "${CHIMERA_LOG_DIR:=logs}"
: "${CHIMERA_DIST_BUNDLE_PREFIX:=chimera-bundle}"

# Primary make targets (older claudia-* aliases remain in the Makefile).
: "${CHIMERA_MAKE_INSTALL_TARGET:=chimera-install}"
: "${CHIMERA_MAKE_BUILD_TARGET:=chimera-build}"
: "${CHIMERA_MAKE_START_TARGET:=chimera-start}"
: "${CHIMERA_MAKE_STOP_TARGET:=chimera-stop}"
: "${CHIMERA_MAKE_STATUS_TARGET:=chimera-status}"
: "${CHIMERA_MAKE_SERVE_TARGET:=chimera-serve}"
: "${CHIMERA_MAKE_RUN_TARGET:=chimera-run}"
: "${CHIMERA_MAKE_TEST_GATEWAY_TARGET:=test-chimera}"

# Go package paths under ./cmd/
CHIMERA_CMD_GATEWAY="cmd/${CHIMERA_GATEWAY_BIN_BASE}"
CHIMERA_CMD_INDEXER="cmd/${CHIMERA_INDEX_BIN_BASE}"

chimera_pid_path() {
	printf '%s/%s.pid' "${CHIMERA_RUN_DIR}" "${CHIMERA_GATEWAY_BIN_BASE}"
}

chimera_log_path() {
	printf '%s/%s.log' "${CHIMERA_LOG_DIR}" "${CHIMERA_GATEWAY_BIN_BASE}"
}

# Prints the first existing ./<gateway>[.exe] relative path, or returns 1 if missing.
chimera_resolve_gateway_binary() {
	local win unix
	win="./${CHIMERA_GATEWAY_BIN_BASE}.exe"
	unix="./${CHIMERA_GATEWAY_BIN_BASE}"
	if [[ -f "$win" ]]; then
		echo "$win"
		return 0
	fi
	if [[ -f "$unix" ]]; then
		echo "$unix"
		return 0
	fi
	return 1
}
