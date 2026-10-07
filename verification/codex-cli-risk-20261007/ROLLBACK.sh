#!/bin/sh
set -eu
if [ "$#" -lt 1 ] || [ "$#" -gt 2 ]; then
  printf 'usage: %s TARGET_COPY [BASELINE_COPY]\n' "$0" >&2
  exit 64
fi
target=$1
baseline=${2:-"$target.baseline"}
if [ ! -f "$baseline" ]; then
  baseline=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)/ORIGINAL_FILE
fi
[ -f "$target" ] || { printf 'target copy not found: %s\n' "$target" >&2; exit 66; }
[ -f "$baseline" ] || { printf 'baseline copy not found: %s\n' "$baseline" >&2; exit 66; }
tmp="$target.rollback.$$"
trap 'rm -f "$tmp"' EXIT HUP INT TERM
cp "$baseline" "$tmp"
chmod --reference="$target" "$tmp" 2>/dev/null || true
mv "$tmp" "$target"
trap - EXIT HUP INT TERM
