#!/bin/sh
set -eu

if [ "$#" -ne 1 ]; then
  echo "usage: ROLLBACK.sh TARGET_GIT_TREE" >&2
  exit 2
fi

target=$1
script_dir=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
diff_file="$script_dir/DIFF_FILE"

[ -e "$target/.git" ] || { echo "TARGET_IS_NOT_A_GIT_TREE=$target" >&2; exit 1; }
[ -s "$diff_file" ] || { echo "DIFF_FILE_MISSING=$diff_file" >&2; exit 1; }

git -C "$target" apply --unidiff-zero --whitespace=nowarn --reverse --check "$diff_file"
git -C "$target" apply --unidiff-zero --whitespace=nowarn --reverse "$diff_file"

status=$(git -C "$target" status --porcelain)
[ -z "$status" ] || { printf '%s\n' "ROLLBACK_STATUS_NOT_CLEAN" "$status" >&2; exit 1; }

restored_hash=$(git -C "$target" show HEAD:backend/internal/transport/webassets/embed_on.go | shasum -a 256 | awk '{print $1}')
printf '%s\n' \
  "ROLLBACK_RESTORED=$target" \
  "WORKTREE_STATUS=clean" \
  "RESTORED_ANCHOR_SHA256=$restored_hash"
