#!/usr/bin/env bash
# Shared CONFIRM=1 gate for destructive Make targets (clean-product run, clean-all).
# Usage: source scripts/confirm.sh; require_confirm "${1:-}" "message…"
require_confirm() {
	if [[ "${1:-}" != "1" ]]; then
		echo "${2:?require_confirm: missing message}" >&2
		exit 1
	fi
}
