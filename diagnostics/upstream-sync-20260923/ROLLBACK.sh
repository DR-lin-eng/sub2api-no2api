#!/bin/sh
set -eu
BASELINE=${1:?baseline path}
TARGET=${2:?target copy path}
cp "$BASELINE" "$TARGET"
cmp -s "$BASELINE" "$TARGET"
printf 'restored_behavior=baseline_bytes\nrestored_sha256=%s\n' "$(shasum -a 256 "$TARGET" | awk '{print $1}')"
