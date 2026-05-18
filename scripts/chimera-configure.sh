#!/usr/bin/env bash
# Materialize local gateway config from examples (never overwrites existing files).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
COPY="$ROOT/scripts/configure-copy.sh"
missing=0
"$COPY" gateway.example.yaml gateway.yaml || missing=1
"$COPY" api-keys.example.yaml api-keys.yaml || missing=1
exit "$missing"
