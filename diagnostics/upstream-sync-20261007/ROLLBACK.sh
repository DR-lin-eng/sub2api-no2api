#!/bin/bash
set -euo pipefail
repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
baseline=043203dd645eb996c339ba84a808f96f34f5a73b
target=${1:?Pass a new /private/tmp/sub2api-sync-20261007-* directory}
case "$target" in
    /private/tmp/sub2api-sync-20261007-*) ;;
    *) echo 'Only an independent task-specific temporary copy is supported.' >&2; exit 1 ;;
esac
if [ -e "$target" ]; then
    echo 'Target already exists; refusing to overwrite it.' >&2
    exit 1
fi
mkdir -p "$target"
git -C "$repo_root" archive "$baseline" | tar -x -C "$target"
cmp <(git -C "$repo_root" show "$baseline:backend/internal/bootstrap/setup/setup.go") "$target/backend/internal/bootstrap/setup/setup.go"
test ! -e "$target/backend/internal/bootstrap/setup/admin_credentials.go"
echo "ROLLBACK_PASS baseline=$baseline"
