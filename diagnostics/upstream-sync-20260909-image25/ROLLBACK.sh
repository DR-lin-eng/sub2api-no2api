#!/bin/sh
set -eu
TARGET=${1:?usage: ROLLBACK.sh TARGET [BASELINE_COMMIT]}
BASELINE=${2:-5988d12c7b1c0c941bb41a5fd797c9af0b5ccda6}
git -C "$TARGET" reset --hard "$BASELINE" >/dev/null
git -C "$TARGET" clean -fd >/dev/null
HEAD=$(git -C "$TARGET" rev-parse HEAD)
STATUS=$(git -C "$TARGET" status --porcelain)
if [ "$HEAD" != "$BASELINE" ] || [ -n "$STATUS" ]; then
  echo "ROLLBACK verification failed: head=$HEAD status=$STATUS" >&2
  exit 1
fi
printf 'ROLLBACK restored=%s status=clean\n' "$HEAD"
