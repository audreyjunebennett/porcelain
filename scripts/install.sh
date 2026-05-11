#!/usr/bin/env bash
# Idempotent: verify toolchain (auto-install git/make/go/node/gcc when possible), then install-bootstrap.sh (BiFrost + Qdrant from deps.lock).
# Skip auto-install: SKIP_AUTO_GIT, SKIP_AUTO_MAKE, SKIP_AUTO_GO, SKIP_AUTO_NODE, SKIP_AUTO_GCC (see scripts/install-toolchain-deps.sh, install-gcc.sh).
set -euo pipefail
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"
# shellcheck source=scripts/chimera-names.sh
source "$REPO_ROOT/scripts/chimera-names.sh"
# shellcheck source=scripts/compiler-detect.sh
source "$REPO_ROOT/scripts/compiler-detect.sh"
# shellcheck source=scripts/install-toolchain-deps.sh
source "$REPO_ROOT/scripts/install-toolchain-deps.sh"

echo "==> install: toolchain"
missing=0

toolchain_ensure_git || missing=1
toolchain_ensure_make || missing=1
toolchain_ensure_go || missing=1
toolchain_ensure_node || missing=1

# BiFrost's bifrost-http binary is built with CGO; Go needs a C toolchain (gcc or clang on PATH).
if has_cc; then
	echo "    OK  C compiler → $(cc_on_path)"
else
	if [[ "${SKIP_AUTO_GCC:-}" == "1" ]]; then
		echo "    (no gcc/clang — sourcing scripts/install-gcc.sh; SKIP_AUTO_GCC=1 skips auto-install)" >&2
	else
		echo "    (no gcc/clang — sourcing scripts/install-gcc.sh)"
	fi
	# shellcheck source=scripts/install-gcc.sh
	if source "$REPO_ROOT/scripts/install-gcc.sh"; then
		if has_cc; then
			echo "    OK  C compiler → $(cc_on_path)"
		else
			echo "    MISSING  gcc or clang after auto-install — open a new shell or see docs/installation.md#c-compiler-cgo" >&2
			missing=1
		fi
	else
		echo "    MISSING  gcc or clang (auto-install failed or SKIP_AUTO_GCC=1 — see docs/installation.md#c-compiler-cgo)" >&2
		missing=1
	fi
fi

if [ "$missing" -ne 0 ]; then
	echo "" >&2
	echo "install: install missing tools, then re-run: make ${CHIMERA_MAKE_INSTALL_TARGET}" >&2
	exit 1
fi

echo "==> install: BiFrost + Qdrant (deps.lock)"
bash "$REPO_ROOT/scripts/install-bootstrap.sh"

echo "==> install: artifacts"
found=0
for f in bin/bifrost-http bin/bifrost-http.exe bin/qdrant bin/qdrant.exe; do
	if [ -f "$f" ]; then
		echo "    OK  $f"
		found=1
	fi
done
if [ "$found" -eq 0 ]; then
	echo "    WARN  no bifrost-http or qdrant under bin/ — check bootstrap output above" >&2
fi

echo ""
echo "install: done. Next:"
echo "    make configure   # seed config/gateway.yaml from gateway.example.yaml if missing; tokens.yaml via /ui/setup or manual copy"
