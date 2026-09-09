#!/bin/sh
set -eu
script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
target=${1:-"$script_dir/ROLLBACK_TARGET"}
cp "$script_dir/ORIGINAL_FILE" "$target"
