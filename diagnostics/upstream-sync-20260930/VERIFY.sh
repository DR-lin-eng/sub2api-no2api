#!/usr/bin/env bash
set -euo pipefail
artifact_dir=$(cd -- "$(dirname -- "$0")" && pwd)
repo_dir=$(cd "$artifact_dir/../.." && pwd)
mode=${1:?baseline, modified, or rollback}
case "$mode" in
  baseline) source_dir=/private/tmp/sub2api-upstream-rollback-20260930/backend; expected=$artifact_dir/BASELINE_FILE ;;
  modified) source_dir=$repo_dir/backend; expected=$artifact_dir/MODIFIED_FILE ;;
  rollback)
    source_dir=/private/tmp/sub2api-sync-replay-rollback-20260930/backend
    mkdir -p "$(dirname "$source_dir")"
    cp -R "$repo_dir/backend" "$(dirname "$source_dir")/"
    "$artifact_dir/ROLLBACK.sh" "$source_dir/internal/application/service/gateway_tool_rewrite.go" > "$artifact_dir/replay-rollback-restore.log"
    expected=$artifact_dir/BASELINE_FILE
    ;;
  *) exit 2 ;;
esac
cmp "$expected" "$source_dir/internal/application/service/gateway_tool_rewrite.go"
docker run --rm -v "$source_dir:/workspace" -v /Users/lin/go/pkg/mod:/go/pkg/mod \
  -v sub2api-sync-gocache:/root/.cache/go-build -w /workspace golang:1.26.6-bookworm \
  go test -tags=unit ./internal/application/service -run '^TestApplyToolNameRewriteToBody' -count=1 \
  > "$artifact_dir/replay-$mode.log" 2>&1
printf '%s_PASS hash=verified tool_name_contract=preserved\n' "$(printf '%s' "$mode" | tr '[:lower:]' '[:upper:]')"
