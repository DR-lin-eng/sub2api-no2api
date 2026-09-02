#!/bin/sh
set -eu

if [ "$#" -ne 1 ]; then
  printf 'usage: %s TARGET_COPY\n' "$0" >&2
  exit 2
fi

artifact_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
target=$1
expected_sha256=2e157eb7de21857ec38d1733482900cfa2a090a4fa9cc59ef48446429f2b05b0

patch -R -s "$target" < "$artifact_dir/DIFF_FILE"
actual_sha256=$(shasum -a 256 "$target" | awk '{print $1}')
if [ "$actual_sha256" != "$expected_sha256" ]; then
  printf 'ROLLBACK_RESULT=hash_mismatch\nEXPECTED_SHA256=%s\nACTUAL_SHA256=%s\n' "$expected_sha256" "$actual_sha256" >&2
  exit 1
fi

printf 'ROLLBACK_RESULT=restored\nRESTORED_SHA256=%s\nRESTORED_BEHAVIOR=auto_disable_enabled=false,auto_disable_threshold=3\n' "$actual_sha256"
