#!/bin/sh
# Restore this transaction only; retain audit records and unrelated copy files.
set -eu
if [ "$#" -ne 1 ] || [ ! -d "$1" ]; then
  echo "usage: ROLLBACK.sh TARGET_COPY_DIRECTORY" >&2
  exit 64
fi
role_dir=$(CDPATH= cd -P "$(dirname "$0")" && pwd)
target=$(CDPATH= cd -P "$1" && pwd)
case "$target" in
  /|/Users/lin/.codex/worktrees/2ca9/sub2api-no2api|/private/tmp/sub2api-upstream-sync-20261016)
    echo "refusing original workspace or candidate checkout" >&2; exit 64 ;;
esac
baseline="$role_dir/BASELINE_SOURCE_FILES.tar.gz"
[ -f "$baseline" ] || { echo "missing sibling baseline archive" >&2; exit 66; }
while IFS= read -r name; do
  [ -n "$name" ] && rm -f "$target/$name"
done <<'ADDED_SOURCE_PATHS'
backend/internal/application/service/gemini_compat_thinking.go
backend/internal/application/service/gemini_compat_thinking_test.go
backend/internal/application/service/gemini_compat_thinking_upstream_test.go
backend/internal/application/service/openai_model_retirement.go
backend/internal/application/service/openai_model_retirement_test.go
backend/internal/application/service/openai_responses_created_at.go
backend/internal/application/service/openai_responses_created_at_test.go
backend/internal/application/service/upstream_followup_benchmark_test.go
ADDED_SOURCE_PATHS
tar -xzf "$baseline" -C "$target"
echo "Restored pristine source bytes in $target"
