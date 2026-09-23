#!/bin/sh
set -eu
SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
TARGET=${1:-"$SCRIPT_DIR/target-under-test.go"}
if [ ! -f "$SCRIPT_DIR/ORIGINAL_FILE" ]; then
  echo "missing ORIGINAL_FILE" >&2
  exit 1
fi
cp "$SCRIPT_DIR/ORIGINAL_FILE" "$TARGET"
printf 'restored %s\n' "$TARGET"
