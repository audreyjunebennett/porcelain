#!/usr/bin/env bash
# make desktop-run — ensure desktop binary exists, then exec with remaining args (e.g. desktop -qdrant-bin …).
set -euo pipefail
root=$(cd "$(dirname "$0")/.." && pwd)
# shellcheck source=scripts/chimera-names.sh
source "$root/scripts/chimera-names.sh"
bin="${1:?desktop-run.sh: missing binary name (e.g. ${CHIMERA_DESKTOP_BIN_BASE}.exe)}"
make_cmd="${2:-make}"
shift 2 || true
cd "$root"
if [[ ! -f "$bin" ]]; then
	echo "desktop-run: $bin missing — building..."
	"$make_cmd" desktop-build
fi
exec "$root/$bin" "$@"
