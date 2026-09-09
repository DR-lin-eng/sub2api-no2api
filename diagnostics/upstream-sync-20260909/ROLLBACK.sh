#!/bin/sh
set -eu
HERE=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
COPY=${1:?independent git copy path is required}
BASE=${2:-dd31d9958d7d62c9e212e2c1ac54967cbd9d733a}
[ -d "$COPY/.git" ] || { echo "ROLLBACK error: $COPY is not a git copy" >&2; exit 64; }
git -C "$COPY" reset --hard --quiet "$BASE"
git -C "$COPY" clean -fd --quiet
test "$(git -C "$COPY" rev-parse HEAD)" = "$BASE"
test -z "$(git -C "$COPY" status --porcelain)"
printf 'ROLLBACK restored=%s status=clean\n' "$BASE"
