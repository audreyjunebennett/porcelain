#!/usr/bin/env bash
# make desktop-install — native deps for claudia-desktop (webview_go + CGO). Next: make desktop-build.
set -euo pipefail

os=$(uname -s)

if [[ "$os" == "Linux" ]]; then
  if [[ ! -f /etc/debian_version ]]; then
    echo "desktop-install: non-Debian Linux. Install gtk+3 and webkit2gtk for your distro, then make desktop-build." >&2
    exit 1
  fi
  echo "desktop-install: Debian/Ubuntu — WebKitGTK + build tools..."
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
    echo "desktop-install: webview_go uses pkg-config webkit2gtk-4.0; linked $wk41_pc as webkit2gtk-4.0.pc under /usr/local/lib/pkgconfig." >&2
  fi
  echo "desktop-install: done. Next: make desktop-build"

elif [[ "$os" == "Darwin" ]]; then
  echo "desktop-install: macOS — Xcode Command Line Tools (clang + SDK) required for CGO."
  if xcode-select -p >/dev/null 2>&1 && command -v clang >/dev/null 2>&1; then
    echo "desktop-install: CLT present."
    exit 0
  fi
  echo "desktop-install: run: xcode-select --install"
  xcode-select --install 2>/dev/null || true
  exit 1

elif [[ "$os" =~ ^MINGW ]] || [[ "$os" =~ ^MSYS ]] || [[ "$os" =~ ^CYGWIN ]]; then
  REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
  # shellcheck source=scripts/msys2-gcc-path.sh
  source "$REPO_ROOT/scripts/msys2-gcc-path.sh"
  echo "desktop-install: Windows — use MSYS2 UCRT64 gcc (see scripts/install-gcc.sh / docs/plans/makefile-plan.md)."
  echo "desktop-install: Also install the WebView2 runtime if the window is blank:"
  echo "  https://developer.microsoft.com/en-us/microsoft-edge/webview2/"
  msys2_prepend_gcc_path || true
  if command -v gcc >/dev/null 2>&1; then
    gcc --version | head -1
  fi
else
  echo "desktop-install: unsupported OS: $os" >&2
  exit 1
fi
