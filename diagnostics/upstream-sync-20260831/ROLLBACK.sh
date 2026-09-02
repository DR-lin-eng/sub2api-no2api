#!/bin/sh
set -eu

if [ "$#" -ne 1 ]; then
  printf 'usage: %s TARGET_COPY\n' "$0" >&2
  exit 2
fi

artifact_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
target=$1
expected_sha256=7a13d4a44ca18c40836cec39900ac1576ce41e556b7c15bcace27cf67844c5d8

patch -R -s "$target" < "$artifact_dir/DIFF_FILE"
actual_sha256=$(shasum -a 256 "$target" | awk '{print $1}')
if [ "$actual_sha256" != "$expected_sha256" ]; then
  printf 'ROLLBACK_RESULT=hash_mismatch\nEXPECTED_SHA256=%s\nACTUAL_SHA256=%s\n' "$expected_sha256" "$actual_sha256" >&2
  exit 1
fi

printf 'ROLLBACK_RESULT=restored\nRESTORED_SHA256=%s\nRESTORED_BEHAVIOR=branch=unreviewed-upstream-delta,field=pending,raw_stream_status=false_success\n' "$actual_sha256"
