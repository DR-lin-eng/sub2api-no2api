#!/bin/sh
set -eu

target=${1:?usage: ROLLBACK.sh TARGET}
script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
cp "$script_dir/BASELINE_FILE.json" "$target"
printf 'restored %s from %s\n' "$target" "$script_dir/BASELINE_FILE.json"
