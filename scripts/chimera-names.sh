#!/usr/bin/env bash
# chimera-names.sh — canonical binary / path / make-target names for shell scripts.
# Makefiles mirror the gateway/indexer basenames: see chimera/Makefile (CHIMERA_GATEWAY_BIN, CHIMERA_INDEX_BIN).
#
# Usage (from repo root after: ROOT="$(cd "$(dirname "$0")/.." && pwd)"; cd "$ROOT"):
#   # shellcheck source=scripts/chimera-names.sh
#   source "$ROOT/scripts/chimera-names.sh"
#
# Forks may export overrides before sourcing (e.g. CHIMERA_GATEWAY_BIN_BASE=mygw).

: "${CHIMERA_GATEWAY_BIN_BASE:=chimera-gateway}"
: "${CHIMERA_INDEX_BIN_BASE:=chimera-indexer}"
: "${CHIMERA_SUPERVISOR_BIN_BASE:=chimera-supervisor}"
: "${CHIMERA_BROKER_BIN_BASE:=chimera-broker}"
: "${CHIMERA_VECTORSTORE_BIN_BASE:=chimera-vectorstore}"
: "${LOCUS_DESKTOP_BIN_BASE:=locus-desktop}"
: "${CHIMERA_RUN_DIR:=run}"
: "${CHIMERA_LOG_DIR:=logs}"
: "${CHIMERA_DIST_BUNDLE_PREFIX:=chimera-bundle}"

# Release make targets (install / build / package).
: "${RELEASE_MAKE_INSTALL_TARGET:=release-install}"
: "${RELEASE_MAKE_BUILD_TARGET:=release-build}"
: "${RELEASE_MAKE_PACKAGE_TARGET:=release-package}"

# Primary make targets (canonical namespace only).
: "${CHIMERA_MAKE_INSTALL_TARGET:=chimera-install}"
: "${CHIMERA_MAKE_BUILD_TARGET:=chimera-gateway-build}"
: "${CHIMERA_MAKE_GATEWAY_INSTALL_TARGET:=chimera-gateway-install}"
: "${CHIMERA_MAKE_START_TARGET:=chimera-run-all}"
: "${CHIMERA_MAKE_STOP_TARGET:=chimera-stop-all}"
: "${CHIMERA_MAKE_STATUS_TARGET:=chimera-status}"
: "${CHIMERA_MAKE_SERVE_TARGET:=chimera-supervisor-run}"
: "${CHIMERA_MAKE_RUN_TARGET:=chimera-gateway-run}"
: "${CHIMERA_MAKE_TEST_GATEWAY_TARGET:=chimera-gateway-test}"
: "${CHIMERA_MAKE_TEST_GATEWAY_UNIT_TARGET:=chimera-gateway-test-unit}"
: "${CHIMERA_MAKE_TEST_GATEWAY_E2E_TARGET:=chimera-gateway-test-e2e}"
: "${CHIMERA_MAKE_BUILD_ALL_TARGET:=chimera-build-all}"
: "${CHIMERA_MAKE_PID_BASENAME:=chimera-supervisor}"
: "${CHIMERA_MAKE_SUPERVISOR_BUILD_TARGET:=chimera-supervisor-build}"
: "${CHIMERA_MAKE_SUPERVISOR_RUN_TARGET:=chimera-supervisor-run}"
: "${CHIMERA_MAKE_SUPERVISOR_TEST_TARGET:=chimera-supervisor-test}"
: "${CHIMERA_MAKE_BROKER_INSTALL_TARGET:=chimera-broker-install}"
: "${CHIMERA_MAKE_BROKER_BUILD_TARGET:=chimera-broker-build}"
: "${CHIMERA_MAKE_BROKER_RUN_TARGET:=chimera-broker-run}"
: "${CHIMERA_MAKE_BROKER_TEST_TARGET:=chimera-broker-test}"
: "${CHIMERA_MAKE_BROKER_TEST_UNIT_TARGET:=chimera-broker-test-unit}"
: "${CHIMERA_MAKE_BROKER_TEST_E2E_TARGET:=chimera-broker-test-e2e}"
: "${CHIMERA_MAKE_VECTORSTORE_INSTALL_TARGET:=chimera-vectorstore-install}"
: "${CHIMERA_MAKE_VECTORSTORE_BUILD_TARGET:=chimera-vectorstore-build}"
: "${CHIMERA_MAKE_VECTORSTORE_RUN_TARGET:=chimera-vectorstore-run}"
: "${CHIMERA_MAKE_VECTORSTORE_TEST_TARGET:=chimera-vectorstore-test}"
: "${CHIMERA_MAKE_VECTORSTORE_TEST_UNIT_TARGET:=chimera-vectorstore-test-unit}"
: "${CHIMERA_MAKE_VECTORSTORE_TEST_E2E_TARGET:=chimera-vectorstore-test-e2e}"
: "${CHIMERA_MAKE_INDEXER_BUILD_TARGET:=chimera-indexer-build}"
: "${CHIMERA_MAKE_INDEXER_RUN_TARGET:=chimera-indexer-run}"
: "${CHIMERA_MAKE_INDEXER_TEST_TARGET:=chimera-indexer-test}"
: "${LOCUS_MAKE_DESKTOP_INSTALL_TARGET:=locus-desktop-install}"
: "${LOCUS_MAKE_DESKTOP_BUILD_TARGET:=locus-desktop-build}"
: "${LOCUS_MAKE_DESKTOP_RUN_TARGET:=locus-desktop-run}"

# Go package paths under ./cmd/.
: "${CHIMERA_CMD_GATEWAY:=chimera/chimera-gateway}"
: "${CHIMERA_CMD_SUPERVISOR:=chimera/chimera-supervisor}"
: "${CHIMERA_CMD_BROKER:=chimera/chimera-broker}"
: "${CHIMERA_CMD_VECTORSTORE:=chimera/chimera-vectorstore}"
: "${LOCUS_CMD_DESKTOP:=locus/locus-desktop}"
: "${CHIMERA_CMD_TOKENCOUNT:=chimera/cmd/tokencount}"
: "${CHIMERA_CMD_INDEXER:=chimera/chimera-indexer}"

chimera_pid_path() {
	printf '%s/%s.pid' "${CHIMERA_RUN_DIR}" "${CHIMERA_MAKE_PID_BASENAME}"
}

chimera_log_path() {
	printf '%s/%s.log' "${CHIMERA_LOG_DIR}" "${CHIMERA_SUPERVISOR_BIN_BASE}"
}

# Prints the first existing ./<supervisor>[.exe] relative path, or returns 1 if missing.
chimera_resolve_supervisor_binary() {
	local win unix
	win="./${CHIMERA_SUPERVISOR_BIN_BASE}.exe"
	unix="./${CHIMERA_SUPERVISOR_BIN_BASE}"
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
