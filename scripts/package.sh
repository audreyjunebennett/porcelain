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
mkdir -p "$OUT/locus/bin" "$OUT/locus/config" "$OUT/locus/run"

ext=""
if [[ "$goos" == "windows" ]]; then
	ext=".exe"
fi

DESKTOP_BIN="${1:-}"
if [[ -z "$DESKTOP_BIN" ]]; then
	DESKTOP_BIN="${LOCUS_DESKTOP_BIN_BASE}${ext}"
fi

if [[ ! -f "$ROOT/locus/bin/$DESKTOP_BIN" ]]; then
	echo "package: building $DESKTOP_BIN (CGO + -tags desktop)..."
	bash "$ROOT/scripts/locus-desktop-build.sh" "$DESKTOP_BIN"
fi

BIF="bifrost-http${ext}"
QDR="qdrant${ext}"
runtime_bif="$ROOT/chimera/bin/$BIF"
runtime_qdr="$ROOT/chimera/bin/$QDR"
if [[ ! -f "$runtime_bif" ]]; then
	runtime_bif="$ROOT/bin/$BIF"
fi
if [[ ! -f "$runtime_qdr" ]]; then
	runtime_qdr="$ROOT/bin/$QDR"
fi
if [[ ! -f "$runtime_bif" ]]; then
	echo "package: missing $BIF in chimera/bin or bin — run: make ${CHIMERA_MAKE_INSTALL_TARGET}" >&2
	exit 1
fi
if [[ ! -f "$runtime_qdr" ]]; then
	echo "package: missing $QDR in chimera/bin or bin — run: make ${CHIMERA_MAKE_INSTALL_TARGET}" >&2
	exit 1
fi

SUPERVISOR_BIN="${CHIMERA_SUPERVISOR_BIN_BASE}${ext}"
supervisor_path="$ROOT/chimera/bin/$SUPERVISOR_BIN"
if [[ ! -f "$supervisor_path" ]]; then
	echo "package: building $SUPERVISOR_BIN..."
	go build -o "$supervisor_path" "./${CHIMERA_CMD_SUPERVISOR}"
fi

cp "$supervisor_path" "$OUT/locus/bin/$SUPERVISOR_BIN"
cp "$ROOT/locus/bin/$DESKTOP_BIN" "$OUT/locus/bin/${LOCUS_DESKTOP_BIN_BASE}${ext}"
cp "$ROOT/chimera/bin/${CHIMERA_BROKER_BIN_BASE}${ext}" "$OUT/locus/bin/${CHIMERA_BROKER_BIN_BASE}${ext}" 2>/dev/null || true
cp "$ROOT/chimera/bin/${CHIMERA_VECTORSTORE_BIN_BASE}${ext}" "$OUT/locus/bin/${CHIMERA_VECTORSTORE_BIN_BASE}${ext}" 2>/dev/null || true
cp "$runtime_bif" "$OUT/locus/bin/$BIF"
cp "$runtime_qdr" "$OUT/locus/bin/$QDR"

cp "$ROOT/config/gateway.example.yaml" "$OUT/locus/config/gateway.yaml"
cp "$ROOT/config/api-keys.example.yaml" "$OUT/locus/config/api-keys.example.yaml"
cp "$ROOT/config/bifrost.config.json" "$OUT/locus/config/bifrost.config.json"
cp "$ROOT/config/routing-policy.yaml" "$OUT/locus/config/routing-policy.yaml"
cp "$ROOT/config/provider-free-tier.yaml" "$OUT/locus/config/provider-free-tier.yaml"
cp "$ROOT/env.example" "$OUT/locus/env.example"

readme_tmp="$OUT/README.txt.tmp"
{
	echo "Personal bundle (make package)"
	echo
	echo "1. Copy locus/env.example to locus/.env and add provider keys."
	echo "2. First run: start locus/bin/${LOCUS_DESKTOP_BIN_BASE}${ext} (double-click or run ./locus/bin/${LOCUS_DESKTOP_BIN_BASE}${ext})."
	echo "3. Runtime root is locus/; config/data/logs/run resolve under that root for desktop launch."
	echo
} >"$readme_tmp"
mv -f "$readme_tmp" "$OUT/README.txt"

echo "package: wrote $OUT"
