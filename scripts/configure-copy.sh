#!/usr/bin/env bash
# Idempotent copy: create DEST from SOURCE when DEST is missing. Never overwrites.
# Paths without a directory component resolve under config/ (repo root relative otherwise).
#
# Usage:
#   configure-copy.sh SOURCE DEST
#
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
CONFIG_DIR="${CHIMERA_CONFIG_DIR:-$ROOT/config}"

usage() {
	echo "Usage: $(basename "$0") SOURCE DEST" >&2
	echo "  Filenames without / resolve under config/." >&2
	exit 2
}

resolve_path() {
	local p="$1"
	if [[ "$p" == /* ]]; then
		printf '%s\n' "$p"
	elif [[ "$p" == */* ]]; then
		printf '%s\n' "$ROOT/$p"
	else
		printf '%s\n' "$CONFIG_DIR/$p"
	fi
}

[[ $# -eq 2 ]] || usage

src="$(resolve_path "$1")"
dst="$(resolve_path "$2")"

if [[ -f "$dst" ]]; then
	echo "configure-copy: ${dst#$ROOT/} already exists (not overwriting)"
	exit 0
fi

if [[ ! -f "$src" ]]; then
	echo "configure-copy: missing source ${src#$ROOT/}" >&2
	exit 1
fi

mkdir -p "$(dirname "$dst")"
cp "$src" "$dst"
echo "configure-copy: created ${dst#$ROOT/} from ${src#$ROOT/}"
