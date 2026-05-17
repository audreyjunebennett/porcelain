#!/usr/bin/env bash
# Shared bootstrap helper for Chimera installers: clone BiFrost at deps.lock ref, build bifrost-http, run qdrant-from-release.sh.
# Requires: git, curl, tar, make (or mingw32-make), unzip on Windows, Node.js 20+ (BiFrost UI),
# Go + CGO C compiler (gcc or clang on PATH). On Windows use Git Bash + MinGW-w64/MSYS2 gcc, or WSL.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck source=scripts/chimera-names.sh
source "$REPO_ROOT/scripts/chimera-names.sh"
# shellcheck source=deps-lock.sh
source "$REPO_ROOT/scripts/deps-lock.sh"

DEPS_DIR="${DEPS_DIR:-$REPO_ROOT/.deps}"
BIFROST_DIR="${BIFROST_DIR:-$DEPS_DIR/bifrost}"

QDRANT_RELEASE="$(deps_lock_get QDRANT_RELEASE)"
BIFROST_GIT_URL="$(deps_lock_get BIFROST_GIT_URL)"
BIFROST_GIT_REF="$(deps_lock_get BIFROST_GIT_REF)"

mkdir -p "$DEPS_DIR" "$REPO_ROOT/bin"

MAKE_BIN="${MAKE:-make}"
if ! command -v "$MAKE_BIN" >/dev/null 2>&1 && command -v mingw32-make >/dev/null 2>&1; then
	MAKE_BIN=mingw32-make
fi
if ! command -v "$MAKE_BIN" >/dev/null 2>&1; then
	echo "install-bootstrap: GNU make not found (tried \$MAKE, make, mingw32-make). Install build tools or set MAKE=…" >&2
	exit 1
fi

# BiFrost's Makefile runs $(MAKE) inside /bin/sh lines without quoting. If GNU make's
# argv0 / MAKE is under "Program Files (x86)" etc., '(' breaks the shell. Shorten
# to a mixed 8.3 path (Git Bash / MSYS cygpath -m -s) and export so recursive $(MAKE) is safe.
_make_short_for_bifrost() {
	local bin="$1" resolved short
	resolved="$(command -v "$bin" 2>/dev/null || true)"
	[[ -z "$resolved" ]] && resolved="$bin"
	if [[ "$resolved" != *" "* && "$resolved" != *"("* && "$resolved" != *")"* ]]; then
		printf '%s\n' "$resolved"
		return 0
	fi
	if command -v cygpath >/dev/null 2>&1; then
		short="$(cygpath -m -s "$resolved" 2>/dev/null || true)"
		if [[ -n "$short" ]]; then
			printf '%s\n' "$short"
			return 0
		fi
	fi
	echo "install-bootstrap: GNU make lives at a path with spaces/parentheses; cygpath could not shorten it." >&2
	echo "install-bootstrap: try MSYS2 make, or put make.exe on PATH from a directory without spaces (see docs/installation.md)." >&2
	printf '%s\n' "$resolved"
}
MAKE_BIN="$(_make_short_for_bifrost "$MAKE_BIN")"
export MAKE="$MAKE_BIN"

echo "==> BiFrost @ $BIFROST_GIT_REF from $BIFROST_GIT_URL -> $BIFROST_DIR"
if [[ ! -d "$BIFROST_DIR/.git" ]]; then
	git clone "$BIFROST_GIT_URL" "$BIFROST_DIR"
else
	echo "    (existing clone; fetching)"
	git -C "$BIFROST_DIR" remote set-url origin "$BIFROST_GIT_URL" 2>/dev/null || true
fi
git -C "$BIFROST_DIR" fetch origin
if ! git -C "$BIFROST_DIR" rev-parse -q --verify "${BIFROST_GIT_REF}^{commit}" >/dev/null 2>&1; then
	git -C "$BIFROST_DIR" fetch origin "$BIFROST_GIT_REF"
fi
git -C "$BIFROST_DIR" checkout -q "$BIFROST_GIT_REF"

command -v node >/dev/null 2>&1 || {
	echo "install-bootstrap: install Node.js 20+ and ensure it is on PATH (BiFrost UI build)." >&2
	exit 1
}
node_major="$(node -p "parseInt(process.versions.node.split('.')[0],10)" 2>/dev/null || echo 0)"
if [[ "$node_major" -lt 20 ]]; then
	echo "install-bootstrap: BiFrost needs Node.js >= 20; found $(node -v 2>/dev/null)." >&2
	exit 1
fi

# BiFrost's default `make build` sets GOWORK=off and compiles against published
# modules (e.g. framework v1.2.x). The clone's transports code can drift ahead
# (e.g. DefaultClientConfig uses fields only present in the local framework tree).
# setup-workspace + LOCAL=1 builds with repo-root go.work so local modules match.
echo "==> Go workspace + build in BiFrost (may run npm ci in ui/)"
"$MAKE_BIN" -C "$BIFROST_DIR" setup-workspace
"$MAKE_BIN" -C "$BIFROST_DIR" build LOCAL=1
BF_ART="$BIFROST_DIR/tmp/bifrost-http"
BF_DST="$REPO_ROOT/bin/bifrost-http"
GOOS="$(go env GOOS)"
if [[ -f "${BF_ART}.exe" ]]; then
	cp -f "${BF_ART}.exe" "${BF_DST}.exe"
	chmod +x "${BF_DST}.exe" 2>/dev/null || true
	# On MSYS/Git Bash, `rm bin/bifrost-http` can remove bin/bifrost-http.exe — do not rm after .exe install.
	if [[ "$GOOS" != windows ]]; then
		rm -f "$BF_DST"
	fi
	echo "    installed ${BF_DST}.exe"
	BF_INSTALLED="${BF_DST}.exe"
elif [[ -f "$BF_ART" ]]; then
	# MinGW/MSYS Go often writes tmp/bifrost-http with no .exe; Windows CreateProcess needs .exe.
	if [[ "$GOOS" == windows ]]; then
		cp -f "$BF_ART" "${BF_DST}.exe"
		chmod +x "${BF_DST}.exe" 2>/dev/null || true
		# Same as above: never rm extensionless name on Windows — it deletes the .exe we just copied.
		echo "    installed ${BF_DST}.exe (from tmp/bifrost-http)"
		BF_INSTALLED="${BF_DST}.exe"
	else
		cp -f "$BF_ART" "$BF_DST"
		chmod +x "$BF_DST" 2>/dev/null || true
		echo "    installed $BF_DST"
		BF_INSTALLED="$BF_DST"
	fi
else
	echo "install-bootstrap: no $BF_ART or ${BF_ART}.exe after BiFrost build (CGO often needs gcc on PATH)." >&2
	echo "install-bootstrap: install gcc/clang, then: make ${CHIMERA_MAKE_INSTALL_TARGET}   (see docs/installation.md#c-compiler-cgo)" >&2
	ls -la "$BIFROST_DIR/tmp" 2>/dev/null || echo "    (tmp/ missing or empty)" >&2
	exit 1
fi

echo "==> Qdrant $QDRANT_RELEASE -> bin/"
bash "$REPO_ROOT/scripts/qdrant-from-release.sh"

QD_INSTALLED="$REPO_ROOT/bin/qdrant"
[[ -f "$REPO_ROOT/bin/qdrant.exe" ]] && QD_INSTALLED="$REPO_ROOT/bin/qdrant.exe"

echo ""
echo "Done. Binaries: $BF_INSTALLED  $QD_INSTALLED"
if [[ "$BF_INSTALLED" == *.exe ]] || [[ "$QD_INSTALLED" == *.exe ]]; then
	echo "On Windows, run ${CHIMERA_SUPERVISOR_BIN_BASE} with e.g. -broker-backend-bin ./bin/bifrost-http.exe -vectorstore-backend-bin ./bin/qdrant.exe"
fi
echo "BiFrost checkout: $BIFROST_DIR (bump BIFROST_GIT_REF in deps.lock and re-run: make ${CHIMERA_MAKE_INSTALL_TARGET})"
