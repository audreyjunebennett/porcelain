#!/usr/bin/env bash
# Seed vectorstore qdrant config if missing (idempotent).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT="$ROOT/config/qdrant.config.yaml"

if [[ -f "$OUT" ]]; then
	echo "chimera-vectorstore-configure: $OUT already exists (not overwriting)"
	exit 0
fi

mkdir -p "$ROOT/config"

# Prefer a local example if added later; otherwise create a minimal scaffold.
for src in \
	"$ROOT/config/qdrant.config.example.yaml"; do
	if [[ -f "$src" ]]; then
		cp "$src" "$OUT"
		echo "chimera-vectorstore-configure: created $OUT from ${src#$ROOT/}"
		exit 0
	fi
done

cat >"$OUT" <<'EOF'
# qdrant uses env vars by default in chimera-vectorstore.
# This file is reserved for future wrapper-owned config materialization.
service:
  host: 127.0.0.1
  http_port: 6333
  grpc_port: 6334
EOF
echo "chimera-vectorstore-configure: created minimal $OUT"
