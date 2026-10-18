#!/bin/sh
set -eu
if [ "$#" -ne 1 ]; then
  echo "usage: PROBE.sh GATEWAY_SOURCE" >&2
  exit 2
fi
source_file=$1
[ -f "$source_file" ] || { echo "SOURCE_MISSING=$source_file" >&2; exit 1; }
if grep -Eq 'POST\("/decisions"' "$source_file"; then
  printf '%s\n' 'DECISIONS_ROUTE=present'
else
  printf '%s\n' 'DECISIONS_ROUTE=absent'
fi
