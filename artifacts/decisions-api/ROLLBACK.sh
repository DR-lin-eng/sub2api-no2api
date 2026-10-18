#!/bin/sh
set -eu
if [ "$#" -ne 1 ]; then
  echo "usage: ROLLBACK.sh TARGET_FILE" >&2
  exit 2
fi
script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
baseline="$script_dir/BASELINE_FILE"
target=$1
[ -f "$baseline" ] || { echo "BASELINE_MISSING=$baseline" >&2; exit 1; }
[ -f "$target" ] || { echo "TARGET_MISSING=$target" >&2; exit 1; }
if ! grep -Eq 'POST\("/decisions"' "$target"; then
  echo "TARGET_DOES_NOT_CONTAIN_DECISIONS_ROUTE=$target" >&2
  exit 1
fi
cp "$baseline" "$target"
if ! cmp -s "$baseline" "$target"; then
  echo "RESTORE_BYTES_MISMATCH=$target" >&2
  exit 1
fi
printf '%s\n' "ROLLBACK_RESTORED=$target" 'ROUTE_BEHAVIOR=absent'
