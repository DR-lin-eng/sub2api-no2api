#!/bin/sh
set -eu
BASELINE=${1:?baseline file required}
TARGET=${2:?target file required}
cp "$BASELINE" "$TARGET"
if cmp -s "$BASELINE" "$TARGET"; then
  echo "restored_behavior=baseline_bytes"
  echo "restored_sha256=$(shasum -a 256 "$TARGET" | awk '{print $1}')"
  exit 0
fi
echo "restored_behavior=failed" >&2
exit 1
