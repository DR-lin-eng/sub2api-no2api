#!/bin/sh
set -eu
target=${1:?Pass the explicit checkout or independent copy to restore}
directory=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
exec node "$directory/EVIDENCE.mjs" rollback "$target"
