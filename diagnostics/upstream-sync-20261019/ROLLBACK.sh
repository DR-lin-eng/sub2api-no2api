#!/bin/sh
set -eu
if [ "$#" -ne 1 ] || [ ! -d "$1" ]; then
  echo "usage: ROLLBACK.sh TARGET_COPY_DIRECTORY" >&2
  exit 64
fi
role_dir=$(CDPATH= cd -P "$(dirname "$0")" && pwd)
target=$(CDPATH= cd -P "$1" && pwd)
case "$target" in
  /|/Users/lin/.codex/visualizations/2026/10/19/01a154a8-4f2d-7902-9e56-fbf19901c236/upstream-sync-20261019/repo|/Users/lin/.codex/worktrees/1f7d/sub2api-no2api)
    echo "refusing original workspace or candidate checkout" >&2
    exit 64 ;;
esac
[ -f "$role_dir/BASELINE_SOURCE_FILES.tar.gz" ] || { echo "missing sibling baseline archive" >&2; exit 66; }
while IFS= read -r name; do
  [ -z "$name" ] || rm -f "$target/$name"
done <<'ADDED_SOURCE_PATHS'
backend/internal/application/service/gemini_messages_stream.go
backend/internal/application/service/gemini_native_stream.go
backend/internal/application/service/gemini_stream_client_abort.go
backend/internal/application/service/gemini_stream_client_abort_local_test.go
backend/internal/application/service/gemini_stream_client_abort_test.go
backend/internal/application/service/gemini_usage_cache_only_test.go
backend/internal/application/service/image_storage_download_test.go
backend/internal/shared/antigravity/usage_cache_only_test.go
docs/UPSTREAM_SYNC_20261019.md
ADDED_SOURCE_PATHS
tar -xzf "$role_dir/BASELINE_SOURCE_FILES.tar.gz" -C "$target"
echo "Restored pristine source bytes in $target"
