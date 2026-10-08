#!/bin/sh
set -eu
role_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd -P)
target=${1:?Usage: ROLLBACK.sh /absolute/path/to/an/independent/modified/copy}
test -d "$target" || { echo 'Target copy must exist.' >&2; exit 1; }
target=$(CDPATH= cd -- "$target" && pwd -P)
repo_root=$(CDPATH= cd -- "$role_dir/../.." && pwd -P)
test "$target" != "$repo_root" || { echo 'Use an independent copy.' >&2; exit 1; }
test "$target" != '/Users/lin/.codex/worktrees/dab8/sub2api-no2api' || { echo 'Use an independent copy.' >&2; exit 1; }
while IFS= read -r path; do
    test -n "$path" || continue
    case "$path" in backend/*|frontend/*) ;; *) echo 'Invalid baseline path.' >&2; exit 1;; esac
    rm -f -- "$target/$path"
done < "$role_dir/ADDED_SOURCE_FILES.txt"
tar -xzf "$role_dir/BASELINE_FILE" -C "$target"
tab=$(printf '\t')
while IFS="$tab" read -r expected path; do
    if command -v sha256sum >/dev/null 2>&1; then
        observed=$(sha256sum "$target/$path" | cut -d ' ' -f 1)
    else
        observed=$(shasum -a 256 "$target/$path" | cut -d ' ' -f 1)
    fi
    test "$observed" = "$expected" || { echo "Hash mismatch: $path" >&2; exit 1; }
done < "$role_dir/SOURCE_FILES_SHA256.txt"
echo 'ROLLBACK_PASS baseline=6fcc1541fae51d0f55b9d39fe2e6d144420e11fa'
