#!/usr/bin/env bash
# Idempotent llama-server install path for chimera-embed wrapper flows.
# Verifies toolchain and documents expected binary layout under chimera/bin/.
set -euo pipefail
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"
# shellcheck source=scripts/chimera-names.sh
source "$REPO_ROOT/scripts/chimera-names.sh"

LLAMA_BIN_DIR="${LLAMA_BIN_DIR:-$REPO_ROOT/chimera/bin}"
DEPS_DIR="${DEPS_DIR:-$REPO_ROOT/chimera/.deps}"
LLAMA_DEPS_DIR="${LLAMA_DEPS_DIR:-$DEPS_DIR/llama-server}"

echo "==> chimera-embed-install: llama-server backend"
mkdir -p "$LLAMA_BIN_DIR" "$LLAMA_DEPS_DIR/bin"

found=0
for f in "$LLAMA_BIN_DIR/llama-server" "$LLAMA_BIN_DIR/llama-server.exe"; do
	if [ -f "$f" ]; then
		echo "    OK  $f"
		found=1
	fi
done

if [ "$found" -eq 0 ]; then
	echo "    WARN  no llama-server binary under $LLAMA_BIN_DIR" >&2
	echo "    Build or copy llama-server from llama.cpp (see docker/llmservice-llamacpp in assistants repo)" >&2
	echo "    Expected names: llama-server or llama-server.exe" >&2
	echo "    Embedding model (GGUF) is operator-supplied at internal_embedding.model_path in gateway.yaml" >&2
fi

echo ""
echo "chimera-embed-install: deps cache -> $LLAMA_DEPS_DIR/bin"
echo "chimera-embed-install: done."
