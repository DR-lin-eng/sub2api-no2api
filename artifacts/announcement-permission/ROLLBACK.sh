#!/bin/sh
set -eu

if [ "$#" -ne 1 ]; then
  echo "usage: ROLLBACK.sh TARGET_REPOSITORY" >&2
  exit 2
fi

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd -P)
source_root=$(CDPATH= cd -- "$script_dir/../.." && pwd -P)
target_root=$(CDPATH= cd -- "$1" && pwd -P)
patch_file="$script_dir/DIFF_FILE"

if [ "$target_root" = "$source_root" ]; then
  echo "TARGET_IS_LIVE_WORKTREE=$target_root" >&2
  exit 1
fi

test -f "$patch_file"
git -C "$target_root" apply --reverse --check "$patch_file"
git -C "$target_root" apply --reverse "$patch_file"

target_file="$target_root/backend/internal/application/service/permission_groups.go"
if command -v sha256sum >/dev/null 2>&1; then
  restored_hash=$(sha256sum "$target_file" | awk '{print $1}')
else
  restored_hash=$(shasum -a 256 "$target_file" | awk '{print $1}')
fi
expected_hash="8bcec1e7dce42be0540f53c913f0ed74a658276a6851c85e2c65149da760e7af"

if [ "$restored_hash" != "$expected_hash" ]; then
  echo "RESTORE_HASH_MISMATCH=$restored_hash" >&2
  exit 1
fi

if [ -n "$(git -C "$target_root" status --short)" ]; then
  echo "ROLLBACK_STATUS=dirty" >&2
  git -C "$target_root" status --short >&2
  exit 1
fi

printf '%s\n' \
  "ROLLBACK_RESTORED=$target_root" \
  "SHA256=$restored_hash" \
  "ROLLBACK_STATUS=clean"
