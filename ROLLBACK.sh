#!/bin/sh
set -eu
BASELINE="${1:?baseline file is required}"
TARGET="${2:?rollback target is required}"
cp "$BASELINE" "$TARGET"
cmp -s "$BASELINE" "$TARGET"
echo "restored: $TARGET"
