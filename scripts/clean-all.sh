#!/usr/bin/env bash
# Full workspace reset (see Makefile clean-all). Logic lives in clean-product.sh.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
exec bash "$ROOT/scripts/clean-product.sh" --each all "${1:-}"
