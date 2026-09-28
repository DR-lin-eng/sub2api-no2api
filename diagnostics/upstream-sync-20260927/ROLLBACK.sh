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
if ! sed -n '/func (s \*BackupService) UpdateS3Config/,/data, err := json.Marshal/p' "$target" | grep -q '} else {'; then
  echo 'rollback branch check failed' >&2
  exit 1
fi
printf 'restored_behavior=inherited_s3_secret_can_be_persisted_without_reencryption\n'
printf 'restored_status=baseline_sha256_match\n'
printf 'restored_sha256=%s\n' "$actual"
