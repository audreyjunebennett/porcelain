#!/usr/bin/env bash
# Used by Makefile clean-all on Windows (POSIX test in the Makefile runs under cmd.exe and fails).
set -euo pipefail
if [[ "${1:-}" != "1" ]]; then
	echo "clean-all: removes bin/, packaging/qdrant-bundles/, packages/, node_modules/, .deps/, run/, logs/ (+ make clean: claudia*, dist/). On Windows also removes %APPDATA%/claudia.exe and %APPDATA%/claudia-desktop.exe when present — re-run with CONFIRM=1" >&2
	exit 1
fi
