#!/bin/sh
set -eu
baseline_name=ORIGINAL_FILE.vue
if [ "$#" -eq 2 ] && [ "$1" = "--publication" ]; then
  baseline_name=PUBLICATION_BASELINE.vue
  shift
fi
if [ "$#" -ne 1 ]; then
  echo "Usage: $0 [--publication] TARGET_COPY" >&2
  exit 64
fi
rollback_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
pristine="$rollback_dir/$baseline_name"
target_copy=$1
if [ "$target_copy" = "$pristine" ]; then
  echo "TARGET_COPY must differ from the pristine sibling" >&2
  exit 64
fi
cp "$pristine" "$target_copy"
