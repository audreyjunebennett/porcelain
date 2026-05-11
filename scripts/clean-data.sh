#!/usr/bin/env bash
# Remove supervised BiFrost and Qdrant data dirs (defaults for claudia serve); see Makefile clean-data.
# First argument must be 1 (from make CONFIRM=1); avoids relying on Make's default shell for `test`.
set -euo pipefail
if [[ "${1:-}" != "1" ]]; then
	echo "clean-data: removes data/bifrost/, data/qdrant/, data/gateway/ — stop the stack first if running; re-run with CONFIRM=1" >&2
	exit 1
fi
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
cd "$ROOT"

DIRS=(
	"$ROOT/data/bifrost"
	"$ROOT/data/qdrant"
	"$ROOT/data/gateway"
)
removed_any=0
rm_leftovers=()

for dir in "${DIRS[@]}"; do
	if [[ -e "$dir" ]]; then
		if rm -rf -- "$dir"; then
			if [[ ! -e "$dir" ]]; then
				echo "clean-data: removed $dir/"
				removed_any=1
			else
				rm_leftovers+=("$dir")
			fi
		else
			rm_leftovers+=("$dir")
		fi
	fi
done

if [[ "${#rm_leftovers[@]}" -gt 0 ]]; then
	echo "clean-data: could not remove locked or busy paths:" >&2
	for d in "${rm_leftovers[@]}"; do
		echo "clean-data:   $d" >&2
	done
	echo "clean-data: stop claudia / BiFrost / Qdrant (make claudia-stop) then retry CONFIRM=1" >&2
	exit 1
fi

if [[ "$removed_any" -eq 0 ]]; then
	echo "clean-data: nothing to remove (data/bifrost, data/qdrant, data/gateway absent under $ROOT/)"
fi

# --- User profile / AppData (Windows): detect both files and directories; fall back when env names differ. ---
_slash() { printf '%s' "${1//\\//}"; }

append_marker() {
	local p="$1" x
	[[ -z "$p" ]] && return
	for x in "${markers[@]:-}"; do
		[[ "$x" == "$p" ]] && return
	done
	markers+=("$p")
}

append_launcher() {
	local p="$1" x
	[[ -z "$p" ]] && return
	for x in "${launchers[@]:-}"; do
		[[ "$x" == "$p" ]] && return
	done
	launchers+=("$p")
}

# ~/.claudia is often a directory (e.g. C:\Users\you\.claudia), not a single file.
markers=()
[[ -n "${HOME:-}" ]] && append_marker "$(_slash "$HOME")/.claudia"
[[ -n "${USERPROFILE:-}" ]] && append_marker "$(_slash "$USERPROFILE")/.claudia"

for mark in "${markers[@]}"; do
	if [[ ! -e "$mark" ]]; then
		continue
	fi
	if [[ -d "$mark" ]]; then
		if rm -rf -- "$mark"; then
			echo "clean-data: removed [user profile] $mark/"
		else
			echo "clean-data: could not remove directory (in use?): $mark" >&2
		fi
	else
		if rm -f -- "$mark"; then
			echo "clean-data: removed [user profile] $mark"
		else
			echo "clean-data: could not remove file (in use?): $mark" >&2
		fi
	fi
done

# Roaming launcher paths (%APPDATA%): Claudia may lay down either a shim .exe file or a same-named directory.
launchers=()
[[ -n "${APPDATA:-}" ]] && append_launcher "$(_slash "$APPDATA")/claudia.exe"
[[ -n "${APPDATA:-}" ]] && append_launcher "$(_slash "$APPDATA")/claudia-desktop.exe"
[[ -n "${USERPROFILE:-}" ]] && append_launcher "$(_slash "$USERPROFILE")/AppData/Roaming/claudia.exe"
[[ -n "${USERPROFILE:-}" ]] && append_launcher "$(_slash "$USERPROFILE")/AppData/Roaming/claudia-desktop.exe"

for path in "${launchers[@]}"; do
	if [[ ! -e "$path" ]]; then
		continue
	fi
	if [[ -d "$path" ]]; then
		if rm -rf -- "$path"; then
			echo "clean-data: removed [windows appdata roaming] $path/"
		else
			echo "clean-data: could not remove directory (in use?): $path" >&2
		fi
	else
		if rm -f -- "$path"; then
			echo "clean-data: removed [windows appdata roaming] $path"
		else
			echo "clean-data: could not remove file (in use?): $path" >&2
		fi
	fi
done
