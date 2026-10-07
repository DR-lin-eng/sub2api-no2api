#!/bin/sh
set -eu
if [ "$#" -lt 1 ] || [ "$#" -gt 2 ]; then
  printf 'usage: %s TARGET_COPY [BASELINE_COPY]\n' "$0" >&2
  exit 64
fi
target=$1
artifact_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
if [ -d "$target" ]; then
  [ -f "$target/backend/go.mod" ] || { printf 'expected a workspace copy: %s\n' "$target" >&2; exit 66; }
  tar -xzf "$artifact_dir/ROLLBACK_WORKSPACE_BASELINE.tar.gz" -C "$target"
  while IFS= read -r added; do
    case "$added" in ''|/*|*..*) printf 'invalid baseline path\n' >&2; exit 65 ;; esac
    rm -f "$target/$added"
  done < "$artifact_dir/ADDED_FILES.txt"
  printf 'workspace_restore=ok\n'
  exit 0
fi
baseline=${2:-"$target.baseline"}
if [ ! -f "$baseline" ]; then
  baseline=$artifact_dir/ORIGINAL_FILE
fi
[ -f "$target" ] || { printf 'target copy not found: %s\n' "$target" >&2; exit 66; }
[ -f "$baseline" ] || { printf 'baseline copy not found: %s\n' "$baseline" >&2; exit 66; }
tmp="$target.rollback.$$"
trap 'rm -f "$tmp"' EXIT HUP INT TERM
cp -p "$baseline" "$tmp"
mv "$tmp" "$target"
trap - EXIT HUP INT TERM
