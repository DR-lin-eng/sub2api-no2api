#!/bin/sh
set -eu

source_file="${1:?source file required}"
copy_file="${2:?copy file required}"
cp "$copy_file" "$source_file"
printf 'restored %s from %s\n' "$source_file" "$copy_file"
