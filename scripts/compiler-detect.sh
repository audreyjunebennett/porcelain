#!/usr/bin/env bash
# Shared C compiler detection for installer scripts and install-gcc.sh.
# Git Bash often does not resolve `command -v gcc` when only gcc.exe is on PATH, or when
# PATH uses `C:/...` instead of `/c/...`.

# Drive-letter path -> MSYS path (e.g. C:/Users/x -> /c/Users/x). Idempotent if already /c/...
win_msys_path() {
	local p="${1//$'\r'/}"
	p="${p//\\//}"
	if [[ "$p" =~ ^([[:alpha:]]):/(.*)$ ]]; then
		local ldrive
		ldrive=$(echo "${BASH_REMATCH[1]}" | tr '[:upper:]' '[:lower:]')
		echo "/${ldrive}/${BASH_REMATCH[2]}"
	else
		echo "$p"
	fi
}

# True if some PATH directory contains gcc.exe or gcc (Git Bash–friendly).
path_has_compiler_file() {
	local _saveIFS=$IFS
	local -a pa
	IFS=':'
	read -r -a pa <<< "$PATH"
	IFS=$_saveIFS
	local d
	for d in "${pa[@]}"; do
		d="${d//$'\r'/}"
		[[ -z "${d// }" ]] && continue
		if [[ -f "$d/gcc.exe" || -f "$d/gcc" ]]; then
			return 0
		fi
	done
	return 1
}

# First gcc.exe path for status output (Windows fallback). Echo path and return 0, or return 1.
win_first_gcc_exe_path() {
	local _saveIFS=$IFS
	local -a pa
	IFS=':'
	read -r -a pa <<< "$PATH"
	IFS=$_saveIFS
	local d
	for d in "${pa[@]}"; do
		d="${d//$'\r'/}"
		[[ -z "${d// }" ]] && continue
		if [[ -f "$d/gcc.exe" ]]; then
			echo "$d/gcc.exe"
			return 0
		fi
		if [[ -f "$d/gcc" ]]; then
			echo "$d/gcc"
			return 0
		fi
	done
	return 1
}

has_cc() {
	if command -v gcc >/dev/null 2>&1 || command -v clang >/dev/null 2>&1; then
		return 0
	fi
	case "$(uname -s 2>/dev/null)" in
	MINGW* | MSYS* | CYGWIN*)
		command -v gcc.exe >/dev/null 2>&1 && return 0
		command -v clang.exe >/dev/null 2>&1 && return 0
		command -v x86_64-w64-mingw32-gcc >/dev/null 2>&1 && return 0
		command -v x86_64-w64-mingw32-gcc.exe >/dev/null 2>&1 && return 0
		path_has_compiler_file && return 0
		;;
	esac
	return 1
}

cc_on_path() {
	local o f
	o=$(command -v gcc 2>/dev/null || command -v gcc.exe 2>/dev/null || command -v clang 2>/dev/null || command -v clang.exe 2>/dev/null || command -v x86_64-w64-mingw32-gcc 2>/dev/null || command -v x86_64-w64-mingw32-gcc.exe 2>/dev/null || true)
	[[ -n "${o:-}" ]] && echo "$o" && return 0
	case "$(uname -s 2>/dev/null)" in
	MINGW* | MSYS* | CYGWIN*)
		f=$(win_first_gcc_exe_path) || true
		[[ -n "${f:-}" ]] && echo "$f" && return 0
		;;
	esac
	echo "gcc"
	return 0
}
