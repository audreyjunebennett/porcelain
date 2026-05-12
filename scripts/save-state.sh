#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

SRC_DIR="${SRC_DIR:-data}"
DEST_ROOT="${DEST_ROOT:-temp/sessions}"
COMMENT="${COMMENT:-}"

if [[ ! -d "$SRC_DIR" ]]; then
  echo "save-state: source directory not found: $SRC_DIR" >&2
  exit 1
fi

mkdir -p "$DEST_ROOT"

now="$(date -u +%Y%m%d-%H%M%S)"
sha="nogit"
if command -v git >/dev/null 2>&1 && git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  sha="$(git rev-parse --short HEAD 2>/dev/null || echo nogit)"
fi

session_id="${now}_${sha}"
dest_dir="${DEST_ROOT}/${session_id}"

# Avoid accidental collision if multiple runs same second.
if [[ -e "$dest_dir" ]]; then
  i=1
  while [[ -e "${dest_dir}-${i}" ]]; do
    i=$((i+1))
  done
  dest_dir="${dest_dir}-${i}"
fi

mkdir -p "$dest_dir"

cat >"${dest_dir}/comment.txt" <<EOF
timestamp_utc=${now}
git_sha=${sha}
source_dir=${SRC_DIR}

${COMMENT}
EOF

mkdir -p "${dest_dir}/data"

# Prefer rsync if available; otherwise fall back to cp.
if command -v rsync >/dev/null 2>&1; then
  rsync -a --delete -- "${SRC_DIR}/" "${dest_dir}/data/"
else
  # -R/-T portability differs; keep it simple.
  cp -a -- "${SRC_DIR}/." "${dest_dir}/data/"
fi

echo "save-state: copied ${SRC_DIR}/ -> ${dest_dir}/data/"
echo "save-state: wrote ${dest_dir}/comment.txt"
