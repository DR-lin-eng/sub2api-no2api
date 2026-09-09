#!/bin/sh
set -eu
if [ "$#" -ne 2 ]; then
  echo "usage: ROLLBACK.sh BASELINE_FILE TARGET_FILE" >&2
  exit 2
fi
baseline=$1
target=$2
expected='ce96b732050d71edb7dd3630f824a15a3904b9bc0fd1e35c2e354ffc801b03d7'
[ -f "$baseline" ] || { echo "BASELINE_MISSING=$baseline"; exit 1; }
actual=$(sha256sum "$baseline" | awk '{print $1}')
[ "$actual" = "$expected" ] || { echo "BASELINE_HASH_MISMATCH=$actual"; exit 1; }
mkdir -p "$(dirname "$target")"
cp "$baseline" "$target"
restored=$(sha256sum "$target" | awk '{print $1}')
[ "$restored" = "$expected" ] || { echo "RESTORE_HASH_MISMATCH=$restored"; exit 1; }
printf '%s\n' "ROLLBACK_RESTORED=$target" "SHA256=$restored"
