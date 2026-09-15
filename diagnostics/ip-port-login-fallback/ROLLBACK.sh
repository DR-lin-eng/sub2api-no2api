#!/bin/sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
TARGET=${1:-"$SCRIPT_DIR/target-under-test.ts"}
EXPECTED_SHA256=26f95615a94482c6b129123680fafc4347cb0bdefc8e4892e80b219a6bf20be4

cp "$SCRIPT_DIR/ORIGINAL_FILE" "$TARGET"
RESTORED_SHA256=$(shasum -a 256 "$TARGET" | awk '{print $1}')

printf 'restored=%s\n' "$TARGET"
printf 'rollback_restored_sha256=%s\n' "$RESTORED_SHA256"
printf 'expected_sha256=%s\n' "$EXPECTED_SHA256"
printf 'restored_behavior=SubtleCrypto-only baseline; HTTP IP-and-port fallback absent\n'

test "$RESTORED_SHA256" = "$EXPECTED_SHA256"
