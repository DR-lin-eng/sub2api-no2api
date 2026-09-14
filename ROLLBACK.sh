#!/bin/sh
set -eu

if [ "$#" -ne 2 ]; then
  echo "usage: $0 BASELINE_FILE TARGET_COPY" >&2
  exit 64
fi

baseline=$1
target=$2
cp "$baseline" "$target"

expected=$(shasum -a 256 "$baseline" | awk '{print $1}')
actual=$(shasum -a 256 "$target" | awk '{print $1}')
test "$actual" = "$expected"

printf 'restored_behavior=baseline_bytes\n'
printf 'restored_sha256=%s\n' "$actual"
