#!/usr/bin/env bash
# Idempotent bifrost-only install path for broker wrapper flows.
# Verifies toolchain (auto-install git/make/go/node/gcc when possible), then installs bifrost from deps.lock.
# Skip auto-install: SKIP_AUTO_GIT, SKIP_AUTO_MAKE, SKIP_AUTO_GO, SKIP_AUTO_NODE, SKIP_AUTO_GCC.
# BIFROST_SKIP_UI=1: build bifrost-http without Next.js (no Node required; stub embedded UI).
set -euo pipefail
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"
# shellcheck source=scripts/chimera-names.sh
source "$REPO_ROOT/scripts/chimera-names.sh"
# shellcheck source=scripts/compiler-detect.sh
source "$REPO_ROOT/scripts/compiler-detect.sh"
# shellcheck source=scripts/install-toolchain-deps.sh
source "$REPO_ROOT/scripts/install-toolchain-deps.sh"
CHIMERA_BROKER_BIN_DIR="${CHIMERA_BROKER_BIN_DIR:-$REPO_ROOT/bin}"

echo "==> chimera-broker-install: toolchain"
missing=0

toolchain_ensure_git || missing=1
toolchain_ensure_make || missing=1
toolchain_ensure_go || missing=1
if [[ "${BIFROST_SKIP_UI:-}" == "1" ]]; then
	echo "    SKIP  node (BIFROST_SKIP_UI=1 — bifrost-http without Next.js UI)"
else
	toolchain_ensure_node || missing=1
fi

# BiFrost's bifrost-http binary is built with CGO; Go needs a C toolchain (gcc or clang on PATH).
if has_cc; then
	echo "    OK  C compiler -> $(cc_on_path)"
else
	if [[ "${SKIP_AUTO_GCC:-}" == "1" ]]; then
		echo "    (no gcc/clang -- sourcing scripts/install-gcc.sh; SKIP_AUTO_GCC=1 skips auto-install)" >&2
	else
		echo "    (no gcc/clang -- sourcing scripts/install-gcc.sh)"
	fi
	# shellcheck source=scripts/install-gcc.sh
	if source "$REPO_ROOT/scripts/install-gcc.sh"; then
		if has_cc; then
			echo "    OK  C compiler -> $(cc_on_path)"
		else
			echo "    MISSING  gcc or clang after auto-install -- open a new shell or see docs/installation.md#c-compiler-cgo" >&2
			missing=1
		fi
	else
		echo "    MISSING  gcc or clang (auto-install failed or SKIP_AUTO_GCC=1 -- see docs/installation.md#c-compiler-cgo)" >&2
		missing=1
	fi
fi

if [ "$missing" -ne 0 ]; then
	echo "" >&2
	echo "chimera-broker-install: install missing tools, then re-run: make ${CHIMERA_MAKE_BROKER_INSTALL_TARGET}" >&2
	exit 1
fi

if [[ "${FORCE:-}" != "1" ]] && { [ -f "$CHIMERA_BROKER_BIN_DIR/bifrost-http" ] || [ -f "$CHIMERA_BROKER_BIN_DIR/bifrost-http.exe" ]; }; then
	echo "==> chimera-broker-install: existing bifrost artifact detected ($CHIMERA_BROKER_BIN_DIR/bifrost-http[.exe]); skipping rebuild"
	echo "    set FORCE=1 to rebuild from deps.lock checkout"
else
	echo "==> chimera-broker-install: BiFrost (deps.lock)"
	export BIFROST_BIN_DIR="$CHIMERA_BROKER_BIN_DIR"
	bash "$REPO_ROOT/scripts/chimera-broker-bifrost-install.sh"
fi

echo "==> chimera-broker-install: artifacts"
found=0
for f in "$CHIMERA_BROKER_BIN_DIR/bifrost-http" "$CHIMERA_BROKER_BIN_DIR/bifrost-http.exe"; do
	if [ -f "$f" ]; then
		echo "    OK  $f"
		found=1
	fi
done
if [ "$found" -eq 0 ]; then
	echo "    WARN  no bifrost-http under $CHIMERA_BROKER_BIN_DIR -- check bootstrap output above" >&2
fi

echo ""
echo "chimera-broker-install: done."
