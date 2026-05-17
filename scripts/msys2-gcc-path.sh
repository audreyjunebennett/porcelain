#!/usr/bin/env bash
# MSYS2 MinGW GCC on PATH for Windows CGO (Fyne). Source from other scripts.
# Non-interactive shells (e.g. make locus-desktop-build) do not load ~/.bashrc; call msys2_prepend_gcc_path.
# shellcheck shell=bash

_msys2_gcc_path_script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
_msys2_gcc_path_repo="$(cd "$_msys2_gcc_path_script_dir/.." && pwd)"
# shellcheck source=scripts/compiler-detect.sh
source "$_msys2_gcc_path_repo/scripts/compiler-detect.sh"

msys2_resolve_root() {
  local r
  local -a candidates=()
  if [[ -n "${MSYS2_ROOT:-}" ]]; then
    candidates+=("${MSYS2_ROOT//\\//}")
  fi
  candidates+=("/c/msys64" "/c/Program Files/MSYS2" "/c/Program Files (x86)/MSYS2")
  for r in "${candidates[@]}"; do
    r="${r//\\//}"
    r="${r%/}"
    if [[ -x "$r/usr/bin/bash.exe" ]]; then
      echo "$r"
      return 0
    fi
  done
  return 1
}

# If gcc is already visible, no-op (exit 0). Else prepend ucrt64/mingw64 bin when gcc.exe exists there.
# Sets CHIMERA_MSYS2_TOOLCHAIN_ROOT and CHIMERA_MSYS2_TOOLCHAIN_ENV when prepended (for gui-install ~/.bashrc).
# Returns 0 if has_cc after this runs, else 1.
msys2_prepend_gcc_path() {
  unset CHIMERA_MSYS2_TOOLCHAIN_ROOT CHIMERA_MSYS2_TOOLCHAIN_ENV 2>/dev/null || true
  case "$(uname -s 2>/dev/null)" in
  MINGW* | MSYS* | CYGWIN*) ;;
  *) return 0 ;;
  esac
  if has_cc; then
    return 0
  fi
  local mroot=""
  if ! mroot="$(msys2_resolve_root)"; then
    return 1
  fi
  local env
  for env in ucrt64 mingw64; do
    if [[ -f "$mroot/$env/bin/gcc.exe" ]]; then
      export PATH="$mroot/$env/bin:$mroot/usr/bin:$PATH"
      export CHIMERA_MSYS2_TOOLCHAIN_ROOT="$mroot"
      export CHIMERA_MSYS2_TOOLCHAIN_ENV="$env"
      if has_cc; then
        return 0
      fi
      return 1
    fi
  done
  return 1
}

unset _msys2_gcc_path_script_dir _msys2_gcc_path_repo
