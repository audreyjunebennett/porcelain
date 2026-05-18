#!/usr/bin/env bash
# Grep inventory for chimera-gateway legacy vocabulary (Phase 1 — no behavior change).
# Usage: scripts/chimera-gateway-legacy-vocab-inventory.sh
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
GW="${ROOT}/chimera/chimera-gateway"

if [[ ! -d "${GW}" ]]; then
	echo "missing ${GW}" >&2
	exit 1
fi

echo "chimera-gateway legacy vocabulary inventory"
echo "root: ${GW}"
echo

for term in bifrost upstream qdrant BiFrost; do
	echo "=== ${term} (case-insensitive word boundary) ==="
	if command -v rg >/dev/null 2>&1; then
		rg -i "\\b${term}\\b" "${GW}" --stats --no-heading 2>/dev/null || true
		echo "files:"
		rg -i "\\b${term}\\b" "${GW}" -l 2>/dev/null | sort || true
	else
		grep -riE "\\b${term}\\b" "${GW}" 2>/dev/null | wc -l | awk '{print $1 " matches (grep)"}'
	fi
	echo
done
