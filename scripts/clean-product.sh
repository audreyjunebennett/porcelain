#!/usr/bin/env bash
# Per-product clean helpers (see Makefile chimera-*-clean* / locus-*-clean*).
# Usage:
#   clean-product.sh <product> <mode> [confirm]
#   clean-product.sh --each <mode> [confirm]
# Products (canonical list): gateway | supervisor | broker | vectorstore | indexer | desktop | workspace
#   workspace — shared dirs (bin/, .deps/, run/, logs/, dist/, …); not a shipped product
# Modes: build | install | configure | run | all
#   confirm: required (1) for Chimera run / all, and for --each run / all
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
# shellcheck source=scripts/chimera-names.sh
source "$ROOT/scripts/chimera-names.sh"

# Single source of truth for clean.sh, clean-all.sh, and Makefile porcelain.
CLEAN_PRODUCTS=(gateway supervisor broker vectorstore indexer desktop)

PRODUCT="${1:-}"
MODE="${2:-all}"
CONFIRM="${3:-}"

usage() {
	echo "usage: clean-product.sh <product> <mode> [confirm]" >&2
	echo "       clean-product.sh --each <mode> [confirm]" >&2
	echo "  product: ${CLEAN_PRODUCTS[*]} | workspace" >&2
	echo "  mode:    build | install | configure | run | all" >&2
	echo "  confirm: 1 when mode is run or all for Chimera products or --each run/all" >&2
	exit 1
}

chimera_clean_run_confirm_msg() {
	case "$PRODUCT" in
	gateway)
		echo "chimera-gateway-clean-run: removes data/gateway/ — stop the stack first; re-run with CONFIRM=1"
		;;
	supervisor)
		echo "chimera-supervisor-clean-run: removes run/, logs/, chimera/run/ and supervisor pid/log — stop the stack first; re-run with CONFIRM=1"
		;;
	broker)
		echo "chimera-broker-clean-run: removes data/chimera-broker/, data/bifrost/ — stop the stack first; re-run with CONFIRM=1"
		;;
	vectorstore)
		echo "chimera-vectorstore-clean-run: removes data/qdrant/ — stop the stack first; re-run with CONFIRM=1"
		;;
	indexer)
		echo "chimera-indexer-clean-run: removes data/gateway/indexer.* — stop the stack first; re-run with CONFIRM=1"
		;;
	*)
		echo "chimera-${PRODUCT}-clean-run: removes runtime state — stop the stack first; re-run with CONFIRM=1"
		;;
	esac
}

clean_each_confirm_msg() {
	case "$1" in
	all)
		echo "clean-all: removes workspace and all product artifacts (bin/, data/, .deps/, run/, logs/, dist/, generated config) — stop the stack first; re-run with CONFIRM=1"
		;;
	run)
		echo "clean-run: removes all product runtime state — stop the stack first; re-run with CONFIRM=1"
		;;
	*)
		echo "clean-each: mode must be run or all when confirmation is required" >&2
		return 1
		;;
	esac
}

want() {
	case "$MODE" in
	all) return 0 ;;
	"$1") return 0 ;;
	*) return 1 ;;
	esac
}

rm_paths() {
	local p removed=0
	for p in "$@"; do
		[[ -e "$p" || -L "$p" ]] || continue
		rm -rf "$p"
		removed=1
	done
	if [[ "$removed" -eq 1 ]]; then
		echo "clean-${PRODUCT}-${MODE}: removed $*"
	fi
}

if [[ "${1:-}" == "--each" ]]; then
	MODE="${2:-}"
	CONFIRM="${3:-}"
	[[ -n "$MODE" ]] || usage
	if [[ "$MODE" == "run" || "$MODE" == "all" ]]; then
		# shellcheck source=scripts/confirm.sh
		source "$ROOT/scripts/confirm.sh"
		require_confirm "$CONFIRM" "$(clean_each_confirm_msg "$MODE")"
	fi
	for product in "${CLEAN_PRODUCTS[@]}"; do
		CLEAN_BATCH=1 bash "$0" "$product" "$MODE" "$CONFIRM"
	done
	CLEAN_BATCH=1 bash "$0" workspace "$MODE" "$CONFIRM"
	echo "clean: finished cleaning ${MODE} for ${CLEAN_PRODUCTS[*]} and workspace"
	exit 0
fi

[[ -n "$PRODUCT" && -n "$MODE" ]] || usage

if want run && [[ "$PRODUCT" != "desktop" && "$PRODUCT" != "workspace" ]] && [[ -z "${CLEAN_BATCH:-}" ]]; then
	# shellcheck source=scripts/confirm.sh
	source "$ROOT/scripts/confirm.sh"
	require_confirm "$CONFIRM" "$(chimera_clean_run_confirm_msg)"
fi

case "$PRODUCT" in
workspace)
	if want all; then
		rm_paths \
			bin packaging/qdrant-bundles packages node_modules \
			.deps chimera/.deps run logs chimera/run dist
	elif want build; then
		rm_paths dist
	fi
	;;
gateway)
	if want build; then
		rm_paths \
			"chimera/bin/${CHIMERA_GATEWAY_BIN_BASE}" "chimera/bin/${CHIMERA_GATEWAY_BIN_BASE}.exe" \
			"bin/${CHIMERA_GATEWAY_BIN_BASE}" "bin/${CHIMERA_GATEWAY_BIN_BASE}.exe"
	fi
	if want install; then
		go clean -i ./chimera/chimera-gateway 2>/dev/null || true
		echo "clean-${PRODUCT}-${MODE}: go clean -i ./chimera/chimera-gateway"
	fi
	if want configure; then
		rm_paths "config/gateway.yaml"
	fi
	if want run; then
		rm_paths "data/gateway"
	fi
	;;
supervisor)
	if want build; then
		rm_paths \
			"chimera/bin/${CHIMERA_SUPERVISOR_BIN_BASE}" "chimera/bin/${CHIMERA_SUPERVISOR_BIN_BASE}.exe" \
			"bin/${CHIMERA_SUPERVISOR_BIN_BASE}" "bin/${CHIMERA_SUPERVISOR_BIN_BASE}.exe"
	fi
	if want install; then
		: # supervisor has no separate install target yet
	fi
	if want configure; then
		: # no generated supervisor config
	fi
	if want run; then
		rm_paths \
			"$(chimera_pid_path)" \
			"$(chimera_log_path)" \
			"chimera/run" \
			"run" \
			"logs"
	fi
	;;
broker)
	if want build; then
		rm_paths \
			"chimera/bin/${CHIMERA_BROKER_BIN_BASE}" "chimera/bin/${CHIMERA_BROKER_BIN_BASE}.exe" \
			"bin/${CHIMERA_BROKER_BIN_BASE}" "bin/${CHIMERA_BROKER_BIN_BASE}.exe" \
			"bin/bifrost-http" "bin/bifrost-http.exe"
	fi
	if want install; then
		rm_paths \
			"chimera/bin/bifrost-http" "chimera/bin/bifrost-http.exe" \
			"bin/bifrost-http" "bin/bifrost-http.exe" \
			"chimera/.deps/bifrost" \
			".deps/bifrost"
	fi
	if want configure; then
		: # chimera-broker.config.json is committed; no generated broker config
	fi
	if want run; then
		rm_paths "data/chimera-broker" "data/bifrost"
	fi
	;;
vectorstore)
	if want build; then
		rm_paths \
			"chimera/bin/${CHIMERA_VECTORSTORE_BIN_BASE}" "chimera/bin/${CHIMERA_VECTORSTORE_BIN_BASE}.exe" \
			"bin/${CHIMERA_VECTORSTORE_BIN_BASE}" "bin/${CHIMERA_VECTORSTORE_BIN_BASE}.exe" \
			"bin/qdrant" "bin/qdrant.exe"
	fi
	if want install; then
		rm_paths \
			"chimera/bin/qdrant" "chimera/bin/qdrant.exe" \
			"bin/qdrant" "bin/qdrant.exe" \
			"chimera/.deps/qdrant" \
			".deps/qdrant"
	fi
	if want configure; then
		: # there is no configuration that is generated for vectorstore
	fi
	if want run; then
		rm_paths "data/qdrant"
	fi
	;;
indexer)
	if want build; then
		rm_paths \
			"chimera/bin/${CHIMERA_INDEX_BIN_BASE}" "chimera/bin/${CHIMERA_INDEX_BIN_BASE}.exe" \
			"bin/${CHIMERA_INDEX_BIN_BASE}" "bin/${CHIMERA_INDEX_BIN_BASE}.exe"
	fi
	if want install; then
		go clean -i ./chimera/chimera-indexer 2>/dev/null || true
		echo "clean-${PRODUCT}-${MODE}: go clean -i ./chimera/chimera-indexer"
	fi
	if want configure; then
		rm_paths "config/indexer.yaml"
	fi
	if want run; then
		rm_paths \
			"data/gateway/indexer.supervised.yaml" \
			"data/gateway/indexer.sync-state.json"
	fi
	;;
desktop)
	if want build; then
		rm_paths \
			"locus/bin/${LOCUS_DESKTOP_BIN_BASE}" "locus/bin/${LOCUS_DESKTOP_BIN_BASE}.exe" \
			"bin/${LOCUS_DESKTOP_BIN_BASE}" "bin/${LOCUS_DESKTOP_BIN_BASE}.exe"
	fi
	if want install; then
		: # OS packages from locus-desktop-install are not removed here
	fi
	if want configure; then
		: # no generated locus desktop config at repo root
	fi
	if want run; then
		rm_paths "locus/run"
	fi
	;;
*)
	echo "clean-product: unknown product: $PRODUCT (expected: ${CLEAN_PRODUCTS[*]} or workspace)" >&2
	exit 1
	;;
esac
