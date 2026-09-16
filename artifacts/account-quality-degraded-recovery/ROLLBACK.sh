#!/bin/sh
set -eu

if [ "$#" -ne 1 ]; then
  echo "usage: ROLLBACK.sh TARGET_FILE" >&2
  exit 2
fi

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
baseline="$script_dir/BASELINE_FILE"
target=$1
expected_baseline='7bbd200e940c9e9238f35e9faae91a0d8e2f19b65a1cca41b8ba79b4c8f6d1a9'
expected_modified='84e5093172cab8f23fc9a3bf17dae605e2bdb5b709d54e9ce7f35a0cc8f84abd'

[ -f "$baseline" ] || { echo "BASELINE_MISSING=$baseline" >&2; exit 1; }
[ -f "$target" ] || { echo "TARGET_MISSING=$target" >&2; exit 1; }

baseline_hash=$(shasum -a 256 "$baseline" | awk '{print $1}')
[ "$baseline_hash" = "$expected_baseline" ] || { echo "BASELINE_HASH_MISMATCH=$baseline_hash" >&2; exit 1; }

target_hash=$(shasum -a 256 "$target" | awk '{print $1}')
[ "$target_hash" = "$expected_modified" ] || { echo "TARGET_HASH_MISMATCH=$target_hash" >&2; exit 1; }

cp "$baseline" "$target"
restored_hash=$(shasum -a 256 "$target" | awk '{print $1}')
[ "$restored_hash" = "$expected_baseline" ] || { echo "RESTORE_HASH_MISMATCH=$restored_hash" >&2; exit 1; }

printf '%s\n' "ROLLBACK_RESTORED=$target" "SHA256=$restored_hash"
