#!/usr/bin/env bash
# Idempotent qdrant-only install path for vectorstore wrapper flows.
# Verifies toolchain needed for fetch/unpack and installs qdrant from deps.lock.
set -euo pipefail
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"
# shellcheck source=scripts/chimera-names.sh
source "$REPO_ROOT/scripts/chimera-names.sh"
# shellcheck source=scripts/install-toolchain-deps.sh
source "$REPO_ROOT/scripts/install-toolchain-deps.sh"
DEPS_DIR="${DEPS_DIR:-$REPO_ROOT/.deps}"
QDRANT_BIN_DIR="${QDRANT_BIN_DIR:-$REPO_ROOT/bin}"
QDRANT_DEPS_DIR="${QDRANT_DEPS_DIR:-$DEPS_DIR/qdrant}"

echo "==> chimera-vectorstore-install: toolchain"
missing=0

toolchain_ensure_git || missing=1

if ! command -v curl >/dev/null 2>&1; then
	echo "    MISSING  curl is required to fetch qdrant releases" >&2
	missing=1
else
	echo "    OK  curl -> $(command -v curl)"
fi

if [ "$missing" -ne 0 ]; then
	echo "" >&2
	echo "chimera-vectorstore-install: install missing tools, then re-run: make ${CHIMERA_MAKE_VECTORSTORE_INSTALL_TARGET}" >&2
	exit 1
fi

if [[ "${FORCE:-}" != "1" ]] && { [ -f "$QDRANT_BIN_DIR/qdrant" ] || [ -f "$QDRANT_BIN_DIR/qdrant.exe" ]; }; then
	echo "==> chimera-vectorstore-install: existing qdrant artifact detected ($QDRANT_BIN_DIR/qdrant[.exe]); skipping download"
	echo "    set FORCE=1 to refresh from deps.lock release pin"
else
	echo "==> chimera-vectorstore-install: qdrant (deps.lock)"
	export QDRANT_BIN_DIR
	bash "$REPO_ROOT/scripts/chimera-vectorstore-qdrant-install.sh"
fi

mkdir -p "$QDRANT_DEPS_DIR/bin"
if [ -f "$QDRANT_BIN_DIR/qdrant.exe" ]; then
	cp -f "$QDRANT_BIN_DIR/qdrant.exe" "$QDRANT_DEPS_DIR/bin/qdrant.exe"
elif [ -f "$QDRANT_BIN_DIR/qdrant" ]; then
	cp -f "$QDRANT_BIN_DIR/qdrant" "$QDRANT_DEPS_DIR/bin/qdrant"
	chmod +x "$QDRANT_DEPS_DIR/bin/qdrant" 2>/dev/null || true
fi

echo "==> chimera-vectorstore-install: artifacts"
found=0
for f in "$QDRANT_BIN_DIR/qdrant" "$QDRANT_BIN_DIR/qdrant.exe"; do
	if [ -f "$f" ]; then
		echo "    OK  $f"
		found=1
	fi
done
if [ "$found" -eq 0 ]; then
	echo "    WARN  no qdrant binary under $QDRANT_BIN_DIR -- check install output above" >&2
fi

echo ""
echo "chimera-vectorstore-install: deps cache -> $QDRANT_DEPS_DIR/bin"
echo "chimera-vectorstore-install: done."
