#!/usr/bin/env bash
# Full local bundle: desktop UI + gateway headless binary + bifrost-http + qdrant + config (make package).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
# shellcheck source=scripts/chimera-names.sh
source "$ROOT/scripts/chimera-names.sh"

goos="$(go env GOOS)"
goarch="$(go env GOARCH)"
name="${CHIMERA_DIST_BUNDLE_PREFIX}_${goos}_${goarch}"
OUT="$ROOT/dist/personal/$name"
rm -rf "$OUT"
mkdir -p "$OUT/config"

ext=""
if [[ "$goos" == "windows" ]]; then
	ext=".exe"
fi

DESKTOP_BIN="${1:-}"
if [[ -z "$DESKTOP_BIN" ]]; then
	DESKTOP_BIN="${CHIMERA_DESKTOP_BIN_BASE}${ext}"
fi

if [[ ! -f "$ROOT/$DESKTOP_BIN" ]]; then
	echo "package: building $DESKTOP_BIN (CGO + -tags desktop)..."
	bash "$ROOT/scripts/desktop-build.sh" "$DESKTOP_BIN"
fi

BIF="bifrost-http${ext}"
QDR="qdrant${ext}"
if [[ ! -f "$ROOT/bin/$BIF" ]]; then
	echo "package: missing bin/$BIF — run: make ${CHIMERA_MAKE_INSTALL_TARGET}" >&2
	exit 1
fi
if [[ ! -f "$ROOT/bin/$QDR" ]]; then
	echo "package: missing bin/$QDR — run: make ${CHIMERA_MAKE_INSTALL_TARGET}" >&2
	exit 1
fi

gw_out="${CHIMERA_GATEWAY_BIN_BASE}${ext}"
cp "$ROOT/$DESKTOP_BIN" "$OUT/$gw_out"
cp "$ROOT/bin/$BIF" "$OUT/"
cp "$ROOT/bin/$QDR" "$OUT/"

cp "$ROOT/config/gateway.example.yaml" "$OUT/config/gateway.yaml"
cp "$ROOT/config/tokens.example.yaml" "$OUT/config/tokens.example.yaml"
cp "$ROOT/config/bifrost.config.json" "$OUT/config/bifrost.config.json"
cp "$ROOT/config/routing-policy.yaml" "$OUT/config/routing-policy.yaml"
cp "$ROOT/config/provider-free-tier.yaml" "$OUT/config/provider-free-tier.yaml"
cp "$ROOT/env.example" "$OUT/env.example"

readme_tmp="$OUT/README.txt.tmp"
{
	echo "Personal bundle (make package)"
	echo
	echo "1. Copy env.example to .env in this folder and add provider keys."
	echo "2. First run: start ${CHIMERA_GATEWAY_BIN_BASE} (double-click or run ./${gw_out}) — setup opens in the browser to create config/tokens.yaml (or copy config/tokens.example.yaml to config/tokens.yaml yourself)."
	echo "3. Restart ${CHIMERA_GATEWAY_BIN_BASE} and use the gateway token from setup when your client asks for it."
	echo
} >"$readme_tmp"
mv -f "$readme_tmp" "$OUT/README.txt"

echo "package: wrote $OUT"
