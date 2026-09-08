#!/bin/sh
set -eu
HERE=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
REPO=$(CDPATH= cd -- "$HERE/../.." && pwd)
COPY=${1:?independent copy is required}
BASE=${2:-2c6606b8374d64c3afd3dae0447eaf19f521523c}
git -C "$COPY" diff --quiet "$BASE" -- || git -C "$COPY" diff "$BASE" --no-ext-diff | git -C "$COPY" apply -R --whitespace=nowarn
git -C "$COPY" read-tree -m -u "$BASE"
test "$(git -C "$COPY" rev-parse HEAD)" = "$BASE"
printf 'ROLLBACK restored=%s\n' "$BASE"
