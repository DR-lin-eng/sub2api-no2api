#!/bin/sh
set -eu

target=${1:?usage: ROLLBACK.sh TARGET BASELINE}
baseline=${2:?usage: ROLLBACK.sh TARGET BASELINE}

test -f "$target"
test -f "$baseline"
cp "$baseline" "$target"
sha256sum "$target"
