#!/usr/bin/env bash
# Smoke-test GoReleaser dist/ binaries for the current host (CI release-build and release workflows).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

smoke_goos() {
	if [[ -n "${SMOKE_GOOS:-}" ]]; then
		echo "$SMOKE_GOOS"
		return
	fi
	case "$(uname -s)" in
		Linux) echo linux ;;
		Darwin) echo darwin ;;
		MINGW* | MSYS* | CYGWIN* | Windows_NT) echo windows ;;
		*)
			echo "ci-release-smoke: unsupported host OS $(uname -s)" >&2
			exit 1
			;;
	esac
}

smoke_goarch() {
	if [[ -n "${SMOKE_GOARCH:-}" ]]; then
		echo "$SMOKE_GOARCH"
		return
	fi
	case "$(uname -m)" in
		x86_64 | amd64) echo amd64 ;;
		aarch64 | arm64) echo arm64 ;;
		*)
			echo "ci-release-smoke: unsupported host arch $(uname -m)" >&2
			exit 1
			;;
	esac
}

GOOS="$(smoke_goos)"
GOARCH="$(smoke_goarch)"
echo "ci-release-smoke: host ${GOOS}/${GOARCH}"

bins=(chimera-gateway chimera-broker chimera-supervisor chimera-vectorstore chimera-indexer)
for name in "${bins[@]}"; do
	bin=""
	if [[ "$GOOS" == windows ]]; then
		bin=$(find dist -type f -path "*_${GOOS}_${GOARCH}*/${name}.exe" 2>/dev/null | head -1)
	else
		bin=$(find dist -type f -path "*_${GOOS}_${GOARCH}*/${name}" ! -name '*.exe' 2>/dev/null | head -1)
	fi
	if [[ -z "$bin" || ! -x "$bin" ]]; then
		echo "ci-release-smoke: missing executable for ${name} (${GOOS}/${GOARCH}) under dist/" >&2
		exit 1
	fi
	echo "ci-release-smoke: ${name} -> ${bin}"
	"$bin" -version
done
echo "ci-release-smoke: ok (${#bins[@]} binaries, ${GOOS}/${GOARCH})"
