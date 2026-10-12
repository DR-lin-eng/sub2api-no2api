#!/usr/bin/env bash
set -euo pipefail
artifact_dir=$(cd -- "$(dirname -- "$0")" && pwd)
target=${1:?Pass the gateway_tool_rewrite.go copy to restore}
expected_modified=c47c5246114316e0270e721945637114cb558261d5eb7e0b2a622ce3edac66d0
expected_baseline=a9270259de4357d4d476cccb6a032a8c43933eb2d89cf054c20b7f11ffa3e671
actual=$(shasum -a 256 "$target" | cut -d ' ' -f 1)
if [[ "$actual" != "$expected_modified" ]]; then
  printf 'ROLLBACK_HASH_MISMATCH expected=%s actual=%s\n' "$expected_modified" "$actual" >&2
  exit 3
fi
cp "$artifact_dir/BASELINE_FILE" "$target"
restored=$(shasum -a 256 "$target" | cut -d ' ' -f 1)
[[ "$restored" == "$expected_baseline" ]]
printf 'ROLLBACK_OK sha256=%s file=%s\n' "$restored" "$target"
