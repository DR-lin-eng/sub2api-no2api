#!/bin/sh
set -eu

if [ "$#" -ne 2 ]; then
  echo "usage: $0 BASELINE_FILE TARGET_COPY" >&2
  exit 64
fi

baseline=$1
target=$2
cp "$baseline" "$target"

expected=$(shasum -a 256 "$baseline" | awk '{print $1}')
actual=$(shasum -a 256 "$target" | awk '{print $1}')
test "$actual" = "$expected"

if grep -Fq 'quality-pool-protection-title' "$target" || grep -Fq 'quality-red-bar-explanation' "$target"; then
  echo "rollback verification failed: public quality explanations remain" >&2
  exit 1
fi

printf 'restored_behavior=public_quality_page_without_pool_and_red_bar_explanations\n'
printf 'restored_status=baseline_copy_verified\n'
printf 'restored_sha256=%s\n' "$actual"
