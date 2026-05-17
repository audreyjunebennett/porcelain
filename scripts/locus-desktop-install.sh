#!/usr/bin/env bash
# make locus-desktop-install — native deps for desktop UI binary (LOCUS_DESKTOP_BIN_BASE in scripts/chimera-names.sh). Next: make locus-desktop-build.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck source=scripts/chimera-names.sh
source "$ROOT/scripts/chimera-names.sh"

os=$(uname -s)

if [[ "$os" == "Linux" ]]; then
  if [[ ! -f /etc/debian_version ]]; then
  echo "locus-desktop-install: non-Debian Linux. Install gtk+3 and webkit2gtk for your distro, then make ${CHIMERA_MAKE_DESKTOP_BUILD_TARGET}." >&2
    exit 1
  fi
  echo "locus-desktop-install: Debian/Ubuntu — WebKitGTK + build tools..."
  sudo apt-get update
  webkit_dev=libwebkit2gtk-4.0-dev
  if ! apt-cache show "$webkit_dev" >/dev/null 2>&1; then
    webkit_dev=libwebkit2gtk-4.1-dev
  fi
  sudo apt-get install -y \
    build-essential \
    gcc \
    pkg-config \
    libgtk-3-dev \
    "$webkit_dev"
  if [[ "$webkit_dev" == libwebkit2gtk-4.1-dev ]]; then
    sudo mkdir -p /usr/local/lib/pkgconfig
    # pkgconf's pkg-config shim lacks --print-filename; locate .pc via the installed package.
    wk41_pc=$(dpkg -L libwebkit2gtk-4.1-dev | grep -E '/webkit2gtk-4\.1\.pc$' | head -n1)
    test -n "$wk41_pc" && test -f "$wk41_pc"
    sudo ln -sf "$wk41_pc" /usr/local/lib/pkgconfig/webkit2gtk-4.0.pc
    echo "locus-desktop-install: webview_go uses pkg-config webkit2gtk-4.0; linked $wk41_pc as webkit2gtk-4.0.pc under /usr/local/lib/pkgconfig." >&2
  fi
  echo "locus-desktop-install: done. Next: make ${CHIMERA_MAKE_DESKTOP_BUILD_TARGET}   (output: ${LOCUS_DESKTOP_BIN_BASE}[.exe])"

elif [[ "$os" == "Darwin" ]]; then
  echo "locus-desktop-install: macOS — Xcode Command Line Tools (clang + SDK) required for CGO."
  if xcode-select -p >/dev/null 2>&1 && command -v clang >/dev/null 2>&1; then
    echo "locus-desktop-install: CLT present."
    exit 0
  fi
  echo "locus-desktop-install: run: xcode-select --install"
  xcode-select --install 2>/dev/null || true
  exit 1

elif [[ "$os" =~ ^MINGW ]] || [[ "$os" =~ ^MSYS ]] || [[ "$os" =~ ^CYGWIN ]]; then
  # shellcheck source=scripts/msys2-gcc-path.sh
  source "$ROOT/scripts/msys2-gcc-path.sh"
  echo "locus-desktop-install: Windows — use MSYS2 UCRT64 gcc (see scripts/install-gcc.sh / docs/plans/makefile-plan.md)."
  echo "locus-desktop-install: Also install the WebView2 runtime if the window is blank:"
  echo "  https://developer.microsoft.com/en-us/microsoft-edge/webview2/"
  msys2_prepend_gcc_path || true
  if command -v gcc >/dev/null 2>&1; then
    gcc --version | head -1
  fi
else
  echo "locus-desktop-install: unsupported OS: $os" >&2
  exit 1
fi
