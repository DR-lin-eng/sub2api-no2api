#!/bin/sh
set -eu

if [ "$#" -ne 1 ]; then
  printf 'usage: %s TARGET_COPY\n' "$0" >&2
  exit 2
fi

artifact_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
target=$1
expected_sha256=3806440e51b321294b3d9f182685e1e64fefde097c6964761533809d5004af60

patch -R -s "$target" < "$artifact_dir/DIFF_FILE"
actual_sha256=$(shasum -a 256 "$target" | awk '{print $1}')
if [ "$actual_sha256" != "$expected_sha256" ]; then
  printf 'ROLLBACK_RESULT=hash_mismatch\nEXPECTED_SHA256=%s\nACTUAL_SHA256=%s\n' "$expected_sha256" "$actual_sha256" >&2
  exit 1
fi

printf 'ROLLBACK_RESULT=restored\nRESTORED_SHA256=%s\nRESTORED_BEHAVIOR=auto_enable_after_quota_reset_enabled=false,auto_enable_when_quota_available_enabled=false\n' "$actual_sha256"
