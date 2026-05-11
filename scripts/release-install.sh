#!/usr/bin/env bash
# Install tools for make release-snapshot (GoReleaser v2 + Qdrant fetch hook deps). Idempotent.
# Cross-compilation is pure Go (CGO_ENABLED=0); this does not install a C toolchain.
set -euo pipefail
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"
# shellcheck source=scripts/chimera-names.sh
source "$REPO_ROOT/scripts/chimera-names.sh"

if ! command -v go >/dev/null 2>&1; then
	echo "release-install: go not on PATH — run: make ${CHIMERA_MAKE_INSTALL_TARGET}" >&2
	exit 1
fi

_ensure_curl_tar_unzip() {
	local os
	os="$(uname -s 2>/dev/null || echo unknown)"
	local miss=()
	command -v curl >/dev/null 2>&1 || miss+=("curl")
	command -v tar >/dev/null 2>&1 || miss+=("tar")
	command -v unzip >/dev/null 2>&1 || miss+=("unzip")
	if [[ "${#miss[@]}" -eq 0 ]]; then
		return 0
	fi
	if [[ "$os" == "Linux" ]] && [[ -f /etc/debian_version ]]; then
		echo "release-install: installing ${miss[*]} (Debian/Ubuntu) for scripts/release-snapshot-qdrant.sh..."
		sudo apt-get update
		sudo apt-get install -y curl tar unzip
		return 0
	fi
	echo "release-install: missing: ${miss[*]} (needed to download Qdrant bundles during release-snapshot)" >&2
	if [[ "$os" == "Darwin" ]]; then
		echo "release-install: macOS: install Xcode Command Line Tools (xcode-select --install)" >&2
	elif [[ "$os" == "Linux" ]]; then
		echo "release-install: Linux: install curl, tar, unzip with your package manager" >&2
	else
		echo "release-install: Windows: use Git for Windows bash (includes curl, tar, unzip in usr/bin)" >&2
	fi
	exit 1
}

_ensure_curl_tar_unzip

_bin_dir=""
if [[ -n "${GOBIN:-}" ]]; then
	_bin_dir="${GOBIN//\\//}"
else
	_bin_dir="$(go env GOPATH)/bin"
	_bin_dir="${_bin_dir//\\//}"
fi
export PATH="${_bin_dir}:$PATH"
hash -r 2>/dev/null || true

echo "release-install: go install github.com/goreleaser/goreleaser/v2@latest → ${_bin_dir}"
go install github.com/goreleaser/goreleaser/v2@latest
hash -r 2>/dev/null || true

if ! command -v goreleaser >/dev/null 2>&1; then
	echo "release-install: goreleaser not found after install — add to PATH: ${_bin_dir}" >&2
	echo "release-install: open a new shell, or: export PATH=\"${_bin_dir}:\$PATH\"" >&2
	exit 1
fi

case "$(uname -s 2>/dev/null || echo unknown)" in
MINGW* | MSYS* | CYGWIN*)
	# shellcheck source=win-persist-user-path.sh
	source "$REPO_ROOT/scripts/win-persist-user-path.sh"
	win_persist_user_path_dir "$_bin_dir"
	;;
esac

echo "release-install: done. Next: make release-snapshot"
