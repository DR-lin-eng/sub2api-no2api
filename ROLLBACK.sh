#!/bin/sh
set -eu
BASELINE="${1:-/tmp/sub2api-quality-split-20260912/BASELINE_FILE}"
TARGET="${2:-/tmp/sub2api-quality-split-20260912/rollback-copy.go}"
cp "$BASELINE" "$TARGET"
cmp -s "$BASELINE" "$TARGET"
echo "restored: $TARGET"
