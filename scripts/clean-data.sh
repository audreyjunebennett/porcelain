#!/usr/bin/env bash
# Remove supervised BiFrost and Qdrant data dirs (defaults when running the stack; see Makefile clean-data / make chimera-serve).
# First argument must be 1 (from make CONFIRM=1); avoids relying on Make's default shell for `test`.
set -euo pipefail
if [[ "${1:-}" != "1" ]]; then
	echo "clean-data: removes data/bifrost/, data/qdrant/, data/gateway/ — stop the stack first if running; re-run with CONFIRM=1" >&2
	exit 1
fi
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
rm -rf data/bifrost data/qdrant data/gateway
echo "clean-data: removed data/bifrost/, data/qdrant/, data/gateway/ (BiFrost + Qdrant + gateway metrics start empty on next serve)"
