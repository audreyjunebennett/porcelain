#!/usr/bin/env bash
# Smoke-test GoReleaser dist/ binaries (CI release-build and release workflows).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

bins=(chimera-gateway chimera-broker chimera-supervisor chimera-vectorstore chimera-indexer)
for name in "${bins[@]}"; do
	bin=$(find dist -type f -path "*/${name}" ! -name '*.exe' 2>/dev/null | head -1)
	if [[ -z "$bin" ]]; then
		bin=$(find dist -type f -name "${name}.exe" 2>/dev/null | head -1)
	fi
	if [[ -z "$bin" || ! -x "$bin" ]]; then
		echo "ci-release-smoke: missing executable for ${name} under dist/" >&2
		exit 1
	fi
	echo "ci-release-smoke: ${name} -> ${bin}"
	"$bin" -version
done
echo "ci-release-smoke: ok (${#bins[@]} binaries)"
