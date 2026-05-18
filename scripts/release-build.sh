#!/usr/bin/env bash
# GoReleaser snapshot build (make release-build). Run under Git Bash on Windows.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
# shellcheck source=scripts/chimera-names.sh
source "$ROOT/scripts/chimera-names.sh"

if command -v go >/dev/null 2>&1; then
	_gobin="${GOBIN:-}"
	if [[ -z "${_gobin//[[:space:]]/}" ]]; then
		_gobin="$(go env GOPATH)/bin"
	fi
	_gobin="${_gobin//\\//}"
	export PATH="${_gobin}:$PATH"
	hash -r 2>/dev/null || true
fi
if ! command -v goreleaser >/dev/null 2>&1; then
	echo "release-build: run: make ${RELEASE_MAKE_INSTALL_TARGET}   (or https://goreleaser.com/install/ / docs/packaging.md)" >&2
	exit 1
fi
exec goreleaser release --snapshot --clean
