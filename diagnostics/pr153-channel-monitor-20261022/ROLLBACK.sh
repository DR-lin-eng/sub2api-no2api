#!/bin/sh
set -eu
if [ "$#" -ne 1 ] || [ ! -d "$1" ]; then
  printf '%s\n' 'usage: ROLLBACK.sh TARGET_COPY_DIRECTORY' >&2
  exit 64
fi
role_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
target_copy=$(CDPATH= cd -- "$1" && pwd)
while IFS= read -r path; do
  [ -n "$path" ] || continue
  case "$path" in /*|../*|*/../*) exit 65 ;; esac
  rm -f -- "$target_copy/$path"
done < "$role_dir/ADDED_FILES.txt"
tar -xzf "$role_dir/BASELINE_SOURCE_FILES.tar.gz" -C "$target_copy"
printf '%s\n' 'ROLLBACK restored pristine source bytes'
