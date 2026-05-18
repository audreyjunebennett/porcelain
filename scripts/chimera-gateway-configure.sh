#!/usr/bin/env bash
# Local gateway config from example (make configure). gateway.yaml is gitignored.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
EX="$ROOT/config/gateway.example.yaml"
OUT="$ROOT/config/gateway.yaml"
if [[ ! -f "$EX" ]]; then
	echo "configure: missing $EX" >&2
	exit 1
fi
if [[ -f "$OUT" ]]; then
	echo "configure: $OUT already exists (not overwriting)"
	exit 0
fi
cp "$EX" "$OUT"
echo "configure: created $OUT from config/gateway.example.yaml"
