#!/usr/bin/env bash
# Personal desktop bundle: Locus UI + full Chimera stack + runtime deps (make release-package).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
# shellcheck source=scripts/chimera-names.sh
source "$ROOT/scripts/chimera-names.sh"

goos="$(go env GOOS)"
goarch="$(go env GOARCH)"
bundle="${CHIMERA_DIST_BUNDLE_PREFIX}_${goos}_${goarch}"
OUT="$ROOT/dist/personal/$bundle"
CHIMERA_BIN="$ROOT/chimera/bin"
STAGE_BIN="$ROOT/bin"
OUT_BIN="$OUT/locus/bin"

ext=""
if [[ "$goos" == "windows" ]]; then
	ext=".exe"
fi

DESKTOP_BIN="${1:-}"
if [[ -z "$DESKTOP_BIN" ]]; then
	DESKTOP_BIN="${LOCUS_DESKTOP_BIN_BASE}${ext}"
fi

_resolve_runtime() {
	local name="$1"
	if [[ -f "$CHIMERA_BIN/$name" ]]; then
		printf '%s\n' "$CHIMERA_BIN/$name"
		return 0
	fi
	if [[ -f "$STAGE_BIN/$name" ]]; then
		printf '%s\n' "$STAGE_BIN/$name"
		return 0
	fi
	return 1
}

_ensure_go_binary() {
	local base="$1"
	local pkg="$2"
	local out="$CHIMERA_BIN/${base}${ext}"
	if [[ -f "$out" ]]; then
		printf '%s\n' "$out"
		return 0
	fi
	echo "release-package: building ${base}${ext}..."
	mkdir -p "$CHIMERA_BIN"
	go build -o "$out" "./${pkg}"
	printf '%s\n' "$out"
}

_require_runtime() {
	local name="$1"
	local hint="$2"
	if path="$(_resolve_runtime "$name")"; then
		printf '%s\n' "$path"
		return 0
	fi
	echo "release-package: missing $name in chimera/bin or bin — run: $hint" >&2
	exit 1
}

rm -rf "$OUT"
mkdir -p "$OUT_BIN" "$OUT/locus/config" "$OUT/locus/run"

if [[ ! -f "$ROOT/locus/bin/$DESKTOP_BIN" ]]; then
	echo "release-package: building $DESKTOP_BIN (CGO + -tags desktop)..."
	bash "$ROOT/scripts/locus-desktop-build.sh" "$DESKTOP_BIN"
fi

BIF="bifrost-http${ext}"
QDR="qdrant${ext}"
bifrost_path="$(_require_runtime "$BIF" "make ${CHIMERA_MAKE_BROKER_INSTALL_TARGET}")"
qdrant_path="$(_require_runtime "$QDR" "make ${CHIMERA_MAKE_VECTORSTORE_INSTALL_TARGET}")"

gateway_path="$(_ensure_go_binary "$CHIMERA_GATEWAY_BIN_BASE" "$CHIMERA_CMD_GATEWAY")"
broker_path="$(_ensure_go_binary "$CHIMERA_BROKER_BIN_BASE" "$CHIMERA_CMD_BROKER")"
vectorstore_path="$(_ensure_go_binary "$CHIMERA_VECTORSTORE_BIN_BASE" "$CHIMERA_CMD_VECTORSTORE")"
supervisor_path="$(_ensure_go_binary "$CHIMERA_SUPERVISOR_BIN_BASE" "$CHIMERA_CMD_SUPERVISOR")"
indexer_path="$(_ensure_go_binary "$CHIMERA_INDEX_BIN_BASE" "$CHIMERA_CMD_INDEXER")"

cp "$gateway_path" "$OUT_BIN/${CHIMERA_GATEWAY_BIN_BASE}${ext}"
cp "$broker_path" "$OUT_BIN/${CHIMERA_BROKER_BIN_BASE}${ext}"
cp "$vectorstore_path" "$OUT_BIN/${CHIMERA_VECTORSTORE_BIN_BASE}${ext}"
cp "$supervisor_path" "$OUT_BIN/${CHIMERA_SUPERVISOR_BIN_BASE}${ext}"
cp "$indexer_path" "$OUT_BIN/${CHIMERA_INDEX_BIN_BASE}${ext}"
cp "$ROOT/locus/bin/$DESKTOP_BIN" "$OUT_BIN/${LOCUS_DESKTOP_BIN_BASE}${ext}"
cp "$bifrost_path" "$OUT_BIN/$BIF"
cp "$qdrant_path" "$OUT_BIN/$QDR"

cp "$ROOT/config/gateway.example.yaml" "$OUT/locus/config/gateway.yaml"
cp "$ROOT/config/api-keys.example.yaml" "$OUT/locus/config/api-keys.example.yaml"
cp "$ROOT/config/chimera-broker.config.json" "$OUT/locus/config/chimera-broker.config.json"
cp "$ROOT/config/routing-policy.yaml" "$OUT/locus/config/routing-policy.yaml"
cp "$ROOT/config/provider-free-tier.yaml" "$OUT/locus/config/provider-free-tier.yaml"
cp "$ROOT/config/indexer.example.yaml" "$OUT/locus/config/indexer.yaml"
cp "$ROOT/config/provider-model-limits.example.yaml" "$OUT/locus/config/provider-model-limits.yaml"
cp "$ROOT/env.example" "$OUT/locus/env.example"

readme_tmp="$OUT/README.txt.tmp"
{
	echo "Personal bundle (make ${RELEASE_MAKE_PACKAGE_TARGET})"
	echo
	echo "1. Copy locus/env.example to locus/.env and add provider keys."
	echo "2. First run: locus/bin/${LOCUS_DESKTOP_BIN_BASE}${ext} (double-click or ./locus/bin/${LOCUS_DESKTOP_BIN_BASE}${ext})."
	echo "3. Runtime root is locus/; config, data, logs, and run/ resolve under that root."
	echo "4. Binaries: chimera-gateway, chimera-broker, chimera-vectorstore, chimera-supervisor, chimera-indexer, bifrost-http, qdrant."
	echo
} >"$readme_tmp"
mv -f "$readme_tmp" "$OUT/README.txt"

echo "release-package: wrote $OUT"
