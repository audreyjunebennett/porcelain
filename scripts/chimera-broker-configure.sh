#!/usr/bin/env bash
# Seed broker config if missing (idempotent).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT="$ROOT/config/chimera-broker.config.json"

if [[ -f "$OUT" ]]; then
	echo "chimera-broker-configure: $OUT already exists (not overwriting)"
	exit 0
fi

mkdir -p "$ROOT/config"

# Prefer local examples; then migrate/copy legacy bifrost config names when present.
for src in \
	"$ROOT/config/chimera-broker.config.example.json" \
	"$ROOT/config/chimera-broker.config.json" \
	"$ROOT/config/bifrost.config.example.json" \
	"$ROOT/config/bifrost.config.json" \
	"$ROOT/../config/bifrost.config.json"; do
	if [[ -f "$src" ]]; then
		cp "$src" "$OUT"
		echo "chimera-broker-configure: created $OUT from ${src#$ROOT/}"
		exit 0
	fi
done

cat >"$OUT" <<'EOF'
{}
EOF
echo "chimera-broker-configure: created minimal $OUT (empty object)"
