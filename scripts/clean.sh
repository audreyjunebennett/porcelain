#!/usr/bin/env bash
# Remove local build artifacts for all products (see Makefile clean).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
exec bash "$ROOT/scripts/clean-product.sh" --each build
