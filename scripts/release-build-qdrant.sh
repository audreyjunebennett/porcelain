#!/usr/bin/env bash
# GoReleaser before hook (make release-build): Qdrant bundles for packaging/qdrant-bundles/.
# Version: QDRANT_RELEASE in chimera/deps.lock.
# Requires: curl, tar; unzip for Windows asset.
set -euo pipefail
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck source=deps-lock.sh
source "$REPO_ROOT/scripts/deps-lock.sh"
VER="$(deps_lock_get QDRANT_RELEASE)"
ROOT="$REPO_ROOT"
BASE="https://github.com/qdrant/qdrant/releases/download/${VER}"
# Outside dist/ so GoReleaser --clean does not race with this hook (hook runs after clean).
DEST="$ROOT/packaging/qdrant-bundles"
mkdir -p "$DEST"

fetch_tgz() {
  local asset="$1" goos="$2" goarch="$3"
  local out="$DEST/${goos}_${goarch}"
  mkdir -p "$out"
  local tmp
  tmp="$(mktemp -d)"
  curl -fsSL "${BASE}/${asset}" | tar xz -C "$tmp"
  if [[ -f "$tmp/qdrant" ]]; then
    mv "$tmp/qdrant" "$out/qdrant"
  else
    echo "release-build-qdrant: expected qdrant binary in ${asset}" >&2
    ls -la "$tmp" >&2
    exit 1
  fi
  chmod +x "$out/qdrant"
  rm -rf "$tmp"
}

fetch_zip_win() {
  local out="$DEST/windows_amd64"
  mkdir -p "$out"
  local tmp
  tmp="$(mktemp -d)"
  curl -fsSL "${BASE}/qdrant-x86_64-pc-windows-msvc.zip" -o "$tmp/q.zip"
  unzip -q "$tmp/q.zip" -d "$tmp"
  if [[ -f "$tmp/qdrant.exe" ]]; then
    mv "$tmp/qdrant.exe" "$out/qdrant.exe"
  else
    echo "release-build-qdrant: expected qdrant.exe in windows zip" >&2
    find "$tmp" -maxdepth 2 -type f >&2
    exit 1
  fi
  rm -rf "$tmp"
}

fetch_tgz "qdrant-x86_64-unknown-linux-musl.tar.gz" linux amd64
fetch_tgz "qdrant-aarch64-unknown-linux-musl.tar.gz" linux arm64
fetch_tgz "qdrant-x86_64-apple-darwin.tar.gz" darwin amd64
fetch_tgz "qdrant-aarch64-apple-darwin.tar.gz" darwin arm64
fetch_zip_win

echo "release-build-qdrant: Qdrant ${VER} → packaging/qdrant-bundles/{linux,darwin}_*/qdrant, windows_amd64/qdrant.exe"
