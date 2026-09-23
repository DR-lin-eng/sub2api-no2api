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

if grep -Eq 'Platform(Kimi|Zhipu|Deepseek|MiniMax)' "$target"; then
  echo "rollback verification failed: CN provider constants remain" >&2
  exit 1
fi

printf 'restored_behavior=legacy_platform_constants\n'
printf 'restored_status=cn_provider_constants_absent\n'
printf 'restored_sha256=%s\n' "$actual"
