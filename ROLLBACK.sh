#!/bin/sh
set -eu
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
exec "$ROOT/diagnostics/upstream-sync-20260913/ROLLBACK.sh" "$@"
