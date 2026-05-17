#!/usr/bin/env bash
# Install pinned Qdrant into ./bin/qdrant (or qdrant.exe on Windows). Used by install-bootstrap.sh; run directly to refresh Qdrant only.
# Version: QDRANT_RELEASE in chimera/deps.lock.
set -euo pipefail
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck source=deps-lock.sh
source "$REPO_ROOT/scripts/deps-lock.sh"
VER="$(deps_lock_get QDRANT_RELEASE)"
ROOT="$REPO_ROOT"
QDRANT_BIN_DIR="${QDRANT_BIN_DIR:-$ROOT/bin}"
BASE="https://github.com/qdrant/qdrant/releases/download/${VER}"
mkdir -p "$QDRANT_BIN_DIR"

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
arch="$(uname -m)"
case "$os" in
mingw*|msys*|cygwin*)
  case "$arch" in
  x86_64) asset="qdrant-x86_64-pc-windows-msvc.zip" ;;
  *)
    echo "qdrant-from-release: unsupported Windows arch: $arch (Qdrant ships windows amd64 only; use WSL or a manual download from ${BASE})" >&2
    exit 1
    ;;
  esac
  command -v unzip >/dev/null 2>&1 || {
    echo "qdrant-from-release: unzip is required for the Windows Qdrant zip (install Git for Windows or add unzip to PATH)." >&2
    exit 1
  }
  tmp="$(mktemp -d)"
  curl -fsSL "${BASE}/${asset}" -o "$tmp/q.zip"
  unzip -q "$tmp/q.zip" -d "$tmp"
  if [[ -f "$tmp/qdrant.exe" ]]; then
    mv "$tmp/qdrant.exe" "$QDRANT_BIN_DIR/qdrant.exe"
  else
    echo "qdrant-from-release: expected qdrant.exe in ${asset}" >&2
    exit 1
  fi
  rm -rf "$tmp"
  echo "Installed $QDRANT_BIN_DIR/qdrant.exe ($VER)"
  ;;
linux)
  case "$arch" in
  x86_64) asset="qdrant-x86_64-unknown-linux-musl.tar.gz" ;;
  aarch64 | arm64) asset="qdrant-aarch64-unknown-linux-musl.tar.gz" ;;
  *) echo "unsupported arch: $arch" >&2; exit 1 ;;
  esac
  tmp="$(mktemp -d)"
  curl -fsSL "${BASE}/${asset}" | tar xz -C "$tmp"
  mv "$tmp/qdrant" "$QDRANT_BIN_DIR/qdrant"
  chmod +x "$QDRANT_BIN_DIR/qdrant"
  rm -rf "$tmp"
  echo "Installed $QDRANT_BIN_DIR/qdrant ($VER)"
  ;;
darwin)
  case "$arch" in
  x86_64) asset="qdrant-x86_64-apple-darwin.tar.gz" ;;
  arm64) asset="qdrant-aarch64-apple-darwin.tar.gz" ;;
  *) echo "unsupported arch: $arch" >&2; exit 1 ;;
  esac
  tmp="$(mktemp -d)"
  curl -fsSL "${BASE}/${asset}" | tar xz -C "$tmp"
  mv "$tmp/qdrant" "$QDRANT_BIN_DIR/qdrant"
  chmod +x "$QDRANT_BIN_DIR/qdrant"
  rm -rf "$tmp"
  echo "Installed $QDRANT_BIN_DIR/qdrant ($VER)"
  ;;
*)
  echo "qdrant-from-release: unsupported OS/kernel: $(uname -s) (try Git Bash on Windows, WSL, Linux, or macOS; or download manually from ${BASE})" >&2
  exit 1
  ;;
esac
