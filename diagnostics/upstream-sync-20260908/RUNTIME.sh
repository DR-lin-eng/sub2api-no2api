#!/bin/sh
set -eu
HERE=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
MODE=${1:?BASELINE|MODIFIED|ROLLBACK|RESTORED_MODIFIED}
case "$MODE" in
  BASELINE|ROLLBACK) export SYNC_IMAGE=sub2api-upstream-5ae0:baseline ;;
  MODIFIED|RESTORED_MODIFIED) export SYNC_IMAGE=sub2api-upstream-5ae0:modified ;;
  *) exit 64 ;;
esac
docker compose -p sub2api-upstream-5ae0 -f "$HERE/runtime.yml" up -d --wait --wait-timeout 180
printf '%s health=' "$MODE"
curl --fail --silent http://127.0.0.1:18871/health
printf '\n'
printf '%s ready=' "$MODE"
curl --fail --silent http://127.0.0.1:18871/ready
printf '\n'
docker compose -p sub2api-upstream-5ae0 -f "$HERE/runtime.yml" exec -T postgres psql -U fixture -d fixture -Atc "SELECT count(*), max(filename) FROM schema_migrations"
docker compose -p sub2api-upstream-5ae0 -f "$HERE/runtime.yml" ps --format json
