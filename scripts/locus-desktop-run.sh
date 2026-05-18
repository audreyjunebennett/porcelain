#!/usr/bin/env bash
# make locus-desktop-run — ensure desktop binary exists, then exec with remaining args (-qdrant-bin …).
set -euo pipefail
root=$(cd "$(dirname "$0")/.." && pwd)
# shellcheck source=scripts/chimera-names.sh
source "$root/scripts/chimera-names.sh"
bin="${1:?locus-desktop-run.sh: missing binary name (e.g. ${LOCUS_DESKTOP_BIN_BASE}.exe)}"
make_cmd="${2:-make}"
runtime_bin_dir="${3:-${LOCUS_RUNTIME_BIN_DIR:-locus/bin}}"
shift 3 || true
cd "$root"
desktop_path="$root/$runtime_bin_dir/$bin"
if [[ ! -f "$desktop_path" ]]; then
	echo "${CHIMERA_MAKE_DESKTOP_RUN_TARGET}: $bin missing — building..."
	"$make_cmd" "${CHIMERA_MAKE_DESKTOP_BUILD_TARGET}"
fi
exec "$desktop_path" "$@"
