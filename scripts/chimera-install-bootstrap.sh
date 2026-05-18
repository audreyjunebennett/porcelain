#!/usr/bin/env bash
# Install BiFrost + Qdrant into ./bin (same as make chimera-install; for docs/manual use).
set -euo pipefail
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
export CHIMERA_BROKER_BIN_DIR="${CHIMERA_BROKER_BIN_DIR:-$REPO_ROOT/bin}"
export QDRANT_BIN_DIR="${QDRANT_BIN_DIR:-$REPO_ROOT/bin}"
export DEPS_DIR="${DEPS_DIR:-$REPO_ROOT/.deps}"
bash "$REPO_ROOT/scripts/chimera-broker-install.sh"
bash "$REPO_ROOT/scripts/chimera-vectorstore-install.sh"
