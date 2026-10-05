#!/bin/sh
set -eu
if [ "$#" -ne 1 ]; then
  echo "usage: $0 /private/tmp/INDEPENDENT_TARGET_COPY" >&2
  exit 64
fi
artifact_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repo_dir=$(CDPATH= cd -- "$artifact_dir/../.." && pwd)
target=$(CDPATH= cd -- "$1" && pwd -P)
case "$target" in /private/tmp/*) ;; *) echo 'target must be an independent temporary copy' >&2; exit 64 ;; esac
test "$target" != "$repo_dir"
while IFS= read -r path; do
  case "$path" in *..*|/*) exit 64 ;; backend/*|frontend/*) rm -f "$target/$path" ;; *) exit 64 ;; esac
done < "$artifact_dir/ADDED_SOURCE_FILES.txt"
baseline=$(cat "$artifact_dir/BASELINE_SHA.txt")
git -C "$repo_dir" archive "$baseline" | tar -x -C "$target"
cmp "$artifact_dir/BASELINE_FILE" "$target/backend/internal/application/service/email_service.go"
printf 'ROLLBACK_PASS baseline=%s source_hash=' "$baseline"
shasum -a 256 "$target/backend/internal/application/service/email_service.go" | awk '{print $1}'
