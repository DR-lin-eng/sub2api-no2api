#!/bin/sh
set -eu
target=${1:?target copy required}
backup=${2:?backup file required}
cp "$backup" "$target"
printf "rollback restored %s from %s\\n" "$target" "$backup"
