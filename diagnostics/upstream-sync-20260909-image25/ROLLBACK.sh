#!/bin/sh
set -eu
rollback_target=${1:?usage: ROLLBACK.sh TARGET_CHECKOUT}
rollback_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd -P)
rollback_patch="$rollback_dir/DIFF_FILE"
# Check the entire reverse patch before writing; later conflicting edits stop
# the operation. HEAD, unrelated paths and untracked files remain untouched.
git -C "$rollback_target" apply --reverse --check "$rollback_patch"
git -C "$rollback_target" apply --reverse "$rollback_patch"
printf 'ROLLBACK status=restored HEAD=%s\n' "$(git -C "$rollback_target" rev-parse HEAD)"
