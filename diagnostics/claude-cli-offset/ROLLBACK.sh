#!/bin/sh
set -eu

target=${1:?usage: ROLLBACK.sh TARGET}
script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
cp "$script_dir/ORIGINAL_FILE.json" "$target"
printf '%s\n' "restored $target from ORIGINAL_FILE.json"
